# Native Policy Hooks

[Repository settings](../hooks.json) register the `agent-policy` adapter once for every `PreToolUse` event. The wrapper
forwards the original JSON payload to the maintained local endpoint with `--harness muse`. Muse honors exit statuses and
ignores Claude-style stdout JSON decisions, so the wrapper converts a denial to stderr plus exit 2 and keeps an allow
silent with exit 0. The registration uses a 30-second timeout and no repository FERRET capture.

## Contents

- [Policy wrapper](run-policy-hook.sh) — The local endpoint transport with the exit-status contract.
- [Adapter regression](run-policy-hook.test.sh) — Allow stays silent, every deny and failure explains on stderr.
