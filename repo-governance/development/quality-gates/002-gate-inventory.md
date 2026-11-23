# Gate Inventory

This is the one inventory of the checks the repository runs, as rule C19 of the
[Test and Gate Contract](../quality/checks/test-and-gate-contract.md) requires. Each row is an entry of `gates.entries`
in `repo-config.yml`, listed under the surface it runs on. Scope follows rule C7. Kind is the kind of work.

## Pre-Commit, Commit-Message, and Pre-Push

| Entry               | Surface    | Scope      | Kind       | What it checks              | Why on this surface       |
| ------------------- | ---------- | ---------- | ---------- | --------------------------- | ------------------------- |
| format-staged       | pre-commit | per file   | formatting | formatters on staged files  | C6: rewrites and restages |
| commit-message      | commit-msg | per commit | lint       | Conventional Commits format | C10: lint at commit time  |
| public-safety-range | pre-push   | global     | leak       | leak scan of pushed commits | C11: leaks leave on push  |

## Pull Request

| Entry                   | Scope      | Kind        | What it checks                 | Why on this surface       |
| ----------------------- | ---------- | ----------- | ------------------------------ | ------------------------- |
| public-safety-tree      | global     | leak        | leak scan of tracked files     | C11: the whole tree       |
| public-safety-range     | global     | leak        | leak scan of the review range  | C11: the range, replayed  |
| public-safety-message   | global     | leak        | leak scan of commit messages   | C11: messages in range    |
| public-safety-tests     | global     | unit        | scanner regression cases       | isolated checkout needed  |
| format-staged           | per file   | formatting  | formatter replay, verify clean | C8: replays the hook      |
| commit-message          | per commit | lint        | Conventional Commits format    | C10: lint, second run     |
| shell-lint              | global     | lint        | ShellCheck on every shell file | C6: a hook may not lint   |
| remote-dispatch-refusal | global     | structure   | remote dispatch refusal text   | C6: a document check      |
| quick                   | global     | unit        | lint, NilAway, unit, coverage  | C6: tests never in a hook |
| markdown-line-length    | global     | document    | Markdown lines, 120 columns    | C6: a document check      |
| repo-config             | global     | structure   | repo-config.yml schema         | C6: a configuration check |
| word-budget             | global     | document    | word limits of governed files  | C6: a document check      |
| quality-gates           | global     | structure   | quality-gate headings          | C6: a document check      |
| directory-map           | global     | structure   | README listings, mapped trees  | C7: moves orphan entries  |
| harness-adapters        | global     | parity      | canon adapters, shell tests    | C7: one source feeds all  |
| internal-links          | global     | document    | Markdown links resolve in repo | C7: a deleted target      |
| mermaid                 | global     | document    | Mermaid label and colour rules | C23: diagram validator    |
| worktree-layout         | global     | structure   | worktree dir, .gitignore       | C6: a repository check    |
| format-check            | global     | formatting  | Prettier, Go fmt, shfmt        | C8: the formatter replay  |
| hook-transport-tests    | global     | unit        | hook and guard tests           | C6: tests of the hooks    |
| architecture-tests      | global     | unit        | architecture checks, go test   | C12: a unit-level Go test |
| contract-tests          | global     | unit        | contract package, with -race   | C12: a unit-level Go test |
| artifact-tests          | global     | structure   | release and artifact policy    | C6: a repository check    |
| gate-wiring             | global     | structure   | registry, hooks, workflows     | C1, C2: W01 to W07        |
| test-integration        | global     | integration | integration suites with -race  | C17: every pull request   |

## Main

| Entry            | Scope  | Kind        | What it checks               | Why on this surface       |
| ---------------- | ------ | ----------- | ---------------------------- | ------------------------- |
| quick            | global | unit        | same checks as pull request  | repeated on Linux, macOS  |
| test-integration | global | integration | same suites as pull request  | repeated on Linux, macOS  |
| test-e2e         | global | e2e         | compiled end-to-end suites   | C17: not per pull request |
| vuln-scan        | global | security    | govulncheck, all Go packages | needs the network         |

## Workflow-Only Checks

These run in workflows and are not registry entries, because their input is not a repository gate input.

| Job                     | Trigger      | Kind      | What it checks                | Why here                    |
| ----------------------- | ------------ | --------- | ----------------------------- | --------------------------- |
| Pinned RHINO bootstrap  | pull request | structure | pinned release resolves       | boundary change only        |
| Release build           | PR and main  | build     | release assets and checksums  | a tag is permanent          |
| Test (macos-15)         | main         | mixed     | the main surface on macOS     | only darwin host code proof |
| Release reproducibility | main         | build     | macOS rebuild, same sums      | archives depend on commit   |
| Loaded (both systems)   | nightly      | mixed     | targets under synthetic load  | not required; a finding     |
| Release                 | version tag  | structure | tag on main, passing main run | refuses an untested tag     |
| Quality gate            | pull request | structure | fails unless all jobs pass    | the required check (C22)    |
