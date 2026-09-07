# Reference

Exact, information-oriented facts about HIPPO. Look things up here; learn elsewhere.

## Pages

- [Command-line interface](./cli.md) — every command, every flag, every default.
- [Exit codes](./exit-codes.md) — the stable `0` / `1` / `73` / `75` / `78` contract.
- [Environment variables](./environment-variables.md) — what HIPPO reads, what it exports, and the
  rules for caller-selected concurrency mappings.
- [Configuration](./configuration.md) — schema 1 and schema 2, coordination caps, profile overrides.
- [Resource policy](./resource-policy.md) — profiles, task classes, thresholds, reservation capacity.
- [Shared state root](./state-root.md) — where runtime state lives and what each file is for.
- [JSON and evidence formats](./json-schemas.md) — every schema HIPPO emits.

## Also canonical

The [specifications tree](../../specs/README.md) is the canonical, implementation-independent
description of HIPPO. [`specs/architecture.md`](../../specs/architecture.md) holds the as-built C4
model, and [`specs/behaviours/`](../../specs/behaviours/README.md) holds the executable Gherkin
corpus. Where this reference and the Gherkin corpus disagree about observable behavior, the corpus
is authoritative.

## Next steps

- [How-to guides](../how-to/README.md) for a goal you already have.
- [Explanation](../explanation/README.md) for the reasoning behind these facts.
