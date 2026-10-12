# Compiled end-to-end tests

`./tests/e2e/run.sh` builds one stamped current candidate, prepares the host helper and both immutable release binaries,
then runs direct and composed public tests serially. `npm test` invokes this same entrypoint as part of the full gate.
Preparation needs no caller-supplied binary, digest, or receipt.

For this release, the acceptance owner deferred six forced public terminal-fault rows: startup, input read, live output
write, restoration, restoration failure taking precedence over interruption, and global JSON restoration-before-report.
Their three scenario names and concrete boundaries are recorded in [tests/contract/contract.go](../contract/contract.go)
with matching `@e2e-exempt` tags; they are deferred, never counted as PASS. Unit and native-I/O Integration retain every
original predicate, including the six-row/29-step fault selection.

The declared `TestTUIBDDPublicSessionHolderSignals` entry also runs `readonly-output/plain` and
`readonly-output/global-json` against the compiled candidate. Those regressions require exit 125, privacy-safe
plain/JSON diagnostics, joined ownership and complete post-Wait terminal/foreground equality; they do not prove public
late read/write/restore faults, failure precedence or restoration before reporting. Release still requires those
replacement gates and every other mandatory gate to pass.

`hippo-old.lock` pins v0.8.4 and preserves its existing public coordination proof. `hippo-v085.lock` separately pins
v0.8.5, commit `456d24bc2a20ee23a7b81746c1789133cd5c3a97`, and four host archive checksums. Each preparation selects the
current host, acquires the official release archive over HTTPS, checks its pinned checksum before extraction, and
executes that extracted binary's `version --json`. The selected Go tool strictly parses the actual receipt; its schema,
version, and full commit must match the selected immutable release. Both preparations reject unexpected stderr or
altered raw version output.

The gate then executes `TestMixedVersionV085DisplayInteroperability` once with `-tags=mixedversion -count=1`, a
120-second test timeout, and the same current candidate used by the retained v0.8.4 public suite. The v0.8.5 preparation
exports `HIPPO_PUBLIC_V085_BIN`, `HIPPO_PUBLIC_V085_BIN_SHA256`, and `HIPPO_PUBLIC_V085_VERSION_RECEIPT` to the harness.
The existing v0.8.4 inputs remain separate. These artifact inputs never become product environment. A preparation or
compatibility failure stops the gate; no missing-input skip exists.

This v0.8.5 compatibility fixture reserves one CPU and one GiB per real owner: two concurrent owners allocate two CPUs
and two GiB. Its scoped configuration caps memory capacity at three GiB, so the exact oracle also applies on a standard
runner with seven GB RAM after the production safety reserve. Production admission retains one CPU for the host, so the
fixture requires at least three logical CPUs. Its three one-CPU FIFO peers remain genuinely queued behind the existing
two-owner limit throughout display transitions. The 90-second owner context, 120-second leaf timeout, complete metadata
and public-reader assertions, and natural zero retirement checks remain mandatory.

The canonical v0.8.4 public E2E row uses a separate compatibility variant with the same one-CPU owner requests,
three-GiB memory cap and three-CPU host minimum. It retains its original four steps, 20-second journey, three-second
retirement budget, real old/current binaries, current status and TUI assertions, unchanged coordination schemas, FIFO
vectors and natural retirement with joined exit 0. The default native mixed fixture separately keeps two CPUs per owner,
its five-CPU host minimum and eight-GiB memory cap. These scoped compatibility budgets do not change production
reservation policy or either suite's selectors.

For offline acquisition, set `HIPPO_E2E_OLD_ARCHIVE` and `HIPPO_E2E_V085_ARCHIVE` to absolute regular archive paths for
the matching host and release. Each goes through the same checksum, member, and executable identity checks:

```sh
HIPPO_E2E_OLD_ARCHIVE=/absolute/hippo_v0.8.4_darwin_arm64.tar.gz \
HIPPO_E2E_V085_ARCHIVE=/absolute/hippo_v0.8.5_darwin_arm64.tar.gz \
./tests/e2e/run.sh
```

Omitting either archive variable downloads that release from its official immutable URL. An empty, relative, missing, or
symlink archive path fails. Inherited binary, digest, receipt, current candidate, or alternate pin cannot replace either
release archive. Supplied archives remain untouched. Every outcome removes owned current and release artifacts through
the E2E gate trap. The gate caller admits and finitely supervises preparation and test processes; each download has its
own 120-second bound.

PS-startup and post-query stack diagnostics require their exact task selectors. They are outside permanent acceptance
routes; a diagnostic pass does not prove an original public scenario.
