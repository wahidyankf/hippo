# internal/adapters/system

The operating-system ports the adapters reach the host through. The adapters take a value of these types instead of
calling `os`, `syscall`, `time`, or `os/exec` directly, so a unit test can pass an in-memory double.

## Contents

- `ports.go`: the port types (`FileSystem`, `Clock`, `SignalSource`, `ProcessTable`, `ProcessRunner`, `Environment`,
  `TokenSource`, `DiskStats`), the `System` value that groups them, and `Production`, which binds every field to the
  standard library.
- `ports_test.go`: fails when a `Production` field is unbound.
- `systemtest/`: the in-memory doubles. See its own README.

## Rules

- Each port field has the signature of the standard-library function it is bound to. `Production` assigns the function
  itself, so the binding holds no statement and the unit coverage floor is unaffected.
- A field needs an adapter only when no standard-library function has its signature. Such an adapter is a statement, so
  add it with the package that needs it and record its coverage there.
- Errno classification stays in the adapter, which reads the error the port returns.
- Only the adapters and the composition root may import this package. The packages under `internal/application` and the
  pure packages never do (see `tests/architecture/imports_test.go`).

## Deferred Shapes

These ports need a handle a double can control, so they are not in `ports.go` yet. The package that first needs one adds
it with its binding and its double, and records the choice in its evidence.

- `File` (interface, without `Fd()`), `Locker` (`flock`, `fstat`), `Terminal` (`ioctl`, `getpgrp`), and `DirectoryTree`
  (`*at` calls) take a file or descriptor the double must stand in for.
- `Clock` timers and tickers (`NewTimer`, `NewTicker`) return `*time.Timer` and `*time.Ticker`, whose channels a double
  cannot drive.
- `ProcessRunner` start, wait, and pipes need a child handle. Output of a command that runs to completion is the
  `CaptureOutput` field, which needs no handle.
- `DiskStats` `statfs` returns a platform-specific struct.
