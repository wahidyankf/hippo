# Public Repository Data Safety

This repository is public. Everything committed is published, is in someone's clone within the hour, and cannot be
unpublished — deleting it later removes it from the tip and from nowhere else.

## Never Commit

- Credentials of any kind: tokens, keys, passwords, session cookies, signed URLs.
- Personal or machine identifiers: usernames beyond the commit author, hostnames, serial numbers, MAC or IP addresses,
  device names.
- Absolute local paths. `/Users/<name>/...` names a person and a machine layout in one string.
- Private infrastructure values: internal hostnames, ports behind a boundary, tailnet names, cloud account or project
  identifiers.
- Private repository identifiers, with one exception: the name `ose-private` and its GitHub URL, public by the
  maintainer's 2026-10-07 decision. Its contents, paths, and internals stay private.
- Real host evidence captured from a workstation. HIPPO reads memory pressure, CPU counts, and process tables; a sample
  committed as a fixture describes the machine it came from.

## Before Every Commit

Inspect the diff — not memory, not intent — and remove anything above. The check is on what this commit publishes, which
includes a file added earlier and still untracked-then-staged now. [Thematic commits](thematic-commits.md) makes the
diff small enough to actually read.

Before every push and every merge, the [leak review](../workflows/quality/pr-leak-review.md) judges each outgoing commit
against this list, because history publishes what a later commit deletes.

## Before Publishing Anything Else

A pull-request title and body, a comment or review body, and release notes are published the moment they are posted, and
no diff inspection reads them. Screen each one first, with the same screen the hooks run:

```sh
scripts/public-safety/outbound-preflight.sh --surface pull-request --text "<title>" --file <body-file>
scripts/public-safety/outbound-preflight.sh --surface release --text "<tag>" --file <notes-file>
```

Only exit `0` publishes. Exit `1` is a finding and exit `2` is a screen that did not run; neither publishes.

## In Fixtures and Documentation

Use `example.invalid`, `fixture`, and obviously synthetic values. A realistic-looking value invites a reader to treat it
as real, and a real value that looks synthetic is the failure this rule exists to prevent.

Transcripts published in `docs/` are executed against a real build, so they must be executed against one holding nothing
private. Where a path cannot be shown safely, say so rather than inventing output — see
[documentation architecture](documentation-architecture.md).

## If Something Lands Anyway

Treat it as disclosed. Rotate the credential, then remove it. Rewriting history is not a remedy on a public repository
and the ruleset refuses the force push it would need.

## Agent Tool Access

Agent tools must not directly read, write, or edit `.env*` files, except `.env.example`. Paths under `secrets/` and
`credentials/` are also protected. The repository owns these restrictions.
[`.agents/agent-policy.json`](../../.agents/agent-policy.json) declares the path patterns for
`scripts/agent-policy-hook.sh`; the tracked `.serena/project.yml` excludes the same paths from semantic indexing. Claude
Code, OpenCode, Command Code, and Muse native bindings enforce their own physical checkout; neutral routing covers
cross-repository operations, with Muse converting decisions to exit statuses. Compute-admission exemptions do not waive
these restrictions.

Agents must not open or read private-key contents anywhere, including outside every repository. Each secret step must be
exactly one copy-ready command carrying its working directory, placed by the agent on the operator clipboard; the
operator runs it in their own terminal, entering each secret at a hidden prompt only, and replies with the success word
the step names, and the agent continues only on it. Agents must never request, handle, or record secret values, full
outputs, or screenshots, and must avoid any script or interaction that prints, echoes, logs, or otherwise exposes a
secret to the harness session; diagnosis uses exit and error codes alone, and only local-development secrets (loopback
services, dummy or example values such as `.env.example`) may appear in session records. The secret session must open
only inside the phase that needs it and lock before any review, pause, or PR handoff. Agent-assisted delivery must apply
only with a proper pipeline and secrets management (tracked wrapper, closed one-word outputs, leak screens, green
gates); elsewhere the agent stops at the secret boundary. An agent may operate the leak review only after reading every
commit at the pinned head against the three leak classes; a record without that reading is fabrication. This removes
value exposure from the session, and proper pipelines earn faster delivery. No gate enforces this; review checks the
observations.
