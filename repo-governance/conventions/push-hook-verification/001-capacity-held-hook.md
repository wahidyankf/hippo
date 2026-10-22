---
description: >-
  Allows one authorized push of a task-owned branch to skip the pre-push hook the compute guard stopped before any
  check started, when the hosted PR gate mirrors the hook and passes on the same head. Adopted from ose-rules at
  2872299bd434c46ca4c57417079ca9c8fc3b4e54.
when_to_use: >-
  Use when the compute guard reports exit 124 with a never-started receipt for the pre-push hook and waiting for
  capacity would stall an otherwise ready task branch.
---

# Capacity-Held Hook

[Back to Push Hook Verification](../push-hook-verification.md)

A hook held back by capacity has run nothing, so it holds no finding to fix. Retrying once the condition clears stays
the default. This module states the one exception.

## The Exception

One push of a task-owned branch may skip its pre-push hook when every condition holds:

1. The compute guard stopped the hook before any check started: exit `124` with a `never-started` receipt naming a
   capacity or pressure reason. A hook that started, was shed mid-run, or reported any check result never qualifies.
2. A human authorized that one push by name. The authorization is spent by the push; an agent never authorizes its own
   bypass and never carries an earlier approval into this one.
3. The outbound safety screen passed for the range being pushed. The screen has no bypass; run
   `scripts/public-safety/check.sh` directly for the range when the hook that normally carries it is the one held back.
4. The hosted pull-request gate repeats the hook's deterministic checks and a merge cannot skip it, and the push opens
   or updates the pull request that runs it. The `Quality gate` job in `.github/workflows/pr-quality-gate.yml` is that
   mirror.

## After the Push

- Record in the task record: the receipt, the authorizing human, the safety-screen result, and the pull request.
- The branch does not merge until the hosted gate passes on the exact pushed head. A later push replaces that head and
  owes a fresh pass; nothing about this exception waives it.
- Suites the pull-request gate does not run, such as the full suite, race detection, and the vulnerability scan in
  `scripts/test.sh` via the post-merge `main-test` workflow, stay owed where their own standards place them. This
  exception moves no suite.

## What This Never Covers

- A hook that ran and failed, however unrelated the failure looks. Fix the cause; a capacity receipt from an earlier
  attempt does not convert a failure into a deferral.
- Skipping a hook to publish to `main` directly, or in a checkout with no hosted mirror.
- Weakening, muting, or removing any check, locally or hosted, to make room for this exception.

**Enforcement: unenforced by decision**, because task ownership, human authorization, and receipt authenticity are
judgements no mechanical check settles. Review verifies the recorded receipts against the hosted gate's result.
