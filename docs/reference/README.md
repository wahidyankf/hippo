# Reference

Exact, information-oriented facts about HIPPO. Look things up here; learn elsewhere.

## Pages

- [Command-line interface](./cli.md) — every command, every flag, every default.
- [Exit codes](./exit-codes.md) — the stable `0` / `1` / `73` / `75` / `78` contract.
- [Environment variables](./environment-variables.md) — what HIPPO reads, what it exports, and the
  rules for caller-selected concurrency mappings.
- [Configuration](./configuration.md) — schemas 1–3, tiers, promotion, coordination caps, and profiles.
- [Resource policy](./resource-policy.md) — profiles, task classes, resource tiers, and safe capacity.
- [Shared state root](./state-root.md) — where runtime state lives and what each file is for.
- [JSON and evidence formats](./json-schemas.md) — status, identity, history, receipt, and evidence schemas.

## Also canonical

The [specifications tree](../../specs/README.md) is the canonical, implementation-independent
description of HIPPO. [`specs/architecture.md`](../../specs/architecture.md) holds the as-built C4
model, and [`specs/behaviours/`](../../specs/behaviours/README.md) holds the executable Gherkin
corpus. Where this reference and the Gherkin corpus disagree about observable behavior, the corpus
is authoritative.

## Next steps

- [How-to guides](../how-to/README.md) for a goal you already have.
- [Explanation](../explanation/README.md) for the reasoning behind these facts.
