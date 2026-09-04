# Resource Guard Contributor Rules

- Keep the CLI generic and repository-independent; do not add product-specific defaults.
- Preserve exit codes `73`, `75`, and `78`, config compatibility, and supported evidence readers.
- Update executable Gherkin before observable behavior and keep unit, integration, and E2E adapters strict.
- Run `./scripts/test-quick.sh` for fast verification and `./scripts/test.sh` before release.
- Keep generated binaries, coverage, local configuration, runtime evidence, and scratch artifacts ignored.
- Never commit credentials, personal or machine identifiers, absolute local paths, or private infrastructure values.
- Comment non-obvious shell safety invariants and lifecycle boundaries; avoid line-by-line narration.
- Build release assets only through `./scripts/build-release.sh <version> <commit> <output-dir>`.
- Never replace an existing release tag or weaken checksum verification.
