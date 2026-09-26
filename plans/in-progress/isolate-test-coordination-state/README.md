# Isolate Test Coordination State

Status: In progress

## Context

[Repo-grounded] The test suite hands its own environment to the binary under test. `tests/e2e/cli_contract_test.go`
runs the built executable with the inherited process environment, and `tests/support/driver.go::environmentWith`,
`tests/support/pending_v04.go`, and the integration tests start from `os.Environ()` and override only `HIPPO_ROOT`
where a scenario chose to. Every other `HIPPO_*` variable in the invoking shell reaches the product, and a run without
`HIPPO_ROOT` resolves the user's default evidence root through `internal/host/runtime.go::DefaultEvidenceRoot` — the
same shared coordination state every guarded process on that machine uses.

[Repo-grounded] Found during a cross-repository standards adoption, where `npm test` failed on a workstation and
passed in CI. Reproduced on 2026-09-26 at `d1bbf41` by running `./tests/e2e/run.sh` beneath `./hippo run`, which
exports `HIPPO_CONFIG`, `HIPPO_DEFAULT_CONFIG`, `HIPPO_DEFAULT_IDENTITY`, `HIPPO_SESSION`, `HIPPO_PROFILE`,
`HIPPO_CONCURRENCY`, `HIPPO_RESERVED_MEMORY_BYTES`, and `HIPPO_BIN` to its child:

| Invoking environment                                   | Result                                                                                                                                                                    |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| everything inherited                                   | four behaviour scenarios fail: `hippo.args.invalid` "schema 3 requires --resource-tier" (exit `2`), and "compiled child exit 124/125 was not admitted after 40 attempts"  |
| `HIPPO_CONFIG` and `HIPPO_DEFAULT_CONFIG` unset only   | all 31 scenarios pass; `TestCommandLineInterfaceContract` fails `cli.args.double-dash-ends-options` and `cli.exit.child-status-passes-through`, each observing exit `125` |
| every `HIPPO_*` unset, `HIPPO_ROOT` an empty directory | `ok github.com/wahidyankf/hippo/tests/e2e`                                                                                                                                |

The original report also named `hippo.coordination.protocol-mismatch` as the reason behind the `125`, and a refused
admission under the `local-constrained` profile when the machine was loaded. Both are the same defect: the working-tree
binary joined coordination state owned by a different, installed release.

## Decision

[Judgment call] The suite isolates itself by default. Each test package that starts the product installs one shared
`TestMain` that removes every inherited `HIPPO_*` variable except the harness's own inputs, points `HIPPO_ROOT` at a
fresh per-run directory, and marks the environment so that a helper process the tests re-execute keeps what its parent
set.

Rejected alternatives:

- scrub only in `scripts/test.sh` and `scripts/test-quick.sh` — a bare `go test ./tests/e2e` would still join the
  shared state;
- patch each `append(os.Environ(), ...)` call site — more than twenty sites, and the next one added reopens the leak;
- a named denylist of product variables — the product can read a new variable before anyone updates the list; the
  existing scrub in `internal/conformance/conformance.go` already lists six and omits `HIPPO_CONFIG`.

## Decision Gate Record

- Filed on 2026-09-26 as a knowledge-capture follow-up.
- Activated on 2026-09-26: the owner approved the decision above and directed execution now, with the complete gate
  proved beneath the guard before and after, and the plan archived inside its delivery pull request rather than through
  a second one. The [quality gate](evidence/quality-gate.md) records the resulting repairs.

## Scope

In scope: one isolation helper in `tests/support/`, a `TestMain` in `tests/unit`, `tests/integration`, `tests/bdd`, and
`tests/e2e`, unit proof of the helper, a guarded process-boundary regression, and the repository gates.

Out of scope: product code, the Gherkin corpus, the public contract, the scrub inside `internal/conformance`, and the
guard exemption in [resource-aware development](../../../repo-governance/development/resource-aware-development.md).

## Approach Summary

1. Prove the helper's contract test-first in `tests/support`.
2. Install it in the four test packages and re-run the recorded reproduction until it passes.
3. Run the repository gates and deliver one pull request.

## Dependencies

- [Repo-grounded] `npm run test:quick` is the pre-push gate and `npm test` the complete gate.
- [Repo-grounded] HIPPO does not guard its own gates; only the regression reproduction runs beneath `./hippo run`,
  because the defect is what that wrapper exports.

## Directory Map

- [Business requirements](brd.md)
- [Product requirements and acceptance criteria](prd.md)
- [Technical design](tech-docs.md)
- [Delivery checklist](delivery.md)
- [Execution learnings](learnings.md)
- [Evidence](evidence/README.md)
