# Command Code Bindings

The Command Code profile in [`repo-config.yml`](../repo-config.yml) selects native leaf agents. Their adapters import
the authoritative definitions in [`.agents/agents/`](../.agents/agents/README.md).
[`agents/catalog.json`](agents/catalog.json) records the generated native selection.

Generate adapters with `./rhino harness adapters generate`; validate them with `./rhino harness adapters validate`.
Change the canonical definitions or profile instead of editing generated routes.

## Repository Policy

- [Settings](settings.json) — One native repository policy endpoint.
- [Hooks](hooks/README.md) — Its transport and selector regression.

Personal approval defaults and FERRET capture belong to global configuration.
