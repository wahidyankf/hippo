# Upstream Tool Defects

A pinned tool that is still maturing fails in ways its own tests have not met, and a consumer is often the first to see
it. A defect noticed and silently worked around is found again by the next consumer at the same cost. This standard
turns each sighting into a record at the owner, and a blocking one into a fix there.

## Scope

It covers tools this repository consumes as pinned releases from an upstream it can contribute to; the decision below
names them and their owners. A tool this repository cannot change is reported through its project's own channel per
[bug reports](../conventions/bug-reports.md); the plan steps below do not apply.

## Watch While Using

Every use is also a check. A defect is behaviour that contradicts the tool's documentation, its own output, or its
stated contract: a wrong exit status, a misleading message, a crash, a silent no-op, a documented option that does
nothing. A peculiarity — behaviour that is not wrong but surprises a careful user — counts too, because the next user is
surprised the same way; its fix may be documentation.

## On a Sighting

1. **Reproduce** on the pinned version, then on the owner's latest trunk. A defect already fixed there needs a repin,
   not a plan.
2. **Check for duplicates** in the owning repository — open issues, open pull requests, in-flight plans, and idea briefs
   — per [bug reports](../conventions/bug-reports.md).
3. **When a match exists, wait for it.** Link it from the current work, add to it what it lacks, and continue on a
   workaround. Repin once it lands, releasing it first per step 5 if no release carries it. With no workaround, a match
   already in repair leaves the current work blocked on that link, while a match that is only an idea brief is promoted
   to a bug-fix plan as in step 5.
4. **When the defect has a workaround or does not block the work in hand, file it and continue.** Write an
   [idea brief](../../plans/ideas/README.md) in the owner's `plans/ideas/`, in the owner's own layout, with the report
   in its problem section and the duplicate check and references in its prior art. Land it through the owner's route,
   then resume the current work on the workaround.
5. **Only when the defect blocks the work in hand and no workaround exists, fix it.** Write a
   [bug-fix plan](../conventions/plans/010-bug-fix-plan.md) in the owning repository, researching the cause and the
   solution and citing every source. Land the plan alone on the owner's trunk through its route first, run the plan
   quality gate on it, and after its verdict execute it through the owner's delivery, regression test first. Once its
   regression test and the owner's full release gate pass on the exact revision, release it through the owner's release
   cut without a further prompt, skipping no step, and repin every consumer.

A workaround is any route to the current work's outcome that does not edit the tool or its pin: another option or
command, a documented manual step, or a retry that reliably succeeds. It is recorded beside the link or brief so the
next consumer reuses it.

A security defect skips every public step and goes through the owner's private security channel.

## Why the Fix Lives at the Owner

The cause lives in the owner, so the fix does too, and this repository never carries a local patch, a vendored copy, or
a weakened gate. The work that found the defect continues on a workaround rather than absorbing the fix, so it stays
reviewable, and each repository's change arrives with its own evidence, as
[rules propagation](../workflows/quality/rules-propagation.md) and [plan lifecycle](../conventions/plan-lifecycle.md)
require.

## This Repository's Decision

| Tool   | Pinned by     | Owning repository                            |
| ------ | ------------- | -------------------------------------------- |
| RHINO  | `rhino.lock`  | `rhino`                                      |
| FERRET | `ferret.lock` | `ose-public`, which builds `apps/ferret-cli` |

Both owners land work through a pull request to their `main`, under their own rules; a brief or plan filed there follows
that route, in that repository's own `plans/ideas/` layout.

HIPPO is this repository's own tool: a HIPPO defect found here is fixed in place under the ordinary rules, and one a
consumer files here follows the [plan lifecycle](../conventions/plan-lifecycle.md).

Adopting this standard is the standing request under which a defect's idea brief, a blocking defect's bug-fix plan, and
a merged fix's release once its tests pass need no further authorization. Adopted from the shared catalog by explicit
copy; its root-cause relationship is restated above.
