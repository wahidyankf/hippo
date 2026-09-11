# Skills

Canonical skill definitions. Every harness reads these; Claude Code reads a generated wrapper under `.claude/skills/`
that routes back here. Edit a skill here and run `node scripts/generate-adapters.mjs`.

## Planning

The six skills of the canonical plan system, in the order a plan moves through them.

- [grill-me](grill-me/SKILL.md) — present a decision as exclusive options with one recommendation.
- [plan-grooming-idea-briefs](plan-grooming-idea-briefs/SKILL.md) — judge whether an idea brief is worth promoting.
- [plan-creating-project-plans](plan-creating-project-plans/SKILL.md) — author the six documents of a formal plan.
- [plan-writing-gherkin-criteria](plan-writing-gherkin-criteria/SKILL.md) — write acceptance scenarios that can fail.
- [plan-validating-quality](plan-validating-quality/SKILL.md) — judge whether a draft is complete and executable.
- [plan-verifying-execution](plan-verifying-execution/SKILL.md) — judge whether execution did what the plan said.

## This Repository

- [release-cut](release-cut/SKILL.md) — cut and publish a HIPPO release.
- [spec-impact-assessment](spec-impact-assessment/SKILL.md) — assess what a change does to the specifications.
- [worktree-to-pr](worktree-to-pr/SKILL.md) — take a change from a fresh worktree to a merged pull request.
