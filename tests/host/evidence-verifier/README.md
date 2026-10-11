# Evidence Recorder and Verifier

This command checks the execution plan's complete ordered host protocol. Run `go test -count=1 ./tests/host/...`.
Synthetic positives model a complete format; the retained genuine REDs and negative assertions prove rejection. Actual
host input, capture provenance, and visual judgment still require the operator and an independent reviewer.

## Prepare

Use absolute paths for `--binary`, `--evidence`, and `--source-root`. The evidence directory must be fresh, private, and
ignored by the candidate checkout. Supply `--config-root` once per actual imported configuration file/tree and per
captured effective mapping inventory. Resolved configuration trees may contain no symlinks; name each resolved import
separately. Preparation fingerprints the full revision, dirty paths, binary SHA-256/embedded identity, and
configuration. A later change invalidates the record.

The operator supplies `--fixtures FILE`, a JSON object with `running` and `queue` arrays containing at least two unique
actual IDs each. Selection is established from real unique labels/status plus job details, never guessed IDs. The
operator prepares the target with a visible `HIPPO_HOST_EVIDENCE_OWNER:` marker followed by its 32-hex ownership token.
Preparation verifies that marker and records no completed action.

Pass `--owner-token TOKEN` and the explicit `--pane-id`. Tmux and Neovim require an isolated `--socket` starting with
`hippo-tui-`; Neovim also needs its absolute `--editor-server` RPC socket. WezTerm uses the operator's task-owned live
pane. The recorder never creates or retires a host or changes unrelated panes.

```bash
tests/host/tui-sequence.sh --host "$HOST_KIND" --binary "$HIPPO_BINARY" --evidence "$HOST_EVIDENCE" \
  --prepare --source-root "$SOURCE_ROOT" --config-root "$HOST_CONFIG" --fixtures "$FIXTURE_IDS" \
  --pane-id "$TASK_PANE" --owner-token "$OWNER_TOKEN" --socket "$TASK_SOCKET"
```

Add the required editor server for Neovim; omit the tmux socket for WezTerm.

## Drive and Inspect

Use `--step ID --expect TEXT` for the next ordered action. Automatic shared tmux/Neovim input resizes the native host,
checks the reported pane/window dimensions, sends actual keys, and saves native before/after captures. It awaits the
visible predicate for five seconds; `--wait` accepts a positive duration up to 30 seconds. A failed attempted action
stays failed. Start a fresh evidence run after a failure rather than erasing or passing that action.

For WezTerm, host shortcuts, separate exits, faults, or busy resize, the operator supplies actual native action evidence
using `--input-receipt FILE --before-capture FILE`. `--after-capture FILE` imports the exact native after bytes;
otherwise the recorder captures the current target and waits. Native resize must report the requested viewport. The
receipt uses one `name=value` line per field, with no duplicates:

- `origin=host-native-input`, `host`, `step`, `input`, `pane`, `ownerToken`, and `viewport=WIDTHxHEIGHT`;
- `binarySHA256`, `beforeCaptureSHA256`, and `captureSHA256`, matching the candidate and exact imported/captured bytes;
- `beforeCaptureStartMonotonicNS`, `beforeCaptureEndMonotonicNS`, `captureStartMonotonicNS`, and
  `captureEndMonotonicNS`, actual positive integer nanosecond boundaries for the before/after capture windows;
- `command`, the actual raw native command/input invocation, and its integer `status`;
- for exits: `actualExit`, `modesBefore`, `modesAfter`, `foregroundBefore`, `foregroundAfter`, and `hostUsable`;
- for host navigation: actual `paneBefore`, `paneAfter`, and `paneReturn` when present;
- for editor actions: actual `editorModeBefore` and `editorModeAfter`.

Each capture window is bounded by three seconds. The interval from before-capture end to after-capture end is at most
500 ms for ordinary actions, navigation, and resize. Completed manual refresh allows three seconds only when the step
ID, `r` input, and viewport exactly match `40x16-r`, `60x20-r`, or `100x30-r` in the canonical sequence. The predicate
polling `--wait` does not replace these receipt clock bounds. A completed `r` receipt does not prove a separate
pending-refresh acknowledgment or busy-worker responsiveness; their existing 500 ms checks remain separate.

These fields bind retained artifacts; they cannot authenticate an invented native invocation. Root-provided actual
command/input/status receipts and trusted captures are mandatory for real acceptance. Fault redirections are proposals
until a real probe establishes the cause; a refused or earlier failing launch remains a failed condition.

The capture is initially `captured`, not `passed`. Copy the pending step JSON into an observation file, preserve all
captured IDs, inputs, dimensions, predicates, and artifact references, and fill its visually inspected before/after
state, actual predicate, native facts, and outcome. Attach with `--observation FILE`. The recorder validates the exact
hashes, expected transition, and preceding state. `refreshCount` counts observed successful sample transitions; it does
not assert an internal counter. Successful observation age must visibly reset.

WezTerm Cmd-B additionally requires `--operator-events FILE` with `origin=operator-key-event`, `host=wezterm`,
`step=wezterm-cmd-b-navigation`, actual pane IDs, and `input=Cmd-B,e,f,h` for the configured sequence. The original
`input=Cmd-B,h` synthetic contract remains supported. CLI availability or a success checkbox cannot replace an event.

## Finalize

Run the runner with `--finalize`, then independently verify:

```bash
go run ./tests/host/evidence-verifier --host "$HOST_KIND" --evidence "$HOST_EVIDENCE" --binary "$HIPPO_BINARY"
```

Verification checks every ordered action, candidate/config identity, artifact digest, plain native text, terminal-cell
bounds, transitions, exit restoration, and host facts. It rejects missing, reordered, timed-out, failed, or stale proof.
Usage failures return 2; standalone evidence rejection returns 1; recording/I/O failures return 125. Help returns 0. A
coherently forged complete artifact set can pass offline consistency checks. Trusted native boundaries and independent
visual inspection remain necessary; no tooling test establishes that a real host sequence passed.
