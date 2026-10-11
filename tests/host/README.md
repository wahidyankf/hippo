# TUI Host Evidence

`tui-sequence.sh` prepares, drives, captures, and finalizes evidence for a task-owned TUI host sequence.
`evidence-verifier/` checks that evidence against the current source, binary, and configuration. The tooling tests use
synthetic documents and native-command stand-ins. They prove format, ordering, binding, and rejection behavior; actual
host acceptance requires separate trusted tmux and WezTerm runs and visual inspection. Neovim shortcut compatibility is
audited against wkf-devbox shortcut documentation and configuration; running HIPPO inside Neovim is outside required
acceptance.

The runner creates no host, task fixture, or candidate binary. The host operator prepares those under the execution
plan, with two known Running and two known Queue rows, safe unique labels, and enough detail content to scroll. The
operator also supplies actual launch, terminal-state, native-input, and induced-condition artifacts. Startup/read fault
causes and pending-refresh timing require bounded real-host probes; naming a condition does not prove it occurred.

Run `go test -count=1 ./tests/host/...` in the pinned Go environment. `integration/` exercises the compiled verifier and
Bash runner through temporary Git/source/config fixtures and command stand-ins. It never creates a real host.

See [the recorder protocol](evidence-verifier/README.md) for staged commands, receipt fields, and proof limits. Ordinary
gate registration and the two actual host sequences remain separate delivery steps.
