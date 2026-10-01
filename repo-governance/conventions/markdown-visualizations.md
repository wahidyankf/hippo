# Markdown Visualizations

Diagrams are plain ASCII art in a `text` fenced block. Not Mermaid, not an image, not a rendered file checked in beside
its source.

`rhino md mermaid validate`, with `authoring-rule: plain-text` declared in [`repo-config.yml`](../../repo-config.yml),
refuses any Mermaid block.

## Why ASCII

This repository's Markdown is read in terminal editors without a renderer, where Mermaid shows as its source. ASCII is
the same text in every reader: an editor, a diff, a search result, and a terminal. An image is worse than either: it is
opaque to review, to diff, and to search.

ASCII has two known weaknesses, and the requirements below answer both. A screen reader cannot follow a drawing, so the
meaning lives in prose too. A drawing goes silently wrong when a box is added without redrawing its lines, so a diagram
changes in the same commit as the structure it shows.

## Requirements

- Use only printable ASCII: `+`, `-`, `|` for boxes and lines, and `>`, `<`, `^`, `v` for arrow heads. Box-drawing
  characters and arrows such as `→` render at ambiguous widths in some terminals.
- Keep every line within the [Markdown line length](markdown-line-length.md).
- Precede each diagram with one sentence that states what it shows.
- Every relationship a diagram shows also appears in prose near it. A diagram is a second way to read something, never
  the only way, which is what makes it safe for a reader who cannot see it at all.
- Label every box. Where a category matters, name it in the label, such as `(system)`; position and line style never
  carry meaning alone.
- Where a graph would cross its own edges, draw one row per relationship, grouped by the component it starts from.

Directory trees are not diagrams; they stay in `text` blocks in their usual form.
