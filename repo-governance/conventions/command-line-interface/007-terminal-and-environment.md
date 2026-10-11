---
description: >-
  Fixes HIPPO diagnostic styling precedence and distinguishes optional decoration from required interactive terminal
  controls and clean machine-readable document bodies.
when_to_use: >-
  Use when adding colour, a pager, a progress indicator, hyperlinks, or interactive terminal behavior.
---

# Terminal and Environment

## HIPPO Local Adoption

Preserve HIPPO's published styling policy when adopting interactive monitoring. Reason: existing callers rely on
explicit forcing and presence-based `NO_COLOR` suppression; an interactive TUI also needs terminal controls when its
decoration is disabled. This locally adopted contract replaces the copied generic nonempty-variable and blanket
all-escape rules without changing diagnostic behavior.

## Diagnostic Styling

Accept exactly `auto`, `always`, and `never` for `--color`, defaulting to `auto`.

- Under `always`, enable styling even through a pipe or when `NO_COLOR` is present or `TERM` is empty or `dumb`.
- Under `never`, disable styling.
- Under `auto`, enable styling only when `NO_COLOR` is absent, `TERM` is nonempty and not `dumb`, and `HIPPO_COLOR=1`.

Treat any presence of `NO_COLOR`, including an empty value or `0`, as suppressing auto styling. Do not add a TTY probe
to this diagnostic policy: injected diagnostic writers have no reliable terminal capability. Forward the same policy to
TUI styling. The flag changes optional decoration, including colour, bold, underline, and hyperlinks; it does not change
state meaning, field values, or the data a caller receives.

For base-directory variables, keep the separate unset-or-empty fallback rule in
[Arguments and Flags](005-arguments-and-flags.md); the `NO_COLOR` presence rule does not govern path defaults.

Enforcement: existing diagnostic contracts and the TUI colour-forwarding contract distinguish auto suppression from
explicit forcing, per [quality gates](../../development/quality-gates.md).

## Inspection and Interactive Entry

Keep ordinary inspection content, record counts, field order, and exit meaning independent of terminal presentation.
Piped inspection remains readable without cursor movement, an alternate screen, progress indicators, or a pager.
Optional styling follows the explicit policy above; a pipe alone is not a diagnostic-writer capability probe.

Require both stdin and stdout to be TTYs before entering `hippo tui`. Refuse a non-TTY stream or `TERM=dumb` with usage
status `2` and `hippo.args.invalid`, directing the caller to status/watch. Parse, validate, and honor help before
changing terminal mode; help performs no TTY check.

Keep keyboard interaction and required cursor/alternate-screen controls working in a monochrome TUI. Disable only its
optional decoration; retain text distinctions without relying on colour. Restore owned terminal state before emitting
failure diagnostics. The TUI has no piped or streaming display mode.

Enforcement is gated: package tests cover CLI capability checks, while concrete PTY integration/E2E tests must prove
terminal entry, control emission, and restoration. Concrete runner, terminal-state handling, and PTY test/binding
implementations are present; required execution and final acceptance, including native host proof, remain open. Fake
cleanup callbacks establish no raw-mode or alternate-screen proof. [Quality gates](../../development/quality-gates.md)
declares these executing boundaries.

## Actual Machine-Readable Documents

Keep every actual machine-readable document body escape-free even with `--color always`, under
[Machine-Readable Output](004-machine-readable-output.md).

For `hippo tui`, global `--output json` selects only the existing failure body; it does not select JSON frames or
suppress interactive controls. Emit that body after terminal restoration. The adjacent human diagnostic retains its
styling policy; the JSON body remains independently parseable and clean.
