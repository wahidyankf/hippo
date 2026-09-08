# Public Repository Data Safety

This repository is public. Everything committed is published, is in someone's clone within the hour, and cannot be unpublished — deleting it later removes it from the tip and from nowhere else.

## Never Commit

- Credentials of any kind: tokens, keys, passwords, session cookies, signed URLs.
- Personal or machine identifiers: usernames beyond the commit author, hostnames, serial numbers, MAC or IP addresses, device names.
- Absolute local paths. `/Users/<name>/...` names a person and a machine layout in one string.
- Private infrastructure values: internal hostnames, ports behind a boundary, tailnet names, cloud account or project identifiers.
- Real host evidence captured from a workstation. HIPPO reads memory pressure, CPU counts, and process tables; a sample committed as a fixture describes the machine it came from.

## Before Every Commit

Inspect the diff — not memory, not intent — and remove anything above. The check is on what this commit publishes, which includes a file added earlier and still untracked-then-staged now. [Thematic commits](thematic-commits.md) makes the diff small enough to actually read.

## In Fixtures and Documentation

Use `example.invalid`, `fixture`, and obviously synthetic values. A realistic-looking value invites a reader to treat it as real, and a real value that looks synthetic is the failure this rule exists to prevent.

Transcripts published in `docs/` are executed against a real build, so they must be executed against one holding nothing private. Where a path cannot be shown safely, say so rather than inventing output — see [documentation architecture](documentation-architecture.md).

## If Something Lands Anyway

Treat it as disclosed. Rotate the credential, then remove it. Rewriting history is not a remedy on a public repository and the ruleset refuses the force push it would need.
