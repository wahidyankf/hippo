# Delivery: Isolate Test Coordination State

> **Legend** — `[AI]`: an agent performs the step. `[HUMAN]`: only a human can perform it because of unavailable
> credentials, physical action, external authority, or an unresolved decision. Split mixed work into separate items.

## Execution Checkout

Repository path: the primary checkout of this repository.

Worktree path: `worktrees/isolate-test-coordination-state/` below it.

Delivery mode: `worktree-to-pr`

Branch: `worktree/isolate-test-coordination-state`

```bash
test "$(git branch --show-current)" = main
test -z "$(git status --porcelain)"
git fetch origin --prune
git merge --ff-only origin/main
git worktree add worktrees/isolate-test-coordination-state \
  -b worktree/isolate-test-coordination-state origin/main
cd worktrees/isolate-test-coordination-state
npm ci
```

The repository gates run directly, never beneath `./hippo`. Only the AC-04 reproduction runs beneath it, because the
defect is what that wrapper exports.

## Delivery Unit

One branch and pull request deliver one test-only outcome: every test package that starts the product owns its
coordination state. Rollback reverts `tests/support/isolation.go`, `tests/support/isolation_test.go`, the four
`main_test.go` files, and the plan record together.

The worktree-to-PR workflow owns commit, push, draft, exact-head quality, leak review, rebase merge, main
reconciliation, and cleanup. Fix every gate failure at its cause; never bypass a hook.

Owner decision, 2026-09-26: the plan is archived inside this delivery pull request rather than through a second one.
Marking the pull request ready, the exact-head leak review, the rebase merge, the primary reconciliation, the complete
gate on reconciled `main`, and the clean-up all follow the archival commit, so they cannot be ticked here; the
worktree-to-PR workflow performs them and the pull request records their proof. See the
[quality gate](evidence/quality-gate.md).

## Phase 0: Environment Setup and Baseline

- [x] `[AI]` Provision the declared worktree with the commands above; acceptance: it is registered once at
      `origin/main` and `git status --porcelain` is empty. `[AC-05]`
  - Result: registered once at `837ad5f`; `npm ci` ran beneath the worktree-local guard; the porcelain status was empty.
- [x] `[AI]` Run `npm run test:quick`; acceptance: the baseline exits `0` without retry. `[AC-05]`
  - Result: exit `0` at `837ad5f`, run directly, with only the staged plan activation in the tree.
- [x] `[AI]` Move the plan to `plans/in-progress/isolate-test-coordination-state/` with `git mv` and update both stage
      indexes; acceptance: one in-progress copy exists and only the in-progress index links it. `[AC-05]`
  - Result: moved with `git mv`; the backlog index lost its entry and the in-progress index gained it.
- [x] `[AI]` Check the plan against every rule in `repo-governance/conventions/plans/006-structural-validation.md` and run `./rhino md internal-link validate`; acceptance: no rule fails and the link check exits `0`. `[AC-05]`
  - Result: 0 structural findings over the whole `plans/` tree; the link check exited `0`. The quality gate returned
    `PASS_WITH_FINDINGS` with four repaired findings; see [the record](evidence/quality-gate.md).

### Phase 0 Gate

- [x] `[AI]` Run `git status --short` and record the result here; acceptance: only plan-activation paths differ.
      `[AC-05]`
  - Result: only the plan's rename into `plans/in-progress/`, its new `evidence/` records, and the two stage indexes.

> **Pause Safety**: a clean worktree holds the active plan. Safe to stop. To resume: `./rhino md internal-link validate`.

## Phase 1: Isolation Cycle

- [x] `[AI]` **RED**: run `./hippo run --class ephemeral --resource-tier standard --disk-path . -- ./tests/e2e/run.sh`;
      acceptance: it fails as the README table records, and the failing scenarios and assertion identifiers are
      recorded under this item. `[AC-04]`
  - Result: exit `1`; 31 scenarios, 27 passed and 4 failed, as the README's first row records:
    `Reservation coordination rejects every compatibility class as a protocol mismatch` ("ephemeral never reached the
    compatibility check in 40 attempts; last exit was 2: hippo.args.invalid schema 3 requires --resource-tier"), both
    `A child-owned reserved exit stays a child failure` examples ("compiled child exit 124/125 was not admitted after
    40 attempts"), and `Interactive guarded child owns the terminal while it runs` ("compiled PTY guard failed:
    hippo.args.invalid"). `TestCommandLineInterfaceContract` passed in this run.
- [x] `[AI]` **RED**: run the complete gate beneath the guard with the shared root exported,
      `HIPPO_ROOT="$HOME/Library/Application Support/hippo" ./hippo run --class ephemeral --resource-tier heavy --disk-path . -- npm test`
      (the `XDG_STATE_HOME` or home-relative default on Linux); acceptance: it fails, and the failing packages and
      assertions are recorded under this item. `[AC-04]`
  - Result: exit `1`. The quick gate, `./tests/integration`, and the unit behaviours passed; `./tests/e2e` failed the
    same four scenarios with the same messages, and `scripts/test.sh` stopped there. A direct `npm test` from a shell
    exporting only `HIPPO_CONFIG` failed the same four, and with that variable unset it failed
    `cli.args.double-dash-ends-options` with exit `125` instead, recorded by the fixture-identifier plan's execution.
- [x] `[AI]` **RED**: add `tests/support/isolation_test.go` with `TestRunIsolatedRemovesInheritedCoordination`,
      `TestRunIsolatedKeepsHarnessInputs` (all five allowlisted inputs), and
      `TestRunIsolatedLeavesMarkedHelpersUnchanged`, driving the helper
      through a re-executed test binary; run `go test -count=1 ./tests/support`; acceptance: the package fails to
      compile or the three tests fail because `RunIsolated` does not exist. `[AC-01]` `[AC-02]` `[AC-03]`
  - Result: exit `1`, `tests/support/isolation_test.go:33:18: undefined: support.RunIsolated`, build failed.
- [x] `[AI]` **GREEN**: add `tests/support/isolation.go` as `tech-docs.md` specifies; run
      `go test -count=1 ./tests/support`; acceptance: the three tests pass. `[AC-01]` `[AC-02]` `[AC-03]`
  - Result: exit `0`; the three tests passed and the probe skipped when not re-executed. Two deliberate mutations bit:
    removing `HIPPO_LOAD_SATURATED` from the allowlist failed `TestRunIsolatedKeepsHarnessInputs`, and disabling the
    marker check failed `TestRunIsolatedLeavesMarkedHelpersUnchanged` on all four helper values.
- [x] `[AI]` Add `./tests/support` to the unit line of `scripts/test-quick.sh`; acceptance: `npm run test:quick` runs
      the three helper tests. `[AC-01]` `[AC-02]` `[AC-03]`
  - Result: the quick gate inside the guarded complete-gate GREEN reported `ok github.com/wahidyankf/hippo/tests/support`.
- [x] `[AI]` **GREEN**: add `main_test.go` with `os.Exit(support.RunIsolated(m))` to `tests/unit`, `tests/integration`,
      `tests/bdd`, and `tests/e2e`; re-run the Phase 1 RED reproduction; acceptance: it exits `0` with every `HIPPO_*`
      variable still exported by the wrapper. `[AC-04]`
  - Result: exit `0`, `ok github.com/wahidyankf/hippo/tests/e2e`, beneath the same guard and the same inherited
    variables as the RED.
- [x] `[AI]` **GREEN**: re-run the guarded complete-gate RED command; acceptance: it exits `0` without retry, with
      every `HIPPO_*` variable and the shared `HIPPO_ROOT` still exported. `[AC-04]`
  - Result: exit `0` on the first admitted run; the guard deferred it six times at the `heavy` tier before admitting it
    (never started, requeued unchanged). Every package passed, including `./tests/e2e`, the race run, and
    `govulncheck`.
- [x] `[AI]` **REFACTOR**: keep the allowlist and marker in one place, run
      `go tool golangci-lint fmt --diff` and `git diff --check`; acceptance: both exit `0` and the four `TestMain`
      bodies are one line each. `[AC-01]` `[AC-05]`
  - Result: the allowlist and the marker live only in `tests/support/isolation.go`; `go tool golangci-lint fmt --diff`
    and `git diff --check` exited `0`; each `TestMain` is one line. `golangci-lint run` flagged two probe paths as
    `G703`, each now carrying a specific, explained `nolint`, and then reported `0 issues`.

### Phase 1 Gate

- [x] `[AI]` Run `git diff --name-only origin/main`; acceptance: only the paths in the file-impact analysis differ.
      `[AC-05]`
  - Result: against the plan's base `837ad5f`, the six test files, `scripts/test-quick.sh`, the plan folder with its
    `evidence/`, and the two stage indexes; nothing else.

> **Pause Safety**: the guarded reproduction passes. Safe to stop. To resume: `go test -count=1 ./tests/support`.

## Phase 2: Repository Gates and Delivery

- [x] `[AI]` Run docs propagation for the change and record whether `end-to-end-testing.md` gains its sentence;
      acceptance: the decision and any edit are recorded here. `[AC-05]`
  - Result: `no-change`. Searching `README.md`, `docs/`, `specs/`, and `CHANGELOG.md` for `npm test`, `test:quick`,
    `HIPPO_ROOT`, `tests/support`, and `TestMain` found no stale fact: the README's gate descriptions stay true, and
    every `HIPPO_ROOT` mention describes the product. `end-to-end-testing.md` gains no sentence: it is governance, the
    change alters no rule, and the helper's own comment is the one home of the isolated default.
- [x] `[AI]` Run `npm run test:quick`; acceptance: exit `0`. `[AC-05]`
  - Result: exit `0`, as the first stage of the direct complete gate below, which calls the same
    `scripts/test-quick.sh`; `./tests/support` ran in it.
- [x] `[AI]` Run `npm test`; acceptance: exit `0` without retry. `[AC-04]` `[AC-05]`
  - Result: exit `0` on the first run, directly, from a shell that still exports `HIPPO_CONFIG` and sets no `HIPPO_ROOT`
    — the condition under which the complete gate failed before this change. `govulncheck` reported no called
    vulnerability.
- [x] `[AI]` Inspect the diff and the proposed commit and PR text against data safety, then commit with a Conventional
      Commit; acceptance: hooks pass and the commit holds only declared paths. `[AC-05]`
  - Result: `test(support): give each test package coordination state of its own` holds the six test files and
    `scripts/test-quick.sh`; the plan records are their own commit. Every added line and both messages screened clean,
    and the `public-safety-tree`, `format-staged`, `public-safety-message`, and `commit-message` hooks passed. The
    branch was rebased onto `main` twice before its first push, as the shell static-analysis and fixture-identifier
    plans landed; only the stage indexes conflicted, and `shell-lint` passed on the edited gate script.
- [x] `[AI]` Push and open the pull request as a draft with a screened body recording every RED and GREEN;
      acceptance: the draft exists at the pushed head. `[AC-05]`
  - Result: the pre-push gate passed, `shell-lint` included; draft pull request #72 opened at `ce8ce7e` after the
    outbound preflight reported the title and body clean.

### Phase 2 Gate

- [x] `[AI]` Read the pull-request quality gate on the pushed head; acceptance: every check passes. `[AC-04]` `[AC-05]`
  - Result: all seven checks passed on `ce8ce7e`, `Quality gate` included, on both `ubuntu-24.04` and `macos-15`.

> **Pause Safety**: the draft is green. Safe to stop. To resume: `gh pr checks` on the draft, no faster than every three
> minutes.

## Phase 3: Execution Review and Knowledge Capture

- [x] `[AI]` Run `repo-governance/workflows/plan-execution-check.md` against AC-01 to AC-05 and record its verdict;
      acceptance: `PASS`. `[AC-05]`
  - Result: `PASS`; see [the record](evidence/execution-check.md).
- [x] `[AI]` Route every `learnings.md` entry to one durable owner or discard it with a reason, filing code follow-ups
      as backlog plans; acceptance: no unresolved entry remains. `[AC-05]`
  - Result: L1 promoted to a code comment, L2 to the quick gate, L3 and L4 discarded with reasons; no follow-up plan
    was needed.

### Phase 3 Gate

- [x] `[AI]` Confirm no product, specification, or public-contract path changed; acceptance: the review permits
      archival. `[AC-05]`
  - Result: outside `plans/`, only `tests/` and `scripts/test-quick.sh` changed; nothing under `cmd/`, `internal/`, or
    `specs/`. The review permits archival.

> **Pause Safety**: substantive work is terminal. Safe to stop. To resume: `git log --oneline origin/main..HEAD`.

## Plan Archival

- [x] `[AI]` Move the plan to `plans/done/YYYY-MM-DD__isolate-test-coordination-state/` with the completion date and
      update both stage indexes; acceptance: one done copy exists. `[AC-05]`
  - Result: moved with `git mv` to `plans/done/2026-09-26__isolate-test-coordination-state/`, after confirming the
    destination did not exist; the in-progress index is empty again and the done index lists the plan.
- [x] `[AI]` Re-check the archived plan against `006-structural-validation.md` and run `npm run test:quick`; acceptance: no rule fails and the gate exits `0`. `[AC-05]`
  - Result: 0 structural findings; the internal-link and directory-map checks reported no findings; the quick gate
    exited `0` with `./tests/support` in it.
