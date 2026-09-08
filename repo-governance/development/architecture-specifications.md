# Architecture Specifications

`specs/architecture.md` is the canonical as-built C4 model: system context, containers, components, and the dynamic view of a guarded run.

## As-Built, Not As-Intended

The model describes what exists. A boundary drawn because it was planned, and never built, is worse than no diagram — a reader trusts it and reasons from a system that is not there.

Synchronize the views with the **final** boundaries in the same change that moves them. Not before, when the shape is still being decided, and not after, when the change has already landed and the diagram is briefly a lie.

## Diagrams

Mermaid, under [markdown visualizations](../conventions/markdown-visualizations.md): node labels at most 32 graphemes, edge labels at most 24, colour from the declared palette only.

Every relationship a diagram shows also appears in the prose beneath it. The prose is not a summary of the diagram; it is the searchable, screen-reader-reachable form of the same claims, and it is what makes the diagram optional for the reader who cannot see it.

## Constraints Belong Here

The architectural constraints — the guard supervises only the process group it started, remote guards never signal one another, HIPPO holds no defaults about the work it guards — are part of the model rather than commentary on it. A constraint recorded only in code is a constraint the next design discussion will not know about.

Impact on this document is assessed before every change, alongside `specs/behaviours/`. See [specification maintenance](specification-maintenance.md).
