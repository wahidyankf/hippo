#!/usr/bin/env bash
# Format exactly the staged or immutable-range paths Rhino selected. The runner
# supplies a disposable index snapshot for mutation and verifies cleanliness in
# pull-request replay, so this script never discovers a broader working tree.
set -euo pipefail

for file in "$@"; do
	[[ -e "$file" || -L "$file" ]] || continue
	case "$file" in
	*.go)
		go tool goimports -w "$file"
		go tool gofumpt -w "$file"
		;;
	*.md | *.json | *.yaml | *.yml)
		prettier --write --ignore-unknown "$file"
		;;
	*.sh | hippo | .husky/commit-msg | .husky/pre-commit | .husky/pre-push)
		go tool shfmt -w "$file"
		;;
	*) ;;
	esac
done
