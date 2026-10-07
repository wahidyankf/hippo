# Language

English, in every artifact this repository produces: source, comments, commit messages, specifications, documentation,
pull-request bodies, and the strings the binary prints.

## Why

The audience is not one person. This is a public repository whose consumers are other repositories, and its output is
read by whoever is holding the failing build. A second language in any of those places splits the readership and leaves
part of it guessing.

## Requirements

- Write prose a reader can follow without the context that produced it.
- Keep comments, commit messages, and work notes brief.
- Use clear, simple, natural English in those texts so readers who use English as an additional language can follow.
- When a work note explains code, state why it is needed rather than narrating its visible behavior.
- Prefer plain words to jargon where both are exact. Where jargon is exact and plain words are not, use the jargon and
  define it once.
- Keep identifier names, exit-code names, and configuration keys stable and in English. Renaming one is a
  [public contract](../development/public-contract.md) change, not a spelling correction.
- Diagnostics say what happened and what the reader may do about it. A message that names a condition without naming its
  remedy sends the reader to the source.

Non-English content is acceptable only where it is the subject rather than the medium — a test fixture proving that
Unicode handling works, for instance.

## Enforcement

Review judges brevity, natural wording, and whether a note explains a reason. A mechanical check cannot distinguish
useful context from needless narration, so these rules are unenforced by decision.
