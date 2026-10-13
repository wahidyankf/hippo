# Push Review

The merge review guards what lands on `main`, but a pushed branch is already public. Every push is therefore reviewed
before it leaves, privately, with nothing posted.

## Range

For each ref the push updates, the range runs from `origin`'s current tip of that ref to the local head. A ref `origin`
lacks starts from its merge base with `origin/main`.

## Sequence

1. **List the range's commits.** The harness lists every commit in the range, oldest first, including merges.
2. **Run the screen on the range.** The `pre-push` hook runs it commit by commit per [enforcement](003-enforcement.md);
   a scan error blocks exactly as a finding does, and the hook is never bypassed — see
   [push hook verification](../../../conventions/push-hook-verification.md).
3. **Spawn a fresh-context reviewer.** One read-only subagent for this range alone: no part in authoring the commits, no
   memory of an earlier range, never pushing or posting. It receives only the range and the three leak classes. A failed
   spawn blocks the push exactly as a finding does; no self-judging fallback, and no human substitutes.
4. **Read every commit.** The subagent reads each commit's added lines, its file names, its message, and the ref name. A
   merge contributes what it resolved beyond the automatic merge.
5. **Judge against the [leak classes](001-leak-classes.md).** The subagent returns the count per class and each finding
   without its value. No candidate is copied into notes, commands, or logs.
6. **Decide.** The harness acts on that verdict unchanged. No finding: push. Any finding: do not push; remediate, then
   start again from step 1 with a new subagent.

## Remediation

Before the push, the fix is to the history, not the tree. A later commit that deletes the value does not pass, because
the commit that added it would still be published. Rewrite the unpushed commits so that none carries the value: amend
the latest commit, or rebuild the range without it.

| Class                            | Replace the value with                                                      |
| -------------------------------- | --------------------------------------------------------------------------- |
| `secret_or_private_value`        | a reference to environment or secret storage; rotate it if it left the host |
| `protected_environment_property` | an environment variable read at run time                                    |
| `machine_specific_absolute_path` | a `~/` path, a repository-relative path, or a documented placeholder        |

After the push, the value is disclosed. Stop, rotate any credential, and report it to the repository owner. Here,
rewriting published history is not a remedy at all: [data safety](../../../conventions/public-repository-data-safety.md)
rules it out, and the `main` ruleset refuses the force push it would need. Correcting the tree alone is never the whole
remedy either.
