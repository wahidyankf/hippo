# Gate Shell Static Analysis

Status: Backlog

## Context

[Repo-grounded] [Shell standards](../../../repo-governance/development/quality/stacks/shell-standards.md) require
every script to be statically analysed at the [lint strictness](../../../repo-governance/development/quality/checks/lint-strictness.md)
threshold — warning and above fails. The
[repository adapter](../../../repo-governance/development/quality/stacks/repository-adapter.md) records the shell stack
as `adapted` with the gap "static analysis: not yet a gate; `shfmt` formats every script". Only `shfmt` runs, from
`scripts/format-check.sh` and `scripts/format-staged.sh`; no hook, registry entry, or workflow runs an analyser. Found
during a cross-repository standards adoption.

[Repo-grounded] ShellCheck 0.11.0 at `--severity=warning`, run on 2026-09-26 at `d1bbf41` over every tracked `*.sh`
file plus `hippo`, `rhino`, `ferret`, and the three `.husky/` hooks, reports 14 findings:

| Rule   | Count | Where                                                                                                                                                                                                                                        | Cause                                                         |
| ------ | ----- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| SC1007 | 10    | `hippo:4`, `scripts/build-release.sh:18,55`, `scripts/check-worktree-layout.sh:4`, `scripts/format-check.sh:4`, `scripts/test-loaded.sh:4`, `scripts/test-quick.sh:4`, `scripts/test.sh:4`, `tests/artifacts/run.sh:4`, `tests/e2e/run.sh:4` | the `CDPATH= cd` idiom                                        |
| SC2115 | 1     | `hippo:107`                                                                                                                                                                                                                                  | `rm -rf -- "$platform_cache/$candidate"` without a `:?` guard |
| SC2148 | 3     | `.husky/commit-msg`, `.husky/pre-commit`, `.husky/pre-push`                                                                                                                                                                                  | no shebang or `shell` directive                               |

Below the threshold it also prints five SC2329 notes and one SC2016 note, all in `scripts/public-safety/`, whose
README states those adopted copies are already clean at `--severity=warning`. `rhino` and `ferret` report nothing.
`scripts/format-check.sh` hands `shfmt` the entrypoints `hippo`, `rhino`, and the hooks, but not `ferret`.

## Decision

[Judgment call] Clean, then gate: fix the 14 findings, then register one `shell-lint` check in `repo-config.yml` on
`pre-push`, `pull-request`, and `main`, running a checksum-pinned ShellCheck at `--severity=warning` over one
enumerated file list that `format-check.sh` shares.

Rejected alternatives:

- ShellCheck from `PATH` or the runner image — a floating version, which
  [dependency selection](../../../repo-governance/development/dependency-selection.md) rejects;
- an npm wrapper package — it downloads the analyser outside any digest this repository holds;
- suppressing SC1007 — the idiom rewrites to `CDPATH='' cd` with identical behaviour, so a fix is cheaper than a waiver.

## Decision Gate Record

- Filed on 2026-09-26 as a knowledge-capture follow-up. No owner gate has been held; activation needs the owner's
  approval of the decision above.

## Scope

In scope: the 14 fixes, the pin and its verifying wrapper, the shared file list, the registry entry, and the adapter and
quality-gate documentation that stop describing a gap.

Out of scope: notes below the threshold, edits to the adopted `scripts/public-safety/` copies, rewriting scripts in
another language, and changes to `shfmt` settings.

## Approach Summary

1. Pin ShellCheck and add its verifying wrapper, following the `rhino.lock` shape.
2. Fix the findings, proving each cause with a mutation.
3. Register the gate, update the documentation, and deliver one pull request.

## Dependencies

- [Repo-grounded] `repo-config.yml` gates run through `./rhino gate run`; the `repository-contract` CI job replays the
  `pull-request` surface, so no workflow edit is needed.
- [Judgment call] ShellCheck publishes per-platform release archives for macOS and Linux on arm64 and x86-64; confirm
  the asset names and digests upstream at execution.

## Directory Map

- [Business requirements](brd.md)
- [Product requirements and acceptance criteria](prd.md)
- [Technical design](tech-docs.md)
- [Delivery checklist](delivery.md)
- [Execution learnings](learnings.md)
