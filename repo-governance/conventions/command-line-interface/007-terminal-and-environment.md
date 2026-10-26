---
description: >-
  Fixes HIPPO diagnostic styling precedence and distinguishes optional decoration from guarded-child terminal
  ownership and clean machine-readable document bodies.
when_to_use: >-
  Use when adding colour, a pager, a progress indicator, hyperlinks, or interactive terminal behavior.
---

# Terminal and Environment

## HIPPO Local Adoption

Preserve HIPPO's published styling policy when presenting guarded work. Reason: existing callers rely on
explicit forcing and presence-based `NO_COLOR` suppression; a guarded child that inherits the controlling terminal
still needs ownership restored when it finishes. This locally adopted contract replaces the copied generic
nonempty-variable and blanket all-escape rules without changing diagnostic behavior.

## Diagnostic Styling

Accept exactly `auto`, `always`, and `never` for `--color`, defaulting to `auto`.

- Under `always`, enable styling even through a pipe or when `NO_COLOR` is present or `TERM` is empty or `dumb`.
- Under `never`, disable styling.
- Under `auto`, enable styling only when `NO_COLOR` is absent, `TERM` is nonempty and not `dumb`, and `HIPPO_COLOR=1`.

Treat any presence of `NO_COLOR`, including an empty value or `0`, as suppressing auto styling. Do not add a TTY probe
to this diagnostic policy: injected diagnostic writers have no reliable terminal capability. The flag changes optional
decoration, including colour, bold, underline, and hyperlinks; it does not change state meaning, field values, or the
data a caller receives.

For base-directory variables, keep the separate unset-or-empty fallback rule in
[Arguments and Flags](005-arguments-and-flags.md); the `NO_COLOR` presence rule does not govern path defaults.

Enforcement: existing diagnostic contracts distinguish auto suppression from
explicit forcing, per [quality gates](../../development/quality-gates.md).

## Inspection and Interactive Entry

Keep ordinary inspection content, record counts, field order, and exit meaning independent of terminal presentation.
Piped inspection remains readable without cursor movement, an alternate screen, progress indicators, or a pager.
Optional styling follows the explicit policy above; a pipe alone is not a diagnostic-writer capability probe.

HIPPO ships no interactive presentation to enter: `status`, `watch`, `history`, and `monitor` render the same text
and JSON with or without a terminal. A guarded child that inherits the controlling terminal owns it while it runs and
restores foreground ownership when it finishes.

Enforcement is gated: package tests cover CLI refusals, while integration tests prove guarded-child terminal ownership
and restoration. [Quality gates](../../development/quality-gates.md) declares these executing boundaries.

## Actual Machine-Readable Documents

Keep every actual machine-readable document body escape-free even with `--color always`, under
[Machine-Readable Output](004-machine-readable-output.md). The adjacent human diagnostic retains its
styling policy; the JSON body remains independently parseable and clean.
