# HIPPO Contributor Rules

This file is an index. Every rule lives in [`repo-governance/`](repo-governance/README.md); this file states none of its own.

Start with [the vision](repo-governance/vision/README.md) if you have not worked here before: it explains what a guard nobody notices is for.

## The Product

HIPPO holds no defaults about the work it guards: [repository independence](repo-governance/principles/repository-independence.md).

Exit codes `73`, `75`, and `78`, the evidence readers, and configuration compatibility are the [public contract](repo-governance/development/public-contract.md); none moves without authorization.

## Plans

Plans are working records under [`plans/`](plans/README.md), never architecture: the [plans convention](repo-governance/conventions/plans.md), the [local rules](repo-governance/conventions/plan-lifecycle.md) layered on it, and [specification changes](repo-governance/conventions/plan-specification-changes.md).

What a plan is groomed, written, executed and reviewed with — every workflow, skill and agent of the roster — is [planning capabilities](repo-governance/development/planning-capabilities.md).

## Specifications

`specs/` is canonical. Assess it before every change and write behaviour there first: [specification maintenance](repo-governance/development/specification-maintenance.md). Keep the C4 model as-built: [architecture specifications](repo-governance/development/architecture-specifications.md).

`README.md`, `docs/`, and `CHANGELOG.md` follow Diátaxis and may not contradict `specs/`: [documentation architecture](repo-governance/conventions/documentation-architecture.md).

## Testing

One corpus, three executing boundaries: [behaviour-driven development](repo-governance/development/behaviour-driven-development.md), with the outermost one in [end-to-end testing](repo-governance/development/end-to-end-testing.md).

Red, green, refactor, with evidence of each: [test-driven development](repo-governance/development/test-driven-development.md) and [the cycle](repo-governance/workflows/red-green-refactor.md). A changed scenario or binding needs [a manual review](repo-governance/workflows/gherkin-implementation-review.md).

What runs and where: [quality gates](repo-governance/development/quality-gates.md). What must pass before a change is done: [software quality enforcement](repo-governance/development/software-quality-enforcement.md). This repository never acquires Nx.

## Change Discipline

[Minimal sufficiency](repo-governance/principles/minimal-sufficiency.md) governs the size of a change; [code clarity](repo-governance/development/code-clarity.md) governs its shape; [dependency selection](repo-governance/development/dependency-selection.md) governs what it may depend on.

HIPPO cannot guard HIPPO; the reason, and the contract this repository owes consumers, is in [resource-aware development](repo-governance/development/resource-aware-development.md).

Workflow storage stays inside the free allowance: [GitHub Actions storage](repo-governance/development/github-actions-storage.md).

## Version Control

Only `main` persists. Work reaches it through [worktree to pull request](repo-governance/workflows/worktree-to-pull-request.md), under the [integration path](repo-governance/conventions/integration-path.md), from a worktree beside this checkout rather than inside it — [worktree location](repo-governance/conventions/worktree-location.md).

[Thematic commits](repo-governance/conventions/thematic-commits.md), one [delivery boundary](repo-governance/conventions/pull-request-boundaries.md) per pull request, a [body](repo-governance/conventions/pull-request-body.md) a reviewer can use, and [merge preconditions](repo-governance/conventions/pull-request-merge.md) that hold every time.

Committing and pushing need [authorization](repo-governance/conventions/commit-authorization.md). Never commit what [data safety](repo-governance/conventions/public-repository-data-safety.md) prohibits. Never bypass a [push hook](repo-governance/conventions/push-hook-verification.md). Never destroy work or history without [approval for that one command](repo-governance/conventions/no-destructive-git-operations.md). Keep the [working tree](repo-governance/conventions/working-tree.md) clean and poll GitHub [no faster than three minutes](repo-governance/conventions/github-polling.md).

Releases: [release cut](repo-governance/workflows/release-cut.md). A published tag is never replaced.

## Harnesses

One canonical instruction body, expressed per harness: [coding harness contract](repo-governance/conventions/coding-harness-contract.md). Changing it follows [the change workflow](repo-governance/workflows/coding-harness-contract-change.md) and [parity verification](repo-governance/workflows/coding-harness-parity-verification.md).

## Working Here

Write in [English](repo-governance/conventions/language.md). Keep [task state in the repository](repo-governance/conventions/task-tracking.md). [Exhaust the repository before asking](repo-governance/conventions/last-resort-questions.md). Diagrams are [Mermaid](repo-governance/conventions/markdown-visualizations.md) and every internal [link resolves](repo-governance/conventions/markdown-links.md).
