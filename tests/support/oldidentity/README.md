# Immutable old-release identity verifier

This E2E harness utility reads the actual old binary's retained `version --json` stdout with Go's standard JSON parser.
It requires schema 1, version v0.8.4, and full commit abde2ecbeff7513549cfd1c0805fd20b5d232c01, and rejects invalid,
missing, unknown or trailing data. It accepts JSON whitespace and key order independently of the shell's additional
exact authentic receipt comparison.

Old-release preparation invokes it through the already-required selected Go tool before any public scenario executes. It
changes no product command, host collector or public frame protocol. Its deterministic tests execute in the ordinary
quick and full race gates. A parsed identity alone does not prove release provenance; acquisition first authenticates
the archive with the pinned platform checksum.
