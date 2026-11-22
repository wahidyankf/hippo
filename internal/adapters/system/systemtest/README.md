# internal/adapters/system/systemtest

In-memory doubles for the ports in `internal/adapters/system`. `NewHost` builds a complete host, and `Port` returns the
`system.System` value that a unit test passes to an adapter.

## Runner Captures

`Host.Runner` answers `CaptureOutput` calls by command name, and records each one:

- `Stdout`: the standard output a captured command returns, keyed by command name.
- `Failures`: the error a captured command returns, keyed by command name. A failure wins over `Stdout`.
- A command with neither fails with `exec.ErrNotFound`.
- `Captures`: every call in call order, each a `Capture` with the command `Name` and its `Arguments`.

## Why This Package Is Excluded From Coverage

It is test support: the doubles exist only to stand in for the host in unit tests. The coverage gate measures production
packages, and `systemtest` is named as test-support in `scripts/test-quick.sh`, so its statements are left out of the
measured set. Its own tests (`doubles_test.go`) still run in the unit run and prove the doubles behave as documented.

## Who May Import It

Only `_test.go` files and the `tests/` tree. The `depguard` rule `production-without-systemtest` in `.golangci.yml`
rejects an import from any other production file, and `tests/architecture/imports_test.go` checks the same edge.

## Limits

- Links are not modelled: `Link`, `Symlink`, and `Readlink` fail, and `EvalSymlinks` fails for a link entry, so a test
  that needs a link must use the real tree in the integration suite.
- `SameFile` compares the name and the `Sys` value of two entries. A test that replaces an entry gives it another `Sys`.
- `Start` and `Wait` fail with an error, because a double runs no process.
- Parents are not checked on create, and modification times are zero.
- A `Deliver` into a full channel blocks, so the test must size the channel.
