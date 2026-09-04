# HIPPO Contributor Rules

- Keep the CLI generic and repository-independent; do not add product-specific defaults.
- Preserve exit codes `73`, `75`, and `78` and supported evidence readers. Preserve config compatibility unless the owner explicitly authorizes a breaking transition.
- Before every repository change, assess both `specs/behaviours/` and `specs/architecture.md` for impact.
- Update every affected Gherkin scenario and C4 view in the same change; create behavior changes Gherkin-first, prove the binding failure, keep every adapter strict, and synchronize C4 with the final as-built boundaries and responsibilities. Record a verified no-op instead of churning an unaffected specification.
- Run every Gherkin scenario through the unit adapter; never add a unit exemption tag or inventory entry. Any integration or E2E exemption must remain exact and name both the concrete boundary and the reason it cannot execute there.
- Install locked contributor tooling with `npm ci`; hooks enforce Conventional Commits, staged formatting, and the quick pre-push gate.
- Run `npm run test:quick` for fast verification and `npm test` before release; never introduce Nx.
- Keep deterministic production core coverage at or above 99%; prove platform and process boundaries through integration and E2E adapters.
- Separate distinct setup, validation, decision, mutation, and return phases in Go functions with blank lines; automated formatters do not replace semantic grouping.
- Keep generated binaries, coverage, local configuration, runtime evidence, `local-tmp/` scratch, and `generated-reports/` ignored.
- Never commit credentials, personal or machine identifiers, absolute local paths, or private infrastructure values.
- Comment non-obvious shell safety invariants and lifecycle boundaries; avoid line-by-line narration.
- Build release assets only through `./scripts/build-release.sh <version> <commit> <output-dir>`.
- Never replace an existing release tag or weaken checksum verification.
