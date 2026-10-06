---
description: >-
  Records which stack packs this repository adopted, the decisions their standards leave open, and every deviation,
  including the shared standards kept under their local owners.
when_to_use: >-
  Use when working in a project here, adopting or retiring a stack pack, or changing a recorded stack decision.
---

# Repository Adapter

This document owns the `extensions.software-development` inventory in [`repo-config.yml`](../../../../repo-config.yml).
A stronger local rule wins over an adopted one, and every difference is recorded here with its reason.

## Adopted Packs

| Pack     | Status  | Reason                                                                                        |
| -------- | ------- | --------------------------------------------------------------------------------------------- |
| `golang` | adapted | stronger local gates are kept, and the test layout and context rule deviate as recorded below |
| `shell`  | adapted | two declared interpreters, as recorded below                                                  |

## Shared Standards

Adopted as written: [Type and Boundary Safety](../code/type-and-boundary-safety.md),
[Shell Scripts](../code/shell-scripts.md), [Lint Strictness](../checks/lint-strictness.md),
[Meaningful Coverage](../testing/meaningful-coverage.md), and
[Stack Packs](../../../conventions/structure/stack-packs.md).

Kept under local owners, linked wherever the catalog linked its own:
[test-driven development](../../test-driven-development.md) and
[red, green, refactor](../../../workflows/quality/red-green-refactor.md),
[behaviour-driven development](../../behaviour-driven-development.md),
[end-to-end testing](../../end-to-end-testing.md), [quality gates](../../quality-gates.md),
[software quality enforcement](../../software-quality-enforcement.md),
[specification maintenance](../../specification-maintenance.md), [public contract](../../public-contract.md),
[code clarity](../../code-clarity.md), [dependency selection](../../dependency-selection.md), and
[docs propagation](../../../workflows/quality/docs-propagation.md). One rule lives in one document here, per
[rules](../../../conventions/rules.md).

Not adopted:

- **Test Boundaries and Gates.** Its fast gate runs no integration or end-to-end suite; the stricter quick gate here
  runs all three behaviour adapters, per [quality gates](../../quality-gates.md).
- **Test Doubles, Test Data Isolation, and Git Fixture Isolation.** Each rests on owners absent here — hexagonal ports,
  browser and identity fixtures, and a six-layer fixture Git rule the existing fixtures do not yet meet — each awaits
  adoption with any fixture repair.
- **A human reference page under `docs/`.**
  [Documentation architecture](../../../conventions/documentation-architecture.md) keeps contributor rules out of
  `docs/`; the [stack index](README.md) serves instead.

A catalog owner with no local counterpart is named in adopted text, unlinked.

## Adopter Decisions

Grouped by the source that leaves the choice open; each entry reads decision: choice — reason.

- `swe-architect`
  - ADR location: `docs/explanation/decisions/`, the default
- `swe-developer`
  - Stack skills: read on demand — a new stack needs no edit
  - Host-integrated proof: not required, the default
- `swe-reviewer`
  - Reviewer output: inline — the copy keeps `read-only`
  - Specification completeness: not checked, the default
  - Test boundary: local — [quality gates](../../quality-gates.md) and
    [behaviour-driven development](../../behaviour-driven-development.md)
- `swe-releaser`
  - Deploy targets: none; releases follow [release cut](../../../workflows/maintenance/release-cut.md)
- **test-driven development**
  - coverage floor: 99% over the deterministic core, in the unit run — local floor; integration and end-to-end prove
    boundaries instead
- **quality gates**
  - task runner: plain scripts behind `npm run test:quick` and `npm test` — this repository never acquires Nx
- `golang-standards.md`
  - assertions: the `testing` package only — no assertion dependency
  - integration selection: a separate test directory per boundary under `tests/` — the layer is visible by path
  - unit test placement: deviation: most unit tests live in `tests/unit` — one corpus runs through three boundary
    adapters
  - race detection: deviation: the full gate's race pass, not the coverage run — keeps the quick gate fast enough never
    to be bypassed
  - gates: every golangci-lint linter, NilAway, and the domain literal analysis — stronger than the standard, per
    [Go Analysis Gates](repository-adapter/001-go-analysis-gates.md)
  - context parameter: deviation: `noctx` disabled — host probes and supervised process groups have no request context
    to propagate
- `shell-scripts.md`
  - interpreter: deviation: POSIX `sh` with `set -eu`; Bash only in the adopted scanner and record check, the
    pinned-tool wrappers `rhino`, `ferret`, and `scripts/shellcheck.sh`, and the gate scripts `format-staged.sh` and
    `check-commit-message.sh` — the bootstrap wrappers run before any toolchain on macOS and Linux; both stay as adopted
- `shell-standards.md`
  - static analysis: the `shell-lint` gate in [quality gates](../../quality-gates.md) — one file list keeps the analyser
    and the formatter from drifting; the pin keeps a green result meaning the same on every machine
  - test tool: plain shell runners and the behaviour corpus — no shell test dependency to pin

## Skill Names

- `cutting-releases` is [release-cut](../../../../.agents/skills/release-cut/SKILL.md).

## Project Applicability

- [hippo](../../../../README.md) — the CLI and `cmd/hippo-conformance`, one module; the root README names its commands.
- [public-safety](../../../../scripts/public-safety/README.md) — the adopted outbound scanner and its case suite.
- repo-scripts — `scripts/`, `tests/artifacts/`, the `./hippo`, `./rhino`, and `./ferret` wrappers, and the Git hooks.
  It has no README: [quality gates](../../quality-gates.md) names the commands, `tests/artifacts/run.sh` and the
  behaviour corpus test it, and it carries no coverage, per Meaningful Coverage.

## Version Sources

- `golang`: `go.mod`
- `shell`: `go.mod` (`shfmt`) and `shellcheck.lock` (ShellCheck)
