# Gherkin Implementation Review

A manual review, required whenever a scenario or a step binding changes. It exists because the automated checks answer a narrower question than the one that matters.

## What the Automation Already Answers

That every step resolves to exactly one handler, that no handler is unreached, that exemptions are exact and inventoried, and that the scenarios ran. None of that says the binding does what the sentence claims.

## What the Review Asks

- **Does the step do what its sentence says?** A step named "the named repository is unchanged" that reads a variable set by the previous step is a step that asserts nothing.
- **Would it fail?** Mutate the code the scenario covers and confirm the scenario goes red. This is the whole review in one question.
- **Is the assertion on the outcome, or on the setup?** Asserting what the fixture just wrote is the most common way a passing scenario proves nothing.
- **Are placeholders, no-ops, or outcome tables that assert nothing present?** All three are rejected outright.
- **Does the scenario read as behaviour?** A sentence naming a function, a struct, or a file path is a unit test wearing Gherkin.
- **Is the boundary classification honest?** A scenario is classified by the strongest real boundary its setup, subject, or assertions touch — not by what it is allowed to use.

## Output

Findings, each naming the scenario and what would make it prove something. A scenario that survives review has been shown to fail for the right reason, and that demonstration belongs in the pull-request body.

See [specification maintenance](../development/specification-maintenance.md) for the cycle this review sits inside.
