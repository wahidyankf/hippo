# Native Repository Binding

Muse reads project instructions (`AGENTS.md`) and project skills (`.agents/skills/`) natively, so this binding carries
no generated agent adapters. It registers one repository policy endpoint for `PreToolUse` and adapts the endpoint's
decision to the exit-status contract Muse honors.

## Contents

- [Hooks](hooks.json) — One repository policy endpoint with a 30-second timeout.
- [Hooks](hooks/) — Exit-status policy adapter and its behavior regression.

Repository policy remains local; model choice and approval defaults belong to the operator's own configuration. Source
settings alone do not prove installed CLI discovery or live execution.
