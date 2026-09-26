---
description: >-
  Records which stack packs this repository adopted, the decisions their standards leave open, and every deviation,
  including the shared standards kept under their local owners.
when_to_use: >-
  Use when working in a project here, adopting or retiring a stack pack, or changing a recorded stack decision.
---

# Repository Adapter

This document owns the `extensions.software-development` inventory in [`repo-config.yml`](../../../../repo-config.yml).
A stronger local rule always wins over an adopted one, and every difference is recorded here with its reason.

## Adopted Packs

| Pack     | Status  | Reason                                                                                        |
| -------- | ------- | --------------------------------------------------------------------------------------------- |
| `golang` | adapted | stronger local gates are kept, and the test layout and context rule deviate as recorded below |
| `shell`  | adapted | two declared interpreters, as recorded below                                                  |

## Shared Standards

Adopted as written: [Type and Boundary Safety](../code/type-and-boundary-safety.md),
[Shell Scripts](../code/shell-scripts.md), [Lint Strictness](../checks/lint-strictness.md),
[Meaningful Coverage](../testing/meaningful-coverage.md), and [Stack Packs](../../../conventions/structure/stack-packs.md).

Kept under their local owners, which an adopted document links wherever the catalog linked its own:
[test-driven development](../../test-driven-development.md) and [red, green, refactor](../../../workflows/red-green-refactor.md),
[behaviour-driven development](../../behaviour-driven-development.md), [end-to-end testing](../../end-to-end-testing.md),
[quality gates](../../quality-gates.md), [software quality enforcement](../../software-quality-enforcement.md),
[specification maintenance](../../specification-maintenance.md), [public contract](../../public-contract.md),
[code clarity](../../code-clarity.md), [dependency selection](../../dependency-selection.md), and
[docs propagation](../../../workflows/docs-propagation.md). One rule lives in one document here, per
[rules](../../../conventions/rules.md).

Not adopted, each with its reason:

- **Test Boundaries and Gates.** Its fast gate never runs an integration or end-to-end suite; the quick gate here runs
  all three behaviour adapters by design, per [quality gates](../../quality-gates.md). The local rule is stricter.
- **Test Doubles, Test Data Isolation, and Git Fixture Isolation.** Each rests on owners this repository does not hold —
  hexagonal ports, browser and identity fixtures, and a six-layer rule for every fixture Git call that the existing
  fixtures do not yet meet. Each waits for its own adoption together with any fixture repair it needs.
- **A human reference page under `docs/`.** [Documentation architecture](../../../conventions/documentation-architecture.md)
  keeps contributor rules out of `docs/`; the [stack index](README.md) serves instead.

A catalog owner with no local counterpart stays named in the adopted text without a link.

## Adopter Decisions

| Source                  | Decision              | Choice                                                                                                                                                                                                                 | Reason                                                                                                                      |
| ----------------------- | --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `swe-code-maker`        | stack skill loading   | read on demand                                                                                                                                                                                                         | a new stack needs no agent edit                                                                                             |
| `swe-code-checker`      | stack rules           | the adopted catalog standards                                                                                                                                                                                          | shared, reviewed choices                                                                                                    |
| test-driven development | coverage floor        | 99% over the deterministic core, in the unit run                                                                                                                                                                       | local floor; integration and end-to-end prove boundaries instead                                                            |
| quality gates           | task runner           | plain scripts behind `npm run test:quick` and `npm test`                                                                                                                                                               | this repository never acquires Nx                                                                                           |
| `golang-standards.md`   | assertions            | the `testing` package only                                                                                                                                                                                             | no assertion dependency                                                                                                     |
| `golang-standards.md`   | integration selection | a separate test directory per boundary under `tests/`                                                                                                                                                                  | the layer is visible by path                                                                                                |
| `golang-standards.md`   | unit test placement   | deviation: most unit tests live in `tests/unit`                                                                                                                                                                        | one corpus runs through three boundary adapters                                                                             |
| `golang-standards.md`   | race detection        | deviation: the full gate's race pass, not the coverage run                                                                                                                                                             | the quick gate stays fast enough never to be bypassed                                                                       |
| `golang-standards.md`   | gates                 | `gofumpt` and `goimports`; golangci-lint with every linter enabled                                                                                                                                                     | stronger than the standard; each disabled linter carries its reason in `.golangci.yml`                                      |
| `golang-standards.md`   | context parameter     | deviation: `noctx` disabled                                                                                                                                                                                            | host probes and supervised process groups have no request context to propagate                                              |
| `shell-scripts.md`      | interpreter           | deviation: POSIX `sh` with `set -eu`; Bash only in the adopted scanner, the pinned-tool wrappers `rhino`, `ferret`, and `scripts/shellcheck.sh`, and the gate scripts `format-staged.sh` and `check-commit-message.sh` | the bootstrap wrappers run before any toolchain on macOS and Linux; the scanner stays as adopted                            |
| `shell-standards.md`    | static analysis       | the `shell-lint` gate: pinned ShellCheck at `--severity=warning` over `scripts/shell-files.sh`, the list `shfmt -d` also reads                                                                                         | one list keeps the analyser and the formatter from drifting; the pin keeps a green result meaning the same on every machine |
| `shell-standards.md`    | test tool             | plain shell runners and the behaviour corpus                                                                                                                                                                           | no shell test dependency to pin                                                                                             |

## Project Applicability

- [hippo](../../../../README.md) — the CLI, and `cmd/hippo-conformance` built from the same module; the root README
  names its commands.
- [public-safety](../../../../scripts/public-safety/README.md) — the adopted outbound scanner and its case suite.
- repo-scripts — `scripts/`, `tests/artifacts/`, the `./hippo`, `./rhino`, and `./ferret` wrappers, and the Git hooks.
  It has no README: [quality gates](../../quality-gates.md) names the commands, `tests/artifacts/run.sh` and the
  behaviour corpus test it, and it carries no coverage, per Meaningful Coverage.

## Version Sources

- `golang`: `go.mod`
- `shell`: `go.mod` (`shfmt`) and `shellcheck.lock` (ShellCheck)
