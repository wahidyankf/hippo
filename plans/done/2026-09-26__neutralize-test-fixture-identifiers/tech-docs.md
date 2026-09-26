# Technical Design: Neutralize Test Fixture Identifiers

## Change

```text
internal/identity/identity_test.go::TestOverrideSourceWithoutFile
├── line 42  identity.Load(<missing path>, <non-synthetic source>, ["group=local"])  -> "fixture-source"
└── line 46  got.Source != <non-synthetic source>                                 -> "fixture-source"
```

## Design Decisions

- Use `fixture-source`, which follows the data-safety convention's `fixture` vocabulary and cannot be mistaken for a real
  repository.
- Change both sites in one edit, so the argument and the assertion cannot disagree.
- Do not name the removed string in any commit message, pull-request body, or this plan. Publishing it again would be the
  finding this plan removes.

## Specification Changes

None. No product behaviour changes. AC-01 is proved by the focused `go test`, AC-02 by `git grep`, and AC-03 by the diff
and repository gates.

## File-Impact Analysis

```text
internal/identity/identity_test.go                    [E] two lines
internal/identity/identity.go                         [G] behaviour under test, unchanged
plans/backlog/README.md, plans/in-progress/README.md  [E] stage indexes
```

## Dependencies

None.

## Rollback

Revert the commit.
