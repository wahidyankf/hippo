# Native Policy Hooks

[Repository settings](../settings.json) register the `agent-policy` selector once for native shell, read, multi-read,
list, write, edit, search and glob tools. The wrapper forwards the original JSON payload to the maintained local router
with `--scope local --harness commandcode`; repository policy decides destination ownership and admission. The
registration uses `failClosed`, a 30-second timeout and no repository FERRET capture.

## Contents

- [Policy wrapper](run-policy-hook.sh) — The local endpoint transport.
- [Selector regression](agent-policy-selector.test.sh) — Synthetic native payload, local ownership and forwarding
  checks.
- [Git fixture isolation regression](git-fixture-isolation.test.sh) — Checks native Git-local environment purge,
  physical fixture ownership, and unchanged disposable parent config, index and HEAD. The
  [quick suite](../../scripts/test-quick.sh) runs it once after the selector regression without recursively invoking the
  full driver.

The existing native check runs the regression. Passing synthetic checks does not prove installed CLI discovery or live
enforcement. Global capture and approval configuration are separate workstation sources.
