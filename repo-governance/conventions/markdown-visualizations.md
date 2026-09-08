# Markdown Visualizations

Diagrams are Mermaid. Not ASCII art, not an image, not a rendered file checked in beside its source.

`rhino md mermaid validate` checks every diagram against the limits and the palette declared in [`repo-config.yml`](../../repo-config.yml).

## Why Mermaid

An ASCII diagram is unreachable by a screen reader, unmaintainable under a rename, and silently wrong the moment a box is added without redrawing the lines around it. An image is worse: it is opaque to review, to diff, and to search. Mermaid is text that renders, so a diagram change is a diff a reader can read.

## Requirements

- Node and state labels: at most **32 graphemes**. Edge and transition labels: at most **24**. A label that will not fit is a label describing more than one thing; split the node or move the detail into prose beneath the diagram.
- Colour comes from the declared palette and nowhere else. The palette is Okabe–Ito, chosen because it stays distinguishable under the common forms of colour blindness.
- Contrast is a constraint, not a preference: `#029E73` carries black text, never white. The validator enforces the pairing.
- Every relationship a diagram shows also appears in prose near it. A diagram is a second way to read something, never the only way — which is what makes it safe for a reader who cannot see it at all.

## Adding the First Diagram to a Tree

The palette must be declared before a `classDef` can name a colour. An empty colour list in `repo-config.yml` fails every diagram that styles anything, which is the intended order: declare what is allowed, then draw.
