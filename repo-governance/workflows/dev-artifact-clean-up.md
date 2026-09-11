# Dev Artifact Clean-Up

## Entry

A task, plan, or investigation has finished, and it produced artifacts that were useful during the work and are not part
of its result.

## Sequence

1. **Enumerate what the task created.** Scratch directories, generated reports, temporary scripts, downloaded fixtures,
   task branches, task worktrees, and any tooling installed only for this work.
2. **Classify each one:**

   | Class    | Disposition                                                             |
   | -------- | ----------------------------------------------------------------------- |
   | result   | keep; it is part of what the work delivered                             |
   | evidence | keep, in the location the plan declared for evidence                    |
   | scratch  | remove                                                                  |
   | unknown  | investigate before removing; never delete something you cannot classify |

3. **Remove the scratch class.** Delete files, remove worktrees, delete task branches that have served their purpose.
4. **Preserve unrelated work.** A dirty file that this task did not create is not cleanup's business. Cleanup removes
   what the task made; it never restores a working copy to some imagined clean state.
5. **Prove absence.** Re-list the paths and confirm they are gone, and confirm the working tree holds only what it
   should. A cleanup that was performed but not verified is a claim.

## Exit

Every task-created artifact is classified, the scratch class is removed, its absence is verified, and unrelated changes
are untouched.

## Deletion Is Not Reversible in the Way People Assume

Version control restores what was committed. Scratch artifacts are, by definition, uncommitted — deleting one is
permanent.

That is why `unknown` exists as a class and why it routes to investigation rather than to removal. The cost of keeping
one unrecognized file for another day is a stale file. The cost of deleting the one thing that was not reproducible is
the work itself.
