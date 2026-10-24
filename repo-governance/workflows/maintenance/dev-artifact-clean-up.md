# Dev Artifact Clean-Up

## Entry

A task, plan, or investigation has finished, and it produced artifacts that were useful during the work and are not part
of its result.

## Sequence

1. **Enumerate what the task created.** Scratch directories, generated reports, temporary scripts, downloaded fixtures,
   task branches, task worktrees, [leaked test containers](#leaked-test-containers), and any tooling installed only for
   this work.

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

   A squash merge puts one new commit on `main` and never the branch's own commits, so `git branch -d` refuses. After
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

## Leaked Test Containers

A **leaked test container** is a container, volume, or network that a task's test or dev run created and that outlives
the run. A run killed before its own teardown, such as one shed by a resource governor, leaks one. Leaked containers
hold memory in the container VM, and a governor then defers unrelated work with memory warnings.

Detect them read-only. `$ROOT` is the task's checkout or worktree; a repository may also declare a name prefix:

```bash
for n in $(docker ps -a --format '{{.Names}}'); do
  w=$(docker inspect "$n" --format '{{index .Config.Labels "com.docker.compose.project.working_dir"}}')
  case "$w" in "$ROOT"|"$ROOT"/*) echo "$n";; esac
done
```

Remove only those names, then re-run the listing and expect zero. `<project>` is the container's
`com.docker.compose.project` label:

```bash
docker rm -f -v <name>
docker volume rm $(docker volume ls -q --filter label=com.docker.compose.project=<project>)
docker network rm $(docker network ls -q --filter label=com.docker.compose.project=<project>)
```

Never use `docker system prune`, `docker volume prune`, or an alias that removes every container for this: each removes
what the task did not create. Never remove a container another actor is running. When no container is left running and
the engine runs in a desktop VM, stop its app (`docker desktop stop`); the VM keeps its memory until it stops.

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
