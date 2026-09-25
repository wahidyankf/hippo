---
description: >-
  Indexes the stack standards this repository adopted, the enforced gates, defaults, and design rules one language adds,
  and the repository adapter that records how they apply here.
when_to_use: >-
  Use when changing Go or shell code here, or when deciding whether a rule belongs to one stack or to every change.
---

# Stack Standards

The stack standards this repository adopted from the shared catalog: one per language it authors. Each governs one stack
by design: it records the normative choices that stack's code must meet, and links the language-neutral standards
instead of restating them. A stack's skill defers to its standard. Placement, inheritance, and the inventory that selects
them follow [Stack Packs](../../../conventions/structure/stack-packs.md); how each applies here, and every deviation, is
in the repository adapter.

## Directory Map

- [Go Standards](golang-standards.md) — gofmt, vet, and linter gates, every error handled or returned,
  consumer-declared interfaces, owned goroutines, and statement coverage
- [Repository Adapter](repository-adapter.md) — the packs adopted here, the decisions their standards leave open, and
  every local deviation
- [Shell Standards](shell-standards.md) — beyond Shell Scripts: a supported dialect, analyser and formatter gates, quoted
  and validated input, and behaviour tests without coverage
