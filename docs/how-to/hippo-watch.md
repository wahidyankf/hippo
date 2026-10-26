# How to keep a live view with the `hippo_watch` recipe

HIPPO ships no interactive dashboard. The same data the dashboard used to show — running owners, queue positions, and
machine capacity — comes out of `hippo status --json` and `hippo watch --json`, so a few lines of shell give you a live
view tailored to your terminal. Copy the `hippo_watch` function below into your shell profile and run `hippo_watch`.

The recipe assumes `jq` is installed, as in [How to watch host pressure](./watch-host-pressure.md). Every field it reads
is part of the stable schema-5 projection described in [JSON schemas](../reference/json-schemas.md).

```sh
# Live one-line summary: resource state, owner count, and allocated CPU.
hippo_watch() {
  while true; do
    clear
    hippo status --json --disk-path . | jq -r \
      '"state=\(.resource.state) owners=\((.coordination.owners // []) | length) cpu=\(.coordination.allocated.cpu)"'
    sleep 5
  done
}
```

For owner and queue rows instead of the summary, print the running table each refresh:

```sh
hippo_watch_rows() {
  while true; do
    clear
    hippo status --json --disk-path . | jq -r \
      '(.coordination.owners // [])[] | "\(.runId) \(.class) \(.tier // "-") \(.requested.cpu)c"'
    sleep 5
  done
}
```

To follow transitions as they happen rather than polling, stream `hippo watch --json`, which emits one schema-5 object
per changed snapshot:

```sh
hippo watch --json --disk-path . --interval 5s | jq -r \
  '"\(.measuredAt) \(.resource.state) owners=\((.coordination.owners // []) | length)"'
```

Quit any of these with Ctrl-C. Nothing here creates a job or reservation, and stopping the view never touches guarded
work. `run --label` still attaches a private display label to a run for API consumers; labels never appear in status or
watch output, so the recipe output is comparable before and after a label change.
