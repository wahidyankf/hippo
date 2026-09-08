# Working Tree

What stays out of history, and why each entry is there.

## Always Ignored

- **Generated binaries.** `hippo`-derived builds, `dist/`, anything a compiler produced. A binary in history is a binary nobody can verify against the source that made it.
- **Coverage output.** `coverage/`. It is a measurement of one run on one machine.
- **Machine-local configuration.** `hippo.local.json` and anything like it. It names a workstation; the tracked `hippo.local.json.example` is the shape without the machine.
- **Runtime evidence.** The guard's coordination ledger, leases, and bounded evidence records. They describe a host, which [data safety](public-repository-data-safety.md) prohibits publishing.
- **`local-tmp/`.** Scratch. Nothing here is authoritative and nothing here is a plan.
- **`generated-reports/`.** Requested audits and reports. Useful, dated, and not a specification — a report is never cited as the reason a rule exists.
- **`node_modules/`.** Locked by `package-lock.json`, restored by `npm ci`.

## Not Ignored, Deliberately

`worktrees/` is **not** an ignore entry here, and adding one would be a defect. This repository's worktrees live beside the checkout, and an ignore entry inside it would invite the layout that breaks `go build` — see [worktree location](worktree-location.md).

## Verifying

`.gitignore` is checked by behaviour, not by inspection: scenarios assert that generated binaries, coverage, machine-local configuration, and runtime evidence are all ignored, and that the example configuration is tracked. Adding an ignored path means adding it to that inventory too, or the rule is only as strong as the next reader's memory.
