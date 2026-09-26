# Quality Gate: Isolate Test Coordination State

## Record

- **Workflow:** [plan quality gate](../../../../repo-governance/workflows/plan-quality-gate.md).
- **Frozen snapshot:** the plan as filed at `837ad5f`.
- **Run:** 2026-09-26T01:07Z.
- **Structural validation:** every check in
  [structural validation](../../../../repo-governance/conventions/plans/006-structural-validation.md), applied to the
  whole `plans/` tree after activation; 0 findings before and after repair. `./rhino md internal-link validate` exited
  `0`.
- **Verdict:** `PASS_WITH_FINDINGS`. All four findings were repaired in one cycle of the two allowed; none remains
  open.

## Findings

1. **The allowlist would break the loaded gate.** `scripts/test-loaded.sh` exports `HIPPO_LOAD_SATURATED`, and
   `tests/support/pending_v04.go` reads it from the test process to accept only the documented capacity deferral on a
   saturated host. The filed allowlist named four inputs and would have removed it. Repair: it joins the allowlist in
   `tech-docs.md`, AC-02, the business risks, and the named harness-input test.
2. **The proof did not cover the complete gate.** The owner directed on 2026-09-26 that `npm test` itself be proved
   beneath the guard with the guard's variables and the shared evidence root inherited, before and after the change.
   The filed plan reproduced only `./tests/e2e/run.sh`. Repair: Phase 1 carries a guarded complete-gate RED and GREEN
   under AC-04.
3. **Archival route conflicts with the owner's direction.** The filed plan archived through a second pull request and
   ticked the merge, reconciliation, and clean-up inside the plan. The owner directed that the plan be archived inside
   its delivery pull request. Repair: the execution review runs on the pushed draft, the archival move follows it in the
   same pull request, and every step after the archival commit is stated as owned by the worktree-to-PR workflow and
   proved on the pull request.

4. **The helper's proofs would run in no gate.** `scripts/test-quick.sh` runs `./tests/unit` and compiles every other
   package with `-run '^$'`, and `scripts/test.sh` adds only process-boundary packages, so the filed
   `go test -count=1 ./tests/support` proof of AC-01 to AC-03 would never run in CI. Repair: `./tests/support` joins
   the quick gate's unit line, and `tech-docs.md` lists the edit.

## Checks That Found Nothing

- Every re-executed helper in the four packages starts from `os.Environ()`, as does the product-side conformance
  runner, so the marker reaches each of them.
- No package under `tests/` defines a `TestMain` today, so each new `main_test.go` is the package's only one.
- The acceptance criteria are testable at the named boundaries: AC-01 to AC-03 in `tests/support`, AC-04 at the
  guarded process boundary, AC-05 by the diff and the gates.
