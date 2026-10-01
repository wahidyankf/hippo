#!/bin/sh
# Enforce the 120-character Markdown line length across the whole tree.
# The rule and its exemptions are in
# repo-governance/conventions/markdown-line-length.md; .markdownlint-cli2.jsonc
# enables MD013 alone so this gate judges nothing else.
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
exec "$root/node_modules/.bin/markdownlint-cli2"
