# Verification Layers

| Layer                  | Proves                                                         | Evidence                                  |
| ---------------------- | -------------------------------------------------------------- | ----------------------------------------- |
| programmatic           | parse, accessibility rules, layout and viewport assertions     | commands, exit classes, machine reports   |
| exploratory/usability  | tasks are understandable and flows survive realistic variation | scripted observations, sanitized findings |
| device-specific visual | intended composition at the declared devices and viewports     | accessible captures, comparison notes     |

## They Are Separately Dispositioned

Each applicable layer gets its own result. One combined "manually verified" line hides which layer was actually run, and
in practice the one that was run is the cheapest.

They catch structurally different things. Automation cannot notice that a correct interface is confusing. Exploration
cannot guarantee coverage. Neither reveals what only appears on a particular device. No two of them substitute for the
third.

## Every Assertion Is Explicit

A manual assertion states four things before it is executed:

1. **the procedure** — what is done, precisely enough to repeat;
2. **the expected observation** — what should be seen;
3. **the failure signal** — what would show it is wrong; and
4. **the evidence** — what is recorded, and where.

An assertion missing the failure signal cannot fail, and one missing the procedure cannot be repeated. Both produce a
tick that means only that somebody looked.

## Green Automation Does Not Close a User-Facing Change

If a required manual layer is unresolved, substantive completion is false — regardless of how green the pipeline is.

This is worth stating flatly because the pressure runs the other way. Automation finishes fast, produces a clear signal,
and feels conclusive; the manual layers are slow and produce prose. The layer that is easiest to skip is the one that
catches what users hit first.

## Not Applicable Is a Disposition

A change with no user-facing surface records the interface layers as not applicable, with a reason. It does not attach
screenshots to satisfy a checklist.

Ritual evidence is worse than none: it costs the same to produce, proves nothing, and teaches everyone reading it that
this section can be skimmed.
