# Resource Guard

Resource Guard is a standalone Go CLI that admits, supervises, and sheds local development work from host resource evidence. It supports macOS and Linux, coordinates heavy work across repositories through a shared per-user lease, and only signals the child process group it created.

## Install

Download a tagged archive for `darwin` or `linux` on `amd64` or `arm64`, then verify it against the release `checksums.txt`. Consumers should pin both the tag and the expected SHA-256; they must not follow `main` at runtime.

Source users can run the tracked bootstrap, which builds and retains a bounded local cache:

```sh
./resource-guard version --json
./resource-guard status --json --disk-path .
./resource-guard run --class ephemeral --disk-path . -- <command>
```

## Resource policy

Ordinary work resolves `balanced` → `constrained` → `minimal` from effective memory, available memory, disk, CPU, and swap capability. Balanced ephemeral work on Darwin may admit after a full stable warning window when 25% of effective memory, clamped to 4–8 GiB, remains available and CPU, disk, OOM, swap-out, and compressor-growth checks remain safe. This degraded path forces known tool concurrency variables to one. Services, fallback profiles, Linux PSI, transactions, and releases cannot use it.

Exit `73` requires storage cleanup. Exit `75` is retryable capacity pressure or a held heavy-work lease. Exit `78` requires configuration or strict-profile replanning. Never bypass the guard or change task class to obtain admission.

## Configuration and state

Copy [`resource-guard.local.json.example`](resource-guard.local.json.example) to ignored `resource-guard.local.json`. `--config` overrides `RESOURCE_GUARD_CONFIG`, which overrides the bootstrap default. Local configuration can make policy stricter but cannot weaken compiled floors.

`RESOURCE_GUARD_ROOT` overrides the shared evidence and lease root. Defaults are `~/Library/Application Support/resource-guard` on macOS and `${XDG_STATE_HOME:-$HOME/.local/state}/resource-guard` on Linux. Samples expire after seven days, summaries after thirty days, and files remain bounded. Evidence never records command arguments, origins, paths, credentials, or user data.

Runtime integration uses `RESOURCE_GUARD_ROOT`, `RESOURCE_GUARD_SESSION`, `RESOURCE_GUARD_BIN`, `RESOURCE_GUARD_BUILD_CACHE`, `RESOURCE_GUARD_HEALTH_URL`, and `RESOURCE_GUARD_ROUTED_ORIGIN`.

## Release monitoring

Strict release monitoring requires explicit local health and routed endpoints:

```sh
./resource-guard release monitor \
  --output samples.jsonl \
  --summary summary.json \
  --deployment-root /path/to/deployment \
  --health-url http://127.0.0.1:8080/health/ready \
  --routed-origin https://service.example \
  --service-port 8080 --service-port 8081
```

New summaries use schema 5 with generic health fields. Assessment remains compatible with retained schema 2–4 summaries.

## Development

The executable specification lives in [`specs/behaviours`](specs/behaviours/README.md). Its shared contract enforces strict unit, integration, and compiled-binary E2E adapters, reviewed exemptions, and complete step resolution.

```sh
./scripts/test-quick.sh
./scripts/test.sh
go tool govulncheck ./...
```

Release artifacts are built with `./scripts/build-release.sh <version> <commit> <output-dir>`. Generated outputs and local configuration are enforced by the artifact suite and `.gitignore`.

## License

Resource Guard is available under the [MIT License](LICENSE).
