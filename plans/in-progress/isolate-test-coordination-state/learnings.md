# Learnings: isolate-test-coordination-state

<!-- Append observations during execution. Resolve every entry before archival. -->

## L1 — A harness input the plan did not name

`scripts/test-loaded.sh` exports `HIPPO_LOAD_SATURATED`, and `tests/support/pending_v04.go` reads it from the test
process. The filed allowlist would have removed it and made the loaded gate reject the deferral it documents as correct.

**Disposition:** promoted to a code comment. `harnessInputs` in `tests/support/isolation.go` names what each allowlisted
variable is for, beside the list a future input must join, and `TestRunIsolatedKeepsHarnessInputs` fails if it is
dropped.

## L2 — The helper's proofs would have run in no gate

`scripts/test-quick.sh` runs one list of unit packages and only compiles the rest, and `scripts/test.sh` adds process
boundaries only, so a test added to `tests/support` was never executed by CI.

**Disposition:** promoted to a test boundary. `./tests/support` joins the quick gate's unit line, so the pre-push hook
and the pull-request gate both run the three proofs.

## L3 — The guard deferred a lint run on a loaded workstation

A guarded `golangci-lint` run returned `hippo.limit.capacity-deferred` twice while the host reported a memory warning,
and ran once requeued unchanged.

**Disposition:** discarded as the product working. A never-started deferral is requeued as the public contract says; no
rule or test is missing.
