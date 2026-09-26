# Learnings: neutralize-test-fixture-identifiers

<!-- Append observations during execution. Resolve every entry before archival. -->

## L1 — The complete gate failed on a workstation for a reason outside this plan

`npm test`, run directly on a workstation, failed twice for reasons the diff cannot reach. With the shell's
`HIPPO_CONFIG` inherited, four end-to-end behaviour scenarios failed with `hippo.args.invalid` "schema 3 requires
--resource-tier" and "compiled child exit 124/125 was not admitted after 40 attempts". With only that variable unset,
every scenario passed and `TestCommandLineInterfaceContract` failed `cli.args.double-dash-ends-options` with exit `125`,
because the binary under test joined the machine's shared evidence root. With `HIPPO_CONFIG` unset and `HIPPO_ROOT` an
empty directory, the gate passed.

**Disposition:** discarded as already owned. These are the first two rows of the reproduction table in the
[isolate test coordination state](../../backlog/isolate-test-coordination-state/README.md) plan, which removes the
defect. No second owner is created.

## L2 — The artifact fixtures read the index, not the working tree

One complete-gate run failed seven release-artifact scenarios with `copy fixture entry: lstat ... no such file or
directory`, because the index still listed the plan's backlog paths after they had been moved on disk and unstaged. With
the index and the working tree agreeing, the same scenarios passed.

**Disposition:** discarded as specific to this execution. The gate reported a real inconsistency correctly; the remedy
was to stage the move before running it, and no rule or test is missing.
