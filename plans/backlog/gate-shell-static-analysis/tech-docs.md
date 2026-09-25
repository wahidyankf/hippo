# Technical Design: Gate Shell Static Analysis

## Resulting Shape

```text
repo-config.yml  gate shell-lint (pre-push, pull-request, main)
└── scripts/shell-lint.sh
    ├── scripts/shell-files.sh            one list: git ls-files '*.sh' + hippo rhino ferret .husky hooks
    └── scripts/shellcheck.sh             reads shellcheck.lock, verifies the archive digest, then executes
        └── shellcheck --severity=warning <list>
scripts/format-check.sh                   hands the same list to shfmt -d
```

## Design Decisions

- `shellcheck.lock` follows `rhino.lock`: `version=` and one `<platform>=<sha256>` line per release archive.
- `scripts/shellcheck.sh` follows the verify-before-execute contract in `ferret`: Bash, `set -euo pipefail`, cache
  under the ignored `/.cache/`, re-digest on every run, publish by atomic rename, and exit `125` on a malformed pin, a
  digest mismatch, or an unpinned platform.
- `scripts/shell-files.sh` prints the list once, NUL-free and sorted, so `format-check.sh` and `shell-lint.sh` cannot
  drift; `ferret` joins the `shfmt` input as a consequence.
- Fixes: `CDPATH= cd` becomes `CDPATH='' cd` (SC1007); `hippo:107` becomes `rm -rf -- "${platform_cache:?}/$candidate"`
  (SC2115); each `.husky/` hook gains `# shellcheck shell=sh` as its first line (SC2148), matching the directive style
  the public-safety cases already use.
- The gate is a registry entry, not a line in `test-quick.sh`, so `./rhino gate run` dispatches it on every declared
  surface and the CI replay picks it up without a workflow edit.

## Specification Changes

None. No product behaviour changes. AC-01 is proved by `tests/artifacts/shellcheck-pin.sh`, which `tests/artifacts/run.sh` calls, AC-02 and AC-03 by the
gate run and a deliberate mutation, AC-04 by comparing both tools' inputs, and AC-05 by the pull-request replay and the
adapter diff.

## File-Impact Analysis

```text
shellcheck.lock                                                   [N] version and per-platform digests
scripts/shellcheck.sh                                             [N] verifying wrapper
scripts/shell-files.sh                                            [N] the one file list
scripts/shell-lint.sh                                             [N] gate entry
tests/artifacts/shellcheck-pin.sh                                 [N] wrapper refusal test
tests/artifacts/run.sh                                            [E] call the refusal test; SC1007 fix
scripts/format-check.sh                                           [E] read the shared list
repo-config.yml                                                   [E] shell-lint gate entry
hippo, scripts/*.sh, tests/e2e/run.sh                             [E] SC1007 and SC2115 fixes
.husky/commit-msg, .husky/pre-commit, .husky/pre-push             [E] shell directive
repo-governance/development/quality/stacks/repository-adapter.md  [E] gap becomes gate; shellcheck.lock pin
repo-governance/development/quality-gates.md                      [E] name the gate
CHANGELOG.md                                                      [E] only if docs propagation finds `hippo` consumer-visible
plans/backlog/README.md, plans/in-progress/README.md              [E] stage indexes
```

## Dependencies

ShellCheck release archives, pinned by digest. No Go module or npm package.

## Rollback

Revert the delivery commits. The fixes are behaviour-neutral, so reverting only the registry entry also disables the
gate cleanly.
