# Internal Packages

HIPPO's command implementations live here. The [as-built architecture](../specs/architecture.md) describes their
responsibilities and runtime boundaries. These packages use Go's
[internal package layout](https://go.dev/doc/modules/layout#package-or-command-with-supporting-packages) to keep
implementation APIs private to this module.

For a guarded invocation, [bootstrap](bootstrap/bootstrap.go) supplies concrete adapters,
[the CLI boundary](adapters/cli/development.go) projects arguments into a request, and
[run preparation](application/run_entry.go) calls the guarded-run use case. Application code owns workflow order;
adapters perform filesystem, host, process, and presentation effects. Pure packages supply values and decisions without
importing adapters or bootstrap.

## Directory Map

- [adapters/](adapters/README.md) — configuration, host, runtime, evidence, health, CLI, and conformance effects.
- [application/](application/README.md) — execution, observation, release, and conformance use cases and their ports.
- [bootstrap/](bootstrap/README.md) — concrete adapter composition for command entry points.
- [domain/](domain/README.md) — pure coordination reducers and evidence aggregation.
- [identity/](identity/) — strict decoding and validation of privacy-safe run identity values.
- [policy/](policy/) — host sample values, capacity profiles, and admission decisions.
- [status/](status/) — named failures, exit statuses, and portable interruption values.
