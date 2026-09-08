# Pull Request Body

The body is what a reviewer is owed. It exists to make the diff reviewable, not to summarize it — the diff is already there.

## Requirements

- **Why this change exists.** The failing case, the missing rule, the request. A reviewer who cannot tell why is reviewing spelling.
- **What was decided, and what was rejected.** An alternative considered and declined is the single most useful thing a body can carry, because it is the thing the diff cannot show.
- **How it was proved.** Name the gate, the scenario, or the transcript. "Tested locally" is not evidence; the name of the test that failed before and passes now is.
- **What is deliberately not in it.** Scope excluded on purpose, so its absence reads as a decision rather than an oversight.
- **Anything that would surprise.** A reversal, a deviation from a plan, a rule this change makes an exception to.

## Requirements of Tone

Write plainly. Never claim a check passed without having read its output, and never describe work as complete when part of it was skipped — say which part and why.

Where the change came from a plan, link the plan and say which delivery unit this is.

## Draft First

Open as a draft. Marking it ready re-triggers the gate on the `ready_for_review` event, so mark it ready first and read the run that follows; the run from before is not the run that will be required. See [pull request merge](pull-request-merge.md).
