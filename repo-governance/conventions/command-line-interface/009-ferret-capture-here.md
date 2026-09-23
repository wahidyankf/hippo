---
description: >-
  Records where the harness activity `./ferret` queries is captured, and that this repository carries no capture
  registration of its own.
when_to_use: >-
  Use when querying coding-agent harness activity here, or before adding a harness hook that forwards payloads to
  FERRET.
---

# FERRET Capture Here

Like [Tiers Here](../command-line-interface.md#tiers-here), this is this repository's own record rather than part of the
contract; it lives here because the entrypoint is at its word budget. `./ferret` sits at the floor tier there.

FERRET records coding-agent harness activity. Capture is registered at the user level of the maintainer's harness
configuration, not in this repository, so nothing here forwards hook payloads. Use `./ferret` to query the local record
— `./ferret status --json`, whose `dataHome` names where it lives, and `./ferret usage --group-by tool --json`.
