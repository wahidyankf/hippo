# Dev Artifact Clean-Up

## Entry

A task, plan, or investigation has finished, and it produced artifacts that were useful during the work and are not part
of its result.

## Sequence

1. **Enumerate what the task created.** Scratch directories, generated reports, temporary scripts, downloaded fixtures,
   task branches, task worktrees, and any tooling installed only for this work.

   Never delete a secret-bearing file or directory from the primary `main` checkout. An exact ignored, nonshared cache
   such as `.fvm-cache/` is scratch after recorded regeneration, non-use, and secret-free evidence, regardless of
   origin.

2. **Classify each one:**

   | Class    | Disposition                                                             |
   | -------- | ----------------------------------------------------------------------- |
   | result   | keep; it is part of what the work delivered                             |
   | evidence | keep, in the location the plan declared for evidence                    |
   | scratch  | remove                                                                  |
   | unknown  | investigate before removing; never delete something you cannot classify |

3. **Remove the scratch class.** Delete files, remove worktrees, delete task branches that have served their purpose.

   A rebase merge gives the landed commits new identifiers, so `git branch -d` refuses. After
   `git fetch origin --prune`, `git branch -D <branch>` may delete a task branch no worktree holds
   (`git worktree list --porcelain` names no `branch refs/heads/<branch>`) that either **landed**, its pull request
   `MERGED` with `headRefOid` equal to the tip or `git cherry origin/main <branch>` printing only `-` lines, or is
   **stale**, its tip's committer date over 72 hours old with no open pull request from it. When that cherry prints a
   `+` line, a stale tip is preserved first: keep `origin/<branch>` if it holds the tip, else
   `git bundle create <path> origin/main..<branch>` under the primary checkout's ignored `local-tmp/`, recording the
   path. Only then does the remote branch go, by exact ref: `git push origin --delete <branch>`. Otherwise retain the
   branch with the reason. Never use wildcard refs, delete `main` or a protected branch, or force a worktree removal.

4. **Preserve unrelated work.** A dirty file that this task did not create is not cleanup's business. Cleanup removes
   what the task made; it never restores a working copy to some imagined clean state.
5. **Prove absence.** Re-list the paths and confirm they are gone, and confirm the working tree holds only what it
   should. A cleanup that was performed but not verified is a claim.

## Exit

Every task-created artifact is classified, the scratch class is removed, its absence is verified, and unrelated changes
are untouched.

## Leftovers

Scratch a crashed session left behind in `local-tmp/` is reclaimed only deliberately, never by an ambient sweep: once
unmodified for seven days, it moves to `local-tmp/.reclaim-quarantine-YYYY-MM-DD/`, and is deleted once nothing needs
it.

## Deletion Is Not Reversible in the Way People Assume

Version control restores what was committed. Scratch artifacts are, by definition, uncommitted — deleting one is
permanent.

That is why `unknown` exists as a class and why it routes to investigation rather than to removal. The cost of keeping
one unrecognized file for another day is a stale file. The cost of deleting the one thing that was not reproducible is
the work itself.
