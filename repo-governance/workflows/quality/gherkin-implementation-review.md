# Gherkin Implementation Review

## Entry

A change has added or modified Gherkin scenarios, and an implementation claims to satisfy them.

## Sequence

1. **Freeze the scenario list.** Every scenario added or changed by this work, enumerated before review starts.
2. **For each scenario, locate the implementation** — the code path that makes it true — and the test that would fail if
   that path broke. Both, separately.
3. **Assign exactly one status:**

   | Status        | Means                                                                         |
   | ------------- | ----------------------------------------------------------------------------- |
   | implemented   | the behaviour exists and a test fails without it                              |
   | untested      | the behaviour exists but nothing fails when it breaks                         |
   | unimplemented | the scenario describes behaviour that does not exist                          |
   | drifted       | the implementation does something the scenario no longer accurately describes |

4. **Resolve `drifted` by deciding which is wrong.** Either the scenario was superseded and must be rewritten, or the
   implementation diverged and must be corrected. Editing the scenario to match whatever the code happens to do is not a
   resolution; it converts a specification into a description.
5. **Record every status** with the implementation path and the test path.

## Exit

Every frozen scenario carries a status, and no scenario remains `untested`, `unimplemented`, or `drifted` without an
explicit decision recorded against it.

## Assertion Theater Is a Failure, Not a Pass

A test that executes the scenario's steps and asserts nothing that could distinguish success from failure is `untested`,
not `implemented`. So is one whose only assertion is that the code ran without throwing.

The check is mechanical: break the behaviour and see whether the test fails. If it still passes, it was never
establishing anything.

Recording this as a failure matters more than it appears. Such a test is worse than no test: it occupies the place where
a real one would go, it is counted in coverage, and it will be trusted by everyone who does not read it.

## Why This Is a Distinct Review

A test suite proves that the tests pass. It does not prove that the tests correspond to the scenarios someone agreed to,
and it cannot notice a scenario nobody implemented — an unimplemented scenario has no failing test, because it has no
test.

That gap only closes by walking the scenarios themselves. One canonical review does this; a repository that has several
overlapping versions of it has several places for a scenario to be missed.
