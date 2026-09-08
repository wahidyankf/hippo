# Done

This stage preserves completed plans as delivery records. A finished folder is named `YYYY-MM-DD__<slug>`, the date being when the work completed, so the directory sorts by delivery rather than by intent.

What is here is history. It describes the repository as it was on purpose, and it may name paths that have since moved. That is why [`repo-config.yml`](../../repo-config.yml) excludes `plans/done/**` as a link _source_ while leaving it a valid target: an archived plan's outward links are allowed to have aged, and reporting them would make the act of finishing a plan generate findings. For what the tool does today, read [`specs/`](../../specs/README.md).

Reconcile required and conditional delivery, acceptance, verification, and learnings before archiving. [Plan execution](../../repo-governance/workflows/plan-execution.md) owns the move itself: it refuses an existing destination rather than merging into it, overwriting it, or adding a suffix; records completion metadata and outcomes; moves the folder out of [`../in-progress/`](../in-progress/README.md); updates both stage indexes in one change; and then verifies the archive rather than assuming it.

## Completed Plans

No plan has completed in this repository yet.

## Directory Map

This stage holds no plan folder, so this README has no siblings to map.
