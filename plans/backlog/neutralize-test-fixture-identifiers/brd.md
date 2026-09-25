# Business Requirements: Neutralize Test Fixture Identifiers

## Business Goal

Keep this public repository's fixtures synthetic, so no tracked file carries forward the name of a private repository.

## Affected Roles

- Maintainers responsible for what the public tree publishes.
- Contributors who copy existing fixtures as templates.

## Desired Outcomes

- The source-override test uses a synthetic value and still proves the override.
- No tracked file keeps the removed string.

## Success Measures

- The mutation RED fails and the GREEN passes on `TestOverrideSourceWithoutFile`.
- `npm run test:quick`, `npm test`, and the pull-request quality gate pass on the exact head.

## Non-Goals

- Rewriting history or removing the value from published commits.
- Auditing other fixtures beyond a repository grep for the removed string.

## Business Risks

- A careless replacement could weaken the assertion; the mutation step proves the test still reads the value.
