# Commit Authorization

Committing and pushing are separate from deciding what to change. This document governs when they are permitted.

## The Rule

Commit or push only when the user has authorized it, or when an approved plan's execution reaches a step that says to. Absent either, prepare the change, verify it, and stop.

Authorization is specific. Permission to commit one change is not permission to commit the next one, and permission given for a plan covers the steps that plan describes.

## Why It Is Separate

A convention that chooses a path — [integration path](integration-path.md), [thematic commits](thematic-commits.md) — describes how work reaches `main` when it is time. It does not decide that it is time. Keeping the two apart means a reader can follow every other convention completely and still be stopped by this one, which is the intended behaviour.

The cost of getting this wrong is asymmetric. An unmade commit is an inconvenience. A pushed commit is public, is in someone's clone within the hour, and cannot be unpublished — see [data safety](public-repository-data-safety.md).

## What Is Always Permitted

Reading, running gates, writing to `local-tmp/` and `generated-reports/`, and preparing a change in the working tree. None of those publish anything.

## What Is Never Permitted

Pushing to `main` directly. The ruleset refuses it for every actor including the owner, and there is no bypass; work reaches the trunk through a pull request. Bypassing a failing hook is governed by [push hook verification](push-hook-verification.md) and is not authorized by anything here.
