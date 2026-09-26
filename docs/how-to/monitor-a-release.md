# How to monitor a release

`hippo release` captures resource and responsiveness evidence across a deployment window and decides
whether that evidence stayed inside the release envelope.

Release monitoring is strict: it requires explicit local health and routed endpoints, and it will not
guess them.

## Gate a release before starting

```sh
hippo release check --disk-path /path/to/deployment
```

Silent success means the host has the headroom to proceed. A stable exit code means it does not, and
one diagnostic line says why: `124` naming `hippo.limit.capacity-deferred` when memory or CPU did not
settle, so a later retry can pass; `124` naming `hippo.limit.storage-blocked` when the deployment disk
is below the release reserve, so free space first; `125` when host evidence could not be collected.

## Capture evidence during the window

```sh
hippo release monitor \
  --output samples.jsonl \
  --summary summary.json \
  --deployment-root /path/to/deployment \
  --health-url http://127.0.0.1:8080/health/ready \
  --routed-origin https://service.example \
  --service-port 8080 --service-port 8081
```

`--output`, `--summary`, and `--deployment-root` are mandatory, and so are both `--health-url` and
`--routed-origin`:

```console
$ hippo release monitor --health-url http://127.0.0.1:8080/health --routed-origin https://service.example
hippo: [hippo.args.invalid] output, summary, and deployment root are required

$ hippo release monitor --output samples.jsonl --summary summary.json --deployment-root .
hippo: [hippo.args.invalid] HTTP(S) health URL is required for release monitoring

$ hippo release monitor ... --health-url http://127.0.0.1:8080/health
hippo: [hippo.args.invalid] bare HTTPS routed origin is required for release monitoring
```

Each is a usage mistake: it prints the one diagnostic line and exits `2` before any sample is taken or any endpoint is probed.

`--service-port` is repeatable and selects which listeners count toward RSS accounting.

Without `--duration-ms`, monitoring continues until a signal stops it; the summary is still written,
and the command exits `130` for `SIGINT` or `143` for `SIGTERM`. A positive value ends the capture
after that many milliseconds and exits `0` — useful in CI where the window is known, and the form to
use when a script needs `0` from a completed capture.

## Assess the result

```sh
hippo release assess --summary summary.json
```

```console
{"accepted":true,"schemaVersion":5}
```

Rejected evidence returns exit `124`, names `hippo.limit.release-envelope-exceeded`, and says why. Retrying the same
summary gives the same answer:

```console
{"accepted":false,"schemaVersion":5}
hippo: [hippo.limit.release-envelope-exceeded] release evidence rejected: release overlap exhausted resource or routed responsiveness headroom
```

A summary that is missing, truncated, or not a summary prints no verdict and exits `125` naming
`hippo.evidence.unreadable`.

Assessment accepts retained schema 2–5 summaries, so old evidence stays readable after an upgrade.

## Stream instead of writing files

Either destination may use the Unix `-` convention, but **not both in one invocation** — raw and
summary schemas must never be mixed on one stream.

Stream raw samples, keep the summary as a file:

```sh
hippo release monitor \
  --output - --summary summary.json \
  --deployment-root /path/to/deployment \
  --health-url http://127.0.0.1:8080/health/ready \
  --routed-origin https://service.example | jq -c .
```

Keep rotating raw evidence, pipe the summary straight into assessment:

```sh
hippo release monitor \
  --output samples.jsonl --summary - \
  --deployment-root /path/to/deployment \
  --health-url http://127.0.0.1:8080/health/ready \
  --routed-origin https://service.example |
  hippo release assess --summary -
```

Asking for both at once fails immediately:

```console
$ hippo release monitor --output - --summary - ...
hippo: [hippo.args.invalid] raw evidence and summary cannot both use standard output
```

File output remains exclusive, private, rotating, and retention-managed. Standard output is
caller-owned: HIPPO neither closes nor retains it, applies normal pipe backpressure, and returns a
failure if the downstream writer fails. Diagnostics stay on stderr.

## Read the evidence

Each raw JSONL record embeds a complete host sample plus six release-specific fields:

```json
{
  "oneMinuteLoad": 4.79,
  "serviceRssBytes": 22511616,
  "healthStatus": 200,
  "healthLatencyMs": 0.916,
  "routedJourneyStatus": 200,
  "routedJourneyLatencyMs": 106.944
}
```

The current raw chunk stays at the requested `--output` path; older chunks use numbered suffixes.

The schema-5 summary aggregates the whole window — including latency percentiles and failure counts —
and stays complete even after older raw chunks have rotated away. See
[JSON schemas](../reference/json-schemas.md#release-summary).

## Set endpoints from the environment

`HIPPO_HEALTH_URL` and `HIPPO_ROUTED_ORIGIN` supply defaults for `--health-url` and
`--routed-origin`, which keeps long deployment scripts readable:

```sh
export HIPPO_HEALTH_URL=http://127.0.0.1:8080/health/ready
export HIPPO_ROUTED_ORIGIN=https://service.example

hippo release monitor --output samples.jsonl --summary summary.json \
  --deployment-root /path/to/deployment --duration-ms 60000
```

## Related

- [Command-line interface](../reference/cli.md#hippo-release-monitor)
- [JSON schemas](../reference/json-schemas.md)
