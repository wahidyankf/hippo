# How to monitor a release

`hippo release` captures resource and responsiveness evidence across a deployment window and decides
whether that evidence stayed inside the release envelope.

Release monitoring is strict: it requires explicit local health and routed endpoints, and it will not
guess them.

## Gate a release before starting

```sh
hippo release check --disk-path /path/to/deployment
```

Silent success means the host has the headroom to proceed. A stable exit code means it does not.

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

Both `--health-url` and `--routed-origin` are mandatory:

```console
$ hippo release monitor --output samples.jsonl --summary summary.json --deployment-root .
Error: HTTP(S) health URL is required for release monitoring

$ hippo release monitor ... --health-url http://127.0.0.1:8080/health
Error: bare HTTPS routed origin is required for release monitoring
```

`--service-port` is repeatable and selects which listeners count toward RSS accounting.

Without `--duration-ms`, monitoring continues until cancellation. A positive value ends the capture
after that many milliseconds — useful in CI where the window is known.

## Assess the result

```sh
hippo release assess --summary summary.json
```

```console
{"accepted":true,"schemaVersion":5}
```

Rejected evidence returns exit `75` and says why:

```console
{"accepted":false,"schemaVersion":5}
release overlap exhausted resource or routed responsiveness headroom
```

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
Error: raw evidence and summary cannot both use standard output
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
