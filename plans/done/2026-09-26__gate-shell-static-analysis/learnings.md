# Learnings: gate-shell-static-analysis

<!-- Append observations during execution. Resolve every entry before archival. -->

- 2026-09-26 — `npm test` run from a shell that exports `HIPPO_CONFIG` fails four end-to-end scenarios with
  `hippo.args.invalid`; unsetting that variable alone makes the full gate pass. Discarded: already covered — the
  isolate-test-coordination-state backlog plan records this exact failure, its cause, and the `TestMain` scrub that
  fixes it, so a second owner would only drift from it.
