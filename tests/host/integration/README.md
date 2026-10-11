# Host Tooling Boundary Tests

These tests build the evidence verifier and run its CLI and the Bash runner with private temporary Git/source/config
fixtures and native-command stand-ins. They cover argument/status propagation, fresh candidate and ownership binding,
ordered native drive, bounded failure retention, capture-bound observations, complete synthetic finalization, changed
configuration, strict JSON, and reported viewport rejection. They never start tmux, Neovim, or WezTerm.

Run `go test -count=1 ./tests/host/integration` with the pinned Go environment. Actual tmux and WezTerm input, capture,
navigation, fault classification, pending-refresh timing, and terminal restoration remain separate required runtime
checks. Neovim shortcut compatibility follows the [documentation and configuration audit](../README.md).
