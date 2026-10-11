# Bootstrap

Composition builds the CLI application with concrete configuration, host, runtime, health, evidence, and TUI adapters.
It supplies the TUI's terminal capabilities and runner with an application-owned observation callback. Command entry
points use these factories; application and domain code do not import this package.
