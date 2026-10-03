# Delegated Agent Concurrency

At most **three** delegated agents run at once beside the main thread, which is the fourth.

## Requirements

- A delegated agent is any agent a session spawns to work on its behalf: a subagent, a background agent, a spawned
  agent. The main thread that orchestrates them takes no slot.
- The count covers every delegated agent alive in the session, foreground or background, at any depth: an agent that a
  delegated agent spawns takes a slot of its own.
- It binds in every harness whose session can spawn delegated agents, whatever that harness calls them: Claude Code's
  `Agent` tool, Codex's spawned agents, and OpenCode's `task` tool alike.
- Work beyond three waits until a running agent returns; it is never launched over the cap. A workflow that runs tasks
  in parallel, such as [PR review](../workflows/quality/pr-review.md)'s specialists, runs them within it.
- Only the user, or an approved plan that states a different number for its own work, changes the cap.

## Why

Each delegated agent is a whole session with its own context, model calls, and tool processes, competing with every
other task on the same workstation and the same rate limits. Past a few, the orchestrator also cannot verify what comes
back as fast as it arrives, and an unread result is not a finished one.

## Enforcement

None mechanical, by decision: the owner declined a hook or a harness concurrency setting for it. The cap depends on the
orchestrating session's attention, and review verifies it. A fourth delegated agent alive at any moment, at any depth,
is a violation.
