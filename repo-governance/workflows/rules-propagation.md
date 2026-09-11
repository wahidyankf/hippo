# Rules Propagation

Apply this workflow automatically whenever a [rule](../conventions/rules.md) in this tree is created, changed, moved, or deleted, or an explicitly requested [rules quality gate](rules-quality-gate.md) hands over findings. No separate user instruction is required: the actor that proposes or notices the change enters propagation as part of the work in hand, and the absence of a request to run it is never permission to skip it.

Propagation is the sole writer. The quality gate and [grooming](rules-grooming.md) discover, rank, and hand off; neither edits. Edits made inside one transaction do not start another.

## It Stops at This Repository's Boundary

A change to this tree propagates nowhere automatically. The siblings hold their own governance and nothing keeps the copies synchronized. That is the decision rather than an oversight: the only shared contract is machine-checked, each repository's `repo-config.yml` and the validator it runs.

Where a rule discovered here genuinely applies elsewhere, change it here with its reason, say in the pull-request body which repositories may want it and why, and let each decide in its own change with its own evidence. A rule copied without that step arrives without the reason that justified it, and the first reader to find it inconvenient will delete it correctly.

A rule that fits one repository is not thereby owed to another. [Worktree location](../conventions/worktree-location.md) is the worked example: containment inside the checkout is right for a Cargo project and breaks `go build`, and adopting it here because a sibling had it would have been the blind propagation this workflow exists to prevent.

Three phrasings of one rule across three repositories is three repositories having decided, not one having decayed. Divergence is never a finding here; where it _is_ decay, grooming catches it.

## Inputs and Transaction

Freeze: the proposed rule and its reason; its intended strength; the files, agents, or tasks in scope; known enforcement routes; any handed-over findings; the Git revision and dirty paths; and authorization. Preserve them through compaction or handoff. A material change to those inputs returns `BLOCKED_INPUT_CHANGED`; it never restarts the transaction.

## Procedure

1. Build one finite ledger from the requested outcome and any handed-over findings. Inspect only the affected rule, its points of use, higher authority, and directly overlapping guidance. Record each material gap as `OPEN`, `RESOLVED`, `NOT_APPLICABLE`, or `BLOCKED`. Do not add style preferences, speculative hardening, or checks a validator owns.
2. Before editing, return `BLOCKED_INPUT` for a missing decision or authority and `BLOCKED_CONFLICT` for an irreconcilable higher-authority conflict. Otherwise apply the minimum repair that closes every `OPEN` row:
   - place each rule at the level that owns it, and leave `AGENTS.md` linking rather than restating;
   - resolve conflicts in the order `vision > principles > conventions > development > workflows`;
   - keep one canonical statement, merge unique meaning, replace copies with links, and apply [progressive disclosure](../principles/progressive-disclosure.md);
   - change only stale, misplaced, overlapping, or repeated content the ledger implicates; and
   - name truthful enforcement under [software quality enforcement](../development/software-quality-enforcement.md), adding machinery only for a demonstrated need.
3. Read the repaired surfaces once for semantic closure. Resolve only conflicts the repair caused, under the hierarchy and [minimal sufficiency](../principles/minimal-sufficiency.md). Never broaden the ledger.
4. Run the gate:

   ```sh
   ./rhino gate run --surface pre-push
   ```

5. Return `PASS_NO_CHANGE` when no edit was necessary, otherwise `PASS_CHANGED`. For deterministic findings this transaction caused, freeze their exact set, repair mechanically, and rerun step 4 only while the count of failing checks and violations strictly decreases and no new failure class appears. That measure is nonnegative and decreasing, so recovery terminates. Return `BLOCKED_TOOLING` if progress stops, a new or unrelated failure appears, or no verdict can be obtained.

## Terminal Contract

The only results are `PASS_NO_CHANGE`, `PASS_CHANGED`, `BLOCKED_INPUT`, `BLOCKED_CONFLICT`, `BLOCKED_TOOLING`, and `BLOCKED_INPUT_CHANGED`. Propagation repairs every authorized row; what it cannot decide or verify maps to a named blocker. Passing means good enough rather than perfect, and authorizes neither commit nor push. With unchanged inputs and repository state, another transaction produces no diff.
