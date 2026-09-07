# Coordinate two tasks through one budget

One guarded command is not very interesting. In this tutorial we will run two at once, watch them
share a single host budget, and see HIPPO refuse a request that could never fit.

It takes about ten minutes. We will use a throwaway state root so nothing here can collide with real
work already running on your machine.

You should have finished [Guard your first command](./guard-your-first-command.md) first.

## Step 1: use a throwaway state root

Every repository on a host that shares a state root coordinates through the same ledger. That is
exactly the behavior we want to observe — but for a tutorial we want it isolated, so let's point
HIPPO somewhere temporary for this shell only.

```sh
cd hippo
export HIPPO_ROOT="$(mktemp -d)/hippo-tutorial"
echo "$HIPPO_ROOT"
```

```console
/var/folders/fr/jg3jv_4d39b48cyqlqz18mgr0000gn/T/tmp.TYCeLRxNKH/hippo-tutorial
```

Your path will differ. When you close this shell, the variable goes with it and HIPPO returns to its
normal root.

## Step 2: turn on reservation coordination

Without configuration HIPPO uses the older _exclusive_ mode, where one heavy task runs at a time.
Reservation mode is what lets several tasks run concurrently against a shared budget, and it is opted
into with a schema-2 configuration file.

```sh
cat > local-tmp/tutorial.json <<'JSON'
{
  "schemaVersion": 2,
  "coordination": { "mode": "reservation" }
}
JSON
```

`local-tmp/` is ignored by Git, so this file will not follow you into a commit.

Now look at the ledger before anything is running:

```sh
./hippo status --config local-tmp/tutorial.json --json --disk-path . | grep -o '"coordination":{[^}]*}[^}]*}[^}]*}[^}]*}'
```

```console
"coordination":{"schemaVersion":4,"mode":"reservation","capacity":{"cpu":0,"memoryBytes":0},"allocated":{"cpu":0,"memoryBytes":0},"waiting":{"cpu":0,"memoryBytes":0},"activeOwners":0,"waitingOwners":0,"ephemeral":0,"service":0,"transactional":0}
```

`mode` is now `reservation`. Everything else is zero: an idle ledger has no owners, and reports no
capacity because there is no live epoch to describe.

## Step 3: claim a reservation and hold it

Let's start a task that holds a small reservation for 25 seconds, in the background, so we can
inspect the ledger while it runs.

```sh
./hippo run --config local-tmp/tutorial.json --disk-path . \
  --reserve-cpu 2 --reserve-memory-mib 1024 -- sleep 25 &
```

We asked for exactly 2 CPU and 1024 MiB rather than letting HIPPO pick an automatic share, because
fixed numbers make the next step easier to read.

Give admission a few seconds to complete:

```sh
sleep 6
```

## Step 4: watch a second HIPPO see the first one

This next command is a completely separate HIPPO process. It has no connection to the background job
except the shared state root.

```sh
./hippo status --config local-tmp/tutorial.json --json --disk-path . | grep -o '"coordination":{[^}]*}[^}]*}[^}]*}[^}]*}'
```

```console
"coordination":{"schemaVersion":4,"mode":"reservation","capacity":{"cpu":11,"memoryBytes":30064771072},"allocated":{"cpu":2,"memoryBytes":1073741824},"waiting":{"cpu":0,"memoryBytes":0},"activeOwners":1,"waitingOwners":0,"ephemeral":1,"service":0,"transactional":0}
```

There is a lot in that line. Read three things:

- **`capacity`** is now populated: `11` CPU and `30064771072` bytes. That is this machine's 12-way
  parallelism minus one safety unit, and 28 GiB — its 32 GiB of memory minus the balanced profile's
  4 GiB reserve. HIPPO never offers the whole machine.
- **`allocated`** is `2` CPU and `1073741824` bytes, exactly what we asked for. The reservation is
  real and it is recorded.
- **`activeOwners`** is `1`, and `ephemeral` is `1` — one owner, of the default task class.

Your capacity numbers will differ; they are derived from your host.

## Step 5: run a second task alongside the first

The first task is still holding its reservation. Let's start another one that fits in the remaining
budget.

```sh
./hippo run --config local-tmp/tutorial.json --disk-path . \
  --reserve-cpu 1 --reserve-memory-mib 512 -- sh -c 'echo "second owner: cpu=$HIPPO_CONCURRENCY mem=$HIPPO_RESERVED_MEMORY_BYTES"'
```

```console
second owner: cpu=1 mem=536870912
```

This is the whole point of reservation mode. In the older exclusive mode, this second task would have
waited for the first to finish. Here both run at once, because `2 + 1` CPU and `1024 + 512` MiB fit
inside the budget together.

Notice the child received `HIPPO_CONCURRENCY=1` — not the host's core count, and not the first task's
allocation. Each owner is told about its own share.

`HIPPO_RESERVED_MEMORY_BYTES` appears here but did not appear in the first tutorial, because it is
exported in reservation mode only.

## Step 6: ask for something impossible

Now let's ask for more than the machine could ever provide.

```sh
./hippo run --config local-tmp/tutorial.json --disk-path . --reserve-cpu 9999 -- true
echo $?
```

```console
Error: reservation requires replanning: requested vector exceeds safe host capacity
78
```

Exit `78` means _replan_. HIPPO is not saying "busy, try later" — it is saying this request can never
succeed on this host, so waiting would be pointless. A temporarily full budget is a different
situation and gets exit `75`, which **is** worth retrying.

That distinction matters when you script around HIPPO: `75` deserves a retry loop, `78` deserves a
smaller request.

## Step 7: watch the budget come back

Wait for the background task to finish, then look one more time.

```sh
wait
./hippo status --config local-tmp/tutorial.json --json --disk-path . | grep -o '"coordination":{[^}]*}[^}]*}[^}]*}[^}]*}'
```

```console
"coordination":{"schemaVersion":4,"mode":"reservation","capacity":{"cpu":0,"memoryBytes":0},"allocated":{"cpu":0,"memoryBytes":0},"waiting":{"cpu":0,"memoryBytes":0},"activeOwners":0,"waitingOwners":0,"ephemeral":0,"service":0,"transactional":0}
```

Back to an idle epoch. The reservations were released when their owners' process groups fully
retired — not when the guard processes exited, which is a distinction that matters and is explained
in [Process ownership and shedding](../explanation/process-ownership-and-shedding.md).

## Step 8: clean up

```sh
rm -rf "$(dirname "$HIPPO_ROOT")"
rm local-tmp/tutorial.json
unset HIPPO_ROOT
```

## What we did

- Isolated a state root with `HIPPO_ROOT` so the tutorial could not disturb real work.
- Enabled reservation coordination with a two-line schema-2 configuration.
- Held a fixed CPU-and-memory reservation and observed it from a separate HIPPO process.
- Ran two owners concurrently against one shared budget.
- Saw an impossible request rejected with `78` rather than queued.
- Watched capacity return when the owner's process group retired.

Two tasks in one shell is a stand-in for the real case: two checkouts, two terminals, two build
tools, one machine. Because coordination lives in the shared state root and not in either process,
that case works the same way — point both repositories at the same root and they will share the
budget without knowing about each other.

## Next steps

- [The reservation model](../explanation/reservation-model.md) — why admission uses a vector, and why
  the queue is strict FIFO.
- [How to enable reservation coordination](../how-to/enable-reservation-coordination.md) — do this
  permanently for a real repository.
- [How to respond to a HIPPO exit code](../how-to/respond-to-exit-codes.md) — handle `73`, `75`, and
  `78` in scripts.
