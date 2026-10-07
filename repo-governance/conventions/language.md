# Language

English, in every artifact this repository produces: source, comments, commit messages, specifications, documentation,
pull-request bodies, and the strings the binary prints.

## Why

The audience is not one person. This is a public repository whose consumers are other repositories, and its output is
read by whoever is holding the failing build. A second language in any of those places splits the readership and leaves
part of it guessing.

## Requirements

- Write prose a reader can follow without the context that produced it.
- Keep authored prose concise, including comments, commit messages, work notes, plans, and other documents, without
  omitting details readers need to act or verify a claim.
- Use clear, simple, natural English in authored prose and English agent replies so readers who use English as an
  additional language can follow.
- When a note or document records a non-obvious code choice or other decision, explain why it was needed rather than
  narrating behavior or steps already visible nearby.
- Prefer plain words to jargon where both are exact. Where jargon is exact and plain words are not, use the jargon and
  define it once.
- Keep identifier names, exit-code names, and configuration keys stable and in English. Renaming one is a
  [public contract](../development/public-contract.md) change, not a spelling correction.
- Diagnostics say what happened and what the reader may do about it. A message that names a condition without naming its
  remedy sends the reader to the source.

Non-English content is acceptable only where it is the subject rather than the medium — a test fixture proving that
Unicode handling works, for instance.

## Enforcement

Review judges brevity, natural wording, and whether a note or document explains a decision. A mechanical check cannot
distinguish useful context from needless narration, so these rules are unenforced by decision.
