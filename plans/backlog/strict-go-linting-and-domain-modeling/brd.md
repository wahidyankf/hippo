# Business Requirements: Strict Go Linting and Domain Modeling

## Business Goal

Make the class of defect that stalled a consumer for over an hour — a domain concept carried as a string, an integer, or
a flag, so a missing case or a silent default reaches a release — fail a repository gate instead. HIPPO is a guard other
repositories trust with their gates; a guard that defers work for the wrong reason costs every consumer that pins it.

## Affected Roles

- **Consumers** who pin a HIPPO release and branch on its exit statuses, reason codes, evidence, and status JSON.
- **Contributors** who change admission, profiles, evidence, or coordination code.
- **Reviewers** who today catch, by reading, what a type or a gate could catch mechanically.
- **The owner,** who releases HIPPO and answers for a defect in a pinned version.

## Desired Outcomes

- A forgotten outcome, an unmapped internal reason, or a rule keyed to a profile's name fails a gate before review.
- `run`, `status`, and the test driver cannot disagree about admission, because they share one decision.
- A configured profile behaves as the built-in profile it derives from, for every profile-sensitive rule.
- A newer or older HIPPO sharing a state root keeps reading the other's evidence, and keeps refusing corrupt
  coordination state.
- Consumers see no contract change: a patch release they can adopt without editing anything.

## Success Measures

- The analyzer's allowlist is empty and its mechanism deleted, so a gate fails on the next `== "balanced"`.
- No string, integer, or boolean stands for an outcome, an internal reason, a lineage, or an admission path in
  production code.
- `npm test` passes on every unit's merged head, with deterministic core coverage at or above 99%.
- `v0.8.5` is published, and its exit-code vocabulary, outcome list, and status JSON shape match `v0.8.4`.

## Non-Goals

- Rewriting HIPPO in another language.
- Classifying every untyped error into a typed failure kind.
- Changing any public contract surface, or publishing the admission path.
- Repinning consumers.

## Business Risks

- **Contract drift through refactoring.** Moving 43 call sites and three codecs could change a wire value; each codec is
  pinned by a test against the `v0.8.4` value, and the existing contract scenarios run at every boundary.
- **Mixed versions on one state root.** A stricter reader could refuse an older writer's evidence; evidence readers stay
  tolerant by decision D6, and decision-bearing state keeps exactly today's acceptance set.
- **Gate fatigue.** A gate that fails unrelated work invites bypass; the analyzer ratchets from an allowlist and NilAway
  starts clean, so every later failure belongs to the change that caused it.
- **Scope growth.** Typing invites typing everything; the analyzer's name list and decision D8 bound it.
