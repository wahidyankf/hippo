# Package Wiring Scan Counts Nested Worktrees

The "Every package with tests runs in a gate" scenario counts packages inside nested `worktrees/` checkouts, so the
quick gate fails in any checkout that holds a worktree.

Filed 2026-10-01 under [upstream tool defects](../../../repo-governance/development/upstream-tool-defects.md). The
defect had a workaround and did not block the work in hand, so it is filed here, not fixed.

## Problem and Evidence

**Description.** `packagesWithTests` in `tests/support/gate_packages.go` walks the whole checkout for `_test.go` files.
It skips the directories the go tool skips by name (`.`/`_` prefixes, `testdata`, `node_modules`, `vendor`), but not a
nested module. A task worktree at `worktrees/<task>` carries its own `go.mod`, so the go tool never treats its packages
as part of this module, yet the scenario reports each of them as a package no gate runs. The quick gate is part of
`pre-push`, so every push from a checkout holding a worktree fails, including a push that only deletes a branch.

**Steps to reproduce**, from a clean primary checkout of `main`:

1. `git worktree add worktrees/demo -b worktree/demo origin/main`
2. `sh scripts/test-quick.sh`, or any `git push` from the primary checkout, which runs the same gate in `pre-push`.

**Expected.** The scenario judges this module's packages only, the set `go test ./...` reaches. The
[quality gates](../../../repo-governance/development/quality-gates.md) document scopes the rule to "every package
holding a `_test.go` file" that a gate's `go test` pattern must run; the nested checkout's packages are not this
module's and no pattern here could run them.

**Actual.** `TestUnitBehaviours/Every_package_with_tests_runs_in_a_gate` fails with:

```text
packages whose tests no gate runs: ./worktrees/<task>/internal/cli, ./worktrees/<task>/internal/conformance, ...,
./worktrees/<task>/tests/unit
```

and `pre-push` reports `[gate] quick failed` and `[gate] quick reported a finding at pre-push`.

**Environment.** macOS 15 on arm64, Go 1.26.1, HIPPO `main` at `2d1ab50eb0ec727aa25080f575561232ab65fcc4`, RHINO v0.7.0
as pinned in `rhino.lock`.

**Observed** on 2026-10-01: after #104 merged, `git push origin --delete` of the superseded #102 branch failed twice
from the primary checkout, because another session's task worktree sat under `worktrees/`.

## Why Now

[Worktree location](../../../repo-governance/conventions/worktree-location.md) puts every worktree below the checkout,
and several sessions work in parallel, so a primary checkout with no worktree is the exception. Any push from it fails
until every other session's worktree is gone, and the failure names packages the change never touched.

## Prior Art

Read 2026-10-01:

- **Duplicate search.** `gh issue list --state all` returned no issues at all. Searches of pull requests for "worktrees
  package" (all states) found none, and the only open pull request is #105 (pin RHINO v0.8.0). A search of `plans/` for
  `packagesWithTests`, `gate_packages`, "Every package with tests", and "nested worktree" found nothing in ideas,
  backlog, in-progress, or done. No duplicate exists.
- **Origin.** Commit `1d60ecc` ("test(gates): run every package's tests in the gates") introduced the scan.
- **The go tool's rule.** `go help packages` (Go 1.26.1) lists "Directories that contain a go.mod file" among those a
  `...` pattern skips: "Directories containing other go modules ... can only be matched by changing the working
  directory into module."

## Workaround

Push from a task worktree, the [integration path](../../../repo-governance/conventions/integration-path.md)'s normal
route; it holds no nested worktree. For a branch deletion run from the primary checkout, delete the ref through the
forge, as done on 2026-10-01: `gh api -X DELETE repos/wahidyankf/hippo/git/refs/heads/<branch>`, once the branch's
commits have landed. A deletion publishes no content, so no outbound screen is skipped.

## Proposed Direction

Skip, during the walk, any directory below the root that holds its own `go.mod`, matching the go tool's module boundary,
rather than naming `worktrees/` specially. A regression case would build a temporary root with a nested module holding a
test file and require the scan to leave it out.

## Scope and Non-Goals

In scope: the scan's directory filter and its regression test. Not in scope: where worktrees live, the gate scripts'
patterns, or any other scenario.

## Risks and Open Questions

- Does any other scan (format, lint, line length, artifact policy) also descend into `worktrees/`? Each would fail the
  same way and should be checked when this is planned.
- A nested module this repository intends to gate would now be skipped silently; there is none today.

## Success

The quick gate passes in a primary checkout that holds another session's worktree, and the regression case fails without
the filter. Promote this brief to a bug-fix plan if the failure blocks work with no workaround.
