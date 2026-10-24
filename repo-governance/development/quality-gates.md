# Quality Gates

## Locally

Hooks run `./rhino gate run --surface <hook>`; [`repo-config.yml`](../../repo-config.yml) declares their gates;
`./rhino gate list` lists them.

- **`commit-msg`**: public-safety message screen, then `scripts/check-commit-message.sh` commitlint: Conventional
  Commits on every commit.
- **`pre-commit`**: public-safety tree screen, then `scripts/format-staged.sh`: staged Go `goimports`/`gofumpt`, shell
  `shfmt`, JSON/Markdown/YAML Prettier.
- **`pre-push`**: public-safety range screen per commit, `shell-lint`, quick gate, repository configuration and
  documentation gates, unguarded.
- **`shell-lint`**: pre-push/PR replay runs checksum-pinned ShellCheck `--severity=warning` on every
  `scripts/shell-files.sh` entry; formatting uses that same list for `shfmt`.

`npm ci` installs hooks. Unrun hooks mean unverified pushes: [integration path](../conventions/integration-path.md).

## The Quick Gate

`scripts/test-quick.sh` must retain this order: layout, formatting, whole-module compilation, strict lint, NilAway,
import graph/negative fixtures, package tests, deterministic coverage, serial compliance, artifact policy. Retain all
cmd/internal/support/coverage/host tests, contract self-tests and `tests/bdd` negatives.

Ordinary support `TestMain` uses `RunCompiled`: it keeps an inherited `HIPPO_BIN` carrying the test stamp, or builds a
temporary candidate from the current tree and removes it afterwards. This setup also runs when `-run '^$'` selects no
tests. These private fixture controls follow [E2E placement](end-to-end-testing.md#placement); acquiring their candidate
does not execute the canonical E2E corpus.

Ordinary selection excludes four [BDD suite host](behaviour-driven-development.md#suite-host) entries and exactly five
compiled-only entries: `TestTUIBDDPublicCanonicalSuite`, `TestTUIBDDPublicObservationCanonical`,
`TestTUIBDDPublicSessionHolderSignals`, `TestTUIBDDPublicPSHelperStartupDiagnostic`,
`TestTUIBDDPublicStatusPostQueryStackDiagnostic`. Anchor exact names; execute other direct TUI tests. Coverage retains
`TestUnitBehaviours`, excludes the other eight entries and runs relocated Unit once alongside existing `tests/unit` and
pure-package selectors. Retain `-coverpkg` denominator, leaf selectors, exclusions, 99% floor and executing domain
literal analysis.

Run full-registry compliance serially, Unit then Integration then E2E:

```sh
HIPPO_BDD_ADAPTER=unit go test -count=1 -run '^TestFeatureCompliance$' ./internal/adapters/tui
HIPPO_BDD_ADAPTER=integration go test -count=1 -run '^TestFeatureCompliance$' ./internal/adapters/tui
HIPPO_BDD_ADAPTER=e2e go test -count=1 -run '^TestFeatureCompliance$' ./internal/adapters/tui
```

Static compliance does not execute scenarios or start a compiled candidate. Mode/selector registrations and
discriminating fixtures exist; execution and final acceptance remain open, with existing obligations until replacements
execute. See [analysis adoption](quality/stacks/repository-adapter/001-go-analysis-gates.md); documentation hygiene runs
separately on the same surfaces.

Every `_test.go` package must execute via a `go test` pattern in `scripts/test-quick.sh`, `scripts/test.sh`, or
`tests/e2e/run.sh`; compile-only `-run '^$'` does not count. "Every package with tests runs in a gate" keeps that
inventory complete. Cheapest failures precede coverage to avoid wasted runs.

## The Full Gate

`scripts/test.sh` must retain existing `tests/integration` plus relocated `TestIntegrationBehaviours`, explicitly
`HIPPO_BDD_SUITE=integration`. `tests/e2e/run.sh` must retain direct E2E plus relocated `TestE2EBehaviours`, explicitly
`HIPPO_BDD_SUITE=e2e`, against one temporary stamped candidate. Run `TestTUIBDDPublicCanonicalSuite`,
`TestTUIBDDPublicObservationCanonical`, `TestTUIBDDPublicSessionHolderSignals` serially with explicit E2E suite/adapter,
that candidate and prepared helper. Each entry runs once, without skips; retain the full corpus and every non-exempt
canonical public row. Owner-approved terminal-fault E2E deferrals in [BDD](behaviour-driven-development.md#suite-host)
retain six rows/29 steps at Unit/Integration. `TestTUIBDDPublicSessionHolderSignals` also requires compiled
`readonly-output/plain` and `readonly-output/global-json` regressions alongside every other gate.

[Immutable old-release preparation](../../tests/e2e/README.md) precedes public execution. PS-startup/post-query
diagnostics use exact task selection and satisfy no acceptance scenario.

Race retains cmd/internal/support/unit/integration patterns, adds contract self-tests/relocated Unit and explicitly
selects relocated Integration. Retain `govulncheck`. Selector fixtures must fail on removal of any host, mode, coverage,
Integration/E2E, race or serial compliance registration. Preserve host/artifact/release inventory. Full runs after merge
on `main`: see [In CI](#in-ci).

The [terminal contract](../conventions/command-line-interface/007-terminal-and-environment.md) requires genuine PTY
lifecycle and separate native host proof; fake callbacks do not establish terminal restoration.

Both gates retain Go's default ten-minute per-package idle-machine timeout. Loaded `tests/unit` has exceeded seven
minutes: raise it locally, e.g. `GOFLAGS=-timeout=30m`. `scripts/test-loaded.sh` instead uses `HIPPO_LOAD_TEST_TIMEOUT`,
default 60 minutes. This operator choice has no gate check.

Reviewers compare this document, BDD owner and actual scripts to distinguish static compliance from executing suites; no
summary-text check enforces that distinction.

## In CI

The pull-request gate, `pr-quality-gate.yml`, mirrors the hooks and nothing heavier: formatting, the repository contract
(`quick`: typecheck, lint, unit), the pinned RHINO bootstrap, and the release build, plus one bounded released-binary
leaf. The full gate runs after merge on `main`, in `main-test.yml`. A failure is fixed forward or reverted before any
release cut. Job detail: [CI placement](quality-gates/001-ci-placement.md).

## Never Nx

Never adopt Nx. `npm run test:quick` and `npm test` directly invoke shell scripts. Repair failure causes:
[push hook verification](../conventions/push-hook-verification.md).
