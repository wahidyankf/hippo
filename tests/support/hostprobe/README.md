# External host-query test helper

This directory contains seven files:

- `protocol.go` defines the nonce-bound query protocol and exact seven-command whitelist.
- `client.go` exchanges real query output, acknowledgments and completion with the owning fixture.
- `helper_test.go` provides the explicitly marked external collector entry.
- `protocol_test.go` checks refusal, output ordering and connection cleanup.
- `protocol_internal_test.go` provides a test-only bridge to the private `exchangeAndClose` function for external tests.
- `prepare.sh` supports external race-helper preparation and validates selected artifacts and dependency closure.
- `README.md` describes this directory's scope and recovery state.

At recovery time, the four original Go sources and preparation script were restored from exact recorded source hashes.
This README is a factual replacement; its original bytes were not recovered. `protocol_internal_test.go` is a later
test-only addition. The helper is separate from the selected HIPPO product binary. Its preparation routine requires the
existing admitted finite gate caller; it does not supervise a whole group itself.

At recovery time, no build, test or native verification had run for this restored source layer. Exact source recovery
does not establish that its current helper artifact, parent fixtures or public TUI scenarios pass.
