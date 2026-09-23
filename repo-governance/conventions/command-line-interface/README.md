---
description: >-
  Indexes the ordered modules holding the command-line interface contract — exit statuses, streams, input, structured
  output, arguments, diagnostics, and terminal behaviour.
when_to_use: >-
  Use to locate the module covering exit statuses, streams, standard input, machine-readable output, arguments,
  diagnostics, terminal and environment behaviour, or help and discovery.
---

# Command-Line Interface Modules

Read in order. Together the first eight hold the contract the [Command-Line Interface](../command-line-interface.md)
entrypoint indexes; the last is this repository's own record, like the entrypoint's tiers.

## Directory Map

- [001 The Exit Status Contract](001-exit-status-contract.md)
- [002 Stream Discipline](002-stream-discipline.md)
- [003 Standard Input](003-standard-input.md)
- [004 Machine-Readable Output](004-machine-readable-output.md)
- [005 Arguments and Flags](005-arguments-and-flags.md)
- [006 Errors and Diagnostics](006-errors-and-diagnostics.md)
- [007 Terminal and Environment](007-terminal-and-environment.md)
- [008 Help and Discovery](008-help-and-discovery.md)
- [009 FERRET Capture Here](009-ferret-capture-here.md)
