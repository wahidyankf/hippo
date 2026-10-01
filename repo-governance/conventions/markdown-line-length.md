# Markdown Line Length

Every line of Markdown in this repository is at most 120 characters, so a document reads in a terminal editor without a
renderer, wrapping, or horizontal scrolling. Diagrams in it are ASCII, under
[Markdown visualizations](markdown-visualizations.md).

## Requirements

- The limit covers every line: prose, headings, list items, table rows, and fenced code.
- Prettier wraps prose at 120. Lines it cannot break are fixed by hand:
  - a table whose aligned width exceeds 120 has its cells shortened, is split into narrower tables, or becomes a list;
  - a long link target moves to a reference-style definition, which is exempt;
  - a long command in a code block continues on the next line with `\`.
- A fenced example of single-line output may be wrapped, with continuation lines indented, only with a sentence before
  it saying the real output is one line.
- Generated adapters comply through their source: the route text in [`repo-config.yml`](../../repo-config.yml).
- A table a test reads, such as the error-code table in `docs/reference/exit-codes.md`, stays a table.

Exempt: `CHANGELOG.md`, archived plans under `plans/done/`, and fixture bytes under `specs/fixtures/`.

## Enforcement

`markdownlint-cli2` runs rule `MD013` alone, configured in [`.markdownlint-cli2.jsonc`](../../.markdownlint-cli2.jsonc),
as the `markdown-line-length` gate on `pre-push`, `pull-request`, and `main`. Prettier's Markdown settings live in
[`.prettierrc.json`](../../.prettierrc.json).
