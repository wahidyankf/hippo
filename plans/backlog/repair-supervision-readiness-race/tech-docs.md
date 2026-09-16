# Technical Design: Repair Supervision Readiness Race

## Current Sequence

```text
fixture starts guard
├── guarded shell starts asynchronously
│   └── writes child.pid when scheduled
└── sequenceCollector reaches failureFrom by sample count
    └── guard returns injected failure
        └── fixture immediately reads child.pid  <-- race
```

[Repo-grounded] `tests/support/driver.go::loseHostEvidence` currently uses `failureFrom: 3`, a no-op injected sleep, and
an immediate `os.ReadFile(pidPath)` after `guard.Run`. None of those establishes that the child wrote the file.

## Resulting Sequence

```text
fixture starts guard
├── guarded shell deliberately delays, then writes child.pid
└── sequenceCollector reaches failureFrom
    ├── boundedly waits for the child-owned PID marker
    ├── returns an explicit readiness error if it never appears
    └── otherwise injects the existing host-evidence failure
        └── existing reaping and lease assertions run unchanged
```

## Design Decisions

- Synchronize on the existing child-written PID path, not a test-authored marker.
- Place the barrier immediately before the injected collector error, so readiness orders the two relevant events.
- Add an optional `beforeFailure func() error` hook to `sequenceCollector`; `nil` preserves every existing caller, and
  the failure branch returns a hook error before it can return `injected host evidence failure`.
- In `loseHostEvidence`, configure that hook to call the existing bounded `awaitMarkerFile` behavior for `pidPath` and
  return `guarded child PID was not ready before host evidence failure` when the bound expires. Do not introduce a
  second polling implementation.
- Keep `specs/behaviours/execution.feature` unchanged because its observable behavior is already correct.
- Change the guarded shell to
  `trap '' TERM; sleep 0.05; printf '%s' "$$" > "$GUARD_CHILD_PID"; while :; do sleep 1; done`; retain that delay after
  GREEN so the regression continues to exercise the repaired ordering.

## Specification Changes

No durable product-specification change is required:

- AC-02 preserves the existing `specs/behaviours/execution.feature` scenario
  `A supervision failure reaps the guarded child before releasing ownership`; its unit binding and existing
  integration/end-to-end exemptions remain unchanged.
- AC-01 stays plan-level because readiness orders the fixture's Given rather than changing product behavior. The Phase
  1 deterministic RED, GREEN focused unit command, and twenty-run stress task prove it.
- AC-03 stays plan-level because test-only scope and repeatability are delivery properties. The Phase 1 diff gate and
  Phase 2 `npm run test:quick` plus `npm test` tasks prove it.
- AC-04 stays plan-level because the timeout is a test-support diagnostic. The Phase 1
  `TestSequenceCollectorRunsBeforeFailureHook` and `TestSequenceCollectorReturnsReadinessErrorWithinBound` tests prove
  hook ordering and bounded failure through `go test -count=1 ./tests/support`.

## File-Impact Analysis

```text
specs/behaviours/execution.feature                                      [G] Existing behavior remains unchanged
tests/support/driver.go                                                 [E] Deterministic delay and readiness barrier
tests/support/driver_test.go                                            [N] Named hook-ordering and bounded-error proofs
tests/support/review_v04.go                                             [G] Existing bounded marker-wait reference
plans/backlog/repair-supervision-readiness-race/README.md               [E] Lifecycle status during execution
plans/backlog/README.md                                                 [E] Backlog index before activation
plans/in-progress/README.md                                             [E] Active index during execution
```

### More Detail

Call the existing helper without editing `review_v04.go`. Keep the new optional hook private to `driver.go`; test it
from the same `support` package. Do not edit `internal/guard/`, `internal/policy/`, `internal/reservation/`, or any
public documentation.

## Dependencies

No new Go module, npm package, service, or operating-system tool is required. The implementation uses existing test
support, `os.Stat`/`os.ReadFile`, and bounded `time` operations.

## Rollback

Revert the test-support commit and its plan progress. The Gherkin corpus and product tree are unchanged, so no runtime
or consumer rollback is required.
