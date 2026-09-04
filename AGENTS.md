# Resource Guard Contributor Rules

- Keep the CLI generic and repository-independent; do not add product-specific defaults.
- Preserve exit codes `73`, `75`, and `78`, config compatibility, and supported evidence readers.
- Update executable Gherkin before observable behavior, prove its binding failure, and keep unit, integration, and E2E adapters strict.
- Install locked contributor tooling with `npm ci`; hooks enforce Conventional Commits, staged formatting, and the quick pre-push gate.
- Run `npm run test:quick` for fast verification and `npm test` before release; never introduce Nx.
- Keep deterministic production core coverage at or above 99%; prove platform and process boundaries through integration and E2E adapters.
- Keep generated binaries, coverage, local configuration, runtime evidence, and scratch artifacts ignored.
- Never commit credentials, personal or machine identifiers, absolute local paths, or private infrastructure values.
- Comment non-obvious shell safety invariants and lifecycle boundaries; avoid line-by-line narration.
- Build release assets only through `./scripts/build-release.sh <version> <commit> <output-dir>`.
- Never replace an existing release tag or weaken checksum verification.
