# Technical Design: Isolate Test Coordination State

## Current Sequence

```text
contributor shell (HIPPO_CONFIG=..., under ./hippo run: HIPPO_SESSION=..., HIPPO_PROFILE=...)
└── go test ./tests/e2e
    └── invoke(binary, ...)                  inherits everything
        ├── reads HIPPO_CONFIG               workstation schema, not the fixture's
        └── no HIPPO_ROOT                    joins the user's default evidence root
            └── live epoch from an installed release -> exit 125
```

## Resulting Sequence

```text
go test ./tests/<package>
└── TestMain -> support.RunIsolated(m)
    ├── marker set?          yes -> m.Run() unchanged (re-executed helper)
    └── no -> unset HIPPO_* except harness inputs
             HIPPO_ROOT=<fresh temporary directory>
             set marker, m.Run(), remove the directory
```

## Design Decisions

- One helper, `tests/support/isolation.go`, exporting `RunIsolated(m *testing.M) int`; each package's `TestMain` is
  `os.Exit(support.RunIsolated(m))`.
- Harness inputs are an explicit allowlist: `HIPPO_BIN`, `HIPPO_BDD_ADAPTER`, `HIPPO_E2E_TEMP_PARENT`,
  `HIPPO_GO_BINARY`. Everything else with the `HIPPO_` prefix is removed, so a variable the product learns to read later
  is isolated without an edit here.
- The marker is `HIPPO_TEST_ISOLATED_ROOT`, holding the per-run root. A child started from `os.Environ()` inherits it,
  so a re-executed helper skips the scrub and keeps flags such as `HIPPO_STALE_RESERVATION_HELPER`.
- Scenarios that already set `HIPPO_ROOT` per scenario keep doing so; the per-run root is only the default.
- `scripts/test.sh` and `scripts/test-quick.sh` do not change: their `HIPPO_BIN` override stays, and every `go test`
  they call now isolates itself.

## Specification Changes

None. The Gherkin corpus describes the product; these criteria describe the harness. AC-01 to AC-03 are proved by
`go test -count=1 ./tests/support`, AC-04 by the guarded `./tests/e2e/run.sh` reproduction, and AC-05 by the diff and
repository gates.

## File-Impact Analysis

```text
tests/support/isolation.go                                   [N] RunIsolated and the allowlist
tests/support/isolation_test.go                              [N] AC-01 to AC-03 proofs
tests/unit/main_test.go                                      [N] TestMain
tests/integration/main_test.go                               [N] TestMain
tests/bdd/main_test.go                                       [N] TestMain
tests/e2e/main_test.go                                       [N] TestMain
internal/conformance/conformance.go                          [G] existing product-side scrub, unchanged
repo-governance/development/end-to-end-testing.md            [E] one sentence on the isolated default, if docs propagation requires it
plans/backlog/README.md, plans/in-progress/README.md         [E] stage indexes
```

## Dependencies

Standard library only: `os`, `strings`, `testing`.

## Rollback

Revert the test-support commit. The product tree is unchanged, so no consumer rollback exists.
