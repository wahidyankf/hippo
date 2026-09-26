# Learnings: gate-shell-static-analysis

<!-- Append observations during execution. Resolve every entry before archival. -->

- 2026-09-26 — `npm test` run from a shell that exports `HIPPO_CONFIG` fails four end-to-end scenarios with
  `hippo.args.invalid`; unsetting that variable alone makes the full gate pass. Open: route or discard before
  archival.
