# Public Contract

What consumers depend on, and what may therefore not move without an explicitly authorized
pre-stable minor release or, after stability, a major release.

## The Contract

- **Exit statuses.** `0` and `1` are a result, affirmative and negative. `2` is an unusable
  invocation. `124` is a limit stopping the work. `125` is HIPPO unable to do its job, with nothing
  started. `126` and `127` are a command that cannot be executed and one that is not there. Nothing
  else is returned except a started child's own status, or `128+N` for a signal. A guarded child's
  status passes through unchanged, including one that collides with a status HIPPO also uses;
  HIPPO's own failures always write a `hippo:` line to stderr and a child's never does.
- **Error codes.** Every failure names one `hippo.area.reason` from the published vocabulary, on
  stderr and, with `--output json`, as `error.code`. The status says what a shell should do; the
  code says what happened.
- **Evidence readers.** The supported readers and their record shapes. A consumer parsing evidence is a consumer whose parser breaks when the shape does.
- **Configuration compatibility.** Existing keys keep their meaning. A breaking transition requires the owner's explicit authorization, not a judgement that the old shape was worse.
- **The command surface.** Command paths, flag names, and the exit-code-to-condition mapping, which is what a caller branches on.

## Why Few Statuses and Many Codes

A new meaning wedged into an existing status costs every consumer its ability to branch on it, and
the obvious escape — a new number per condition — costs more. HIPPO v0.7.0 took that escape and
added `76`; by v0.8.0 there were four such numbers, all inside the range a child may return, so the
number alone never said who chose it, and the `./hippo` bootstrap that nine repositories share
reached `exit 78` from one helper in sixteen call sites without ever asking which reason produced
it. A vocabulary nobody reads is not information.

So the two questions are answered in two places. The status stays small enough to learn, and uses
the numbers `timeout` and every POSIX shell already assign to these situations. The reason has no
such limit, and a new condition gets a new code rather than a new number. Adding a code is not a
breaking change; moving a code to a different status is. While HIPPO remains pre-stable, an
explicitly authorized breaking contract change advances the minor version. See
[minimal sufficiency](../principles/minimal-sufficiency.md).

The `never-started` distinction is load-bearing beyond this repository, and survives the renumbering
inside `124`'s reasons rather than in a status of its own. `state: never-started` means the FIFO
deadline or cancellation happened before launch and the same invocation may be requeued once.
`started-safety-stop`, `pressure-shed`, `storage-shed`, and `started-activation-failure` mean a
payload ran; payload-specific
recovery decides whether retry is safe. Never duplicate a waiter or payload, change its class, or
route around the guard.

## Changing It

A rename is a contract change, not a spelling correction. Where a change is unavoidable, it lands with the specification, the documentation, and the changelog in the same pull request, and the pull-request body says what a consumer must do.
