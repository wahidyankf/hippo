#!/usr/bin/env bash
# Validate the one typed message Rhino projects from the canonical hook file or
# immutable pull-request range. The script never accepts a path or free-form
# argument, so callers cannot replace the declared semantic input.
set -euo pipefail

[[ ${RHINO_GATE_MESSAGE+x} ]] || {
	printf '[commit-message] Rhino supplied no declared message\n' >&2
	exit 2
}

printf '%s\n' "$RHINO_GATE_MESSAGE" | npm exec -- commitlint
