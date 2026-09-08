# GitHub Actions Storage

Stay inside the free allowance. The budget is not a target to approach; it is a line that, once crossed, turns a public repository's CI into a bill.

## Requirements

When changing a workflow:

- Declare `retention-days` on **every** artifact upload. An upload without one inherits the repository default, which is the thing most likely to be raised later by someone who did not know these existed.
- Keep repository artifact and log retention at or below **7 days**.
- Keep the cache limit at or below **10 GB**, with retention at or below **7 days**.
- Keep the owner's Actions budget at **$0**. A budget above zero converts an accident into a charge instead of into a failure.

## Why $0 Specifically

A spending limit of zero makes the failure mode loud and free: the run stops. Any other limit makes it quiet and expensive, and the notification arrives after the money.

## Practical Consequences

- Prefer a job that recomputes over one that stores. A build cache that saves ninety seconds and stores a gigabyte for a week is a bad trade.
- Prefer conditional jobs to unconditional ones where the question is genuinely narrow — a consumer-boundary check that runs only when the consumer boundary changed.
- Disable Go module caching in release-asset jobs deliberately, so a release build is reproducible from nothing rather than from whatever a cache held.

These are checked by behaviour: scenarios assert the retention declarations and the release jobs' cache settings, so a workflow change that forgets one fails the gate rather than the invoice.
