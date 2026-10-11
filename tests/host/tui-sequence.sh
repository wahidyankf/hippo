#!/usr/bin/env bash
# Drive the capture/observation protocol through the typed evidence recorder.
set -euo pipefail

main() {
	local argument root
	for argument in "$@"; do
		if [[ "$argument" == --help || "$argument" == -h ]]; then
			printf '%s\n' 'Usage: tui-sequence.sh --host HOST --binary ABSOLUTE_BINARY --evidence ABSOLUTE_DIRECTORY [--prepare|--step ID|--observation FILE|--finalize]'
			return
		fi
	done
	if [[ -n "${HOST_EVIDENCE_VERIFIER:-}" ]]; then
		exec "$HOST_EVIDENCE_VERIFIER" --record "$@"
	fi
	root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
	cd -- "$root"
	exec go run ./tests/host/evidence-verifier --record "$@"
}

main "$@"
