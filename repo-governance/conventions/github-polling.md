# GitHub Polling

Space checks against GitHub at least **three minutes** apart. Never stream, never watch, never poll in a loop.

## Requirements

- One query, then wait three minutes before the next. That applies to a pull request's checks, a workflow run, a ruleset, and anything else read through the API or `gh`.
- Never use a watching or following mode. `gh run watch` and its equivalents hold a connection open and issue requests continuously on the caller's behalf, which is the behaviour this rule exists to prevent.
- When a run is expected to take longer than the interval, wait for the duration the run actually needs rather than the interval. A gate that takes eight minutes deserves one check at eight minutes, not three at three.
- Read the result before deciding to check again. A second identical query issued without reading the first is a query nobody needed.

## Why

Rate limits are shared across everything the account does, and exhausting them takes out the thing being waited on along with everything else. Polling faster also does not make a build finish sooner; it converts waiting into requests and leaves the wait unchanged.

## When Something Looks Stuck

Say so, and check again after the interval. A run that has not reported is not evidence of a problem — [pull request merge](pull-request-merge.md) covers the case where a re-triggered gate leaves the previous run's result standing.
