# Skills

Canonical skill definitions. Codex and OpenCode read these natively; Claude Code reads a generated wrapper under
`.claude/skills/` that routes back here. Edit a skill here and run `./rhino harness adapters generate`.

## Planning

The six skills of the canonical plan system, in the order a plan moves through them.

- [grill-me](grill-me/SKILL.md) — present a decision as exclusive options with one recommendation.
- [plan-grooming-idea-briefs](plan-grooming-idea-briefs/SKILL.md) — judge whether an idea brief is worth promoting.
- [plan-creating-project-plans](plan-creating-project-plans/SKILL.md) — author the six documents of a formal plan.
- [plan-writing-gherkin-criteria](plan-writing-gherkin-criteria/SKILL.md) — write acceptance scenarios that can fail.
- [plan-validating-quality](plan-validating-quality/SKILL.md) — judge whether a draft is complete and executable.
- [plan-verifying-execution](plan-verifying-execution/SKILL.md) — judge whether execution did what the plan said.

## Software Development

Adopted from the shared catalog with the software-development agents.

- [developing-applications](developing-applications/SKILL.md) — place code, give each error one fate, validate input
  where trust ends.
- [programming-golang](programming-golang/SKILL.md) — apply the Go standard while writing or reviewing Go.
- [programming-shell](programming-shell/SKILL.md) — apply the shell standards while writing or reviewing a script.
- [applying-maker-checker-fixer](applying-maker-checker-fixer/SKILL.md) — the judgement inside a make, check, and fix
  loop.
- [assessing-criticality-confidence](assessing-criticality-confidence/SKILL.md) — rate a finding's criticality and
  confidence.
- [generating-validation-reports](generating-validation-reports/SKILL.md) — write an audit or fix report that survives
  interruption.

## This Repository

- [release-cut](release-cut/SKILL.md) — cut and publish a HIPPO release.
- [spec-impact-assessment](spec-impact-assessment/SKILL.md) — assess what a change does to the specifications.
- [worktree-to-pr](worktree-to-pr/SKILL.md) — take a change from a fresh worktree to a merged pull request.
