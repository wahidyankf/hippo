# Plans

A plan is a working record. It says why work exists, what it will change, and what will prove it finished. It is not a description of the tool: [`specs/`](../specs/README.md) is, and where a plan and a specification disagree the specification is right and the plan is a record of what someone once intended.

Most work here needs no plan. One delivery unit, one pull request, done. A plan earns its cost when work spans several units or several sessions, or when the decision behind it is one a reader will later want the argument for. Writing one requires an explicit request; see the [plans convention](../repo-governance/conventions/plans.md).

## The Stages

```mermaid
flowchart LR
    Ideas["ideas/"] --> Backlog["backlog/"]
    Backlog --> Active["in-progress/"]
    Active --> Done["done/"]

    classDef sketch fill:#CA9161,stroke:#000000,color:#000000
    classDef formal fill:#0173B2,stroke:#000000,color:#FFFFFF
    classDef record fill:#029E73,stroke:#000000,color:#000000

    class Ideas sketch
    class Backlog,Active formal
    class Done record
```

`ideas/` holds two-pagers that are still arguments. `backlog/` and `in-progress/` hold formal plans, queued and running. `done/` holds the delivery record under the date it completed.

A plan occupies exactly one stage. It is moved between them and never copied, because two folders describing the same work will disagree, and nothing says which one is current.

## What This Repository Plans

Work it can deliver by itself. A plan whose delivery would change another repository is planned where that work is coordinated, and reaches this one as its own change carrying its own evidence — the same boundary [rules propagation](../repo-governance/workflows/rules-propagation.md) draws for a rule. Planning a sibling's change from here would be the blind propagation that document exists to prevent.

## Directory Map

- [Backlog](backlog/README.md) holds complete plans that are ready and not started.
- [Done](done/README.md) holds completed plans as dated delivery records.
- [Ideas](ideas/README.md) holds two-pager briefs, grouped by urgency and importance.
- [In progress](in-progress/README.md) holds the plans being executed now.
