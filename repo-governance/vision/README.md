# Vision

## The Future This Repository Exists To Create

A workstation where nobody thinks about the workstation.

Local work competes for one machine: a test suite, a dev server, a build, a language server, and whatever else the day brought. The usual answers are all bad. Run everything and the machine thrashes, or the compiler is killed, or the laptop stops responding to the person using it. Run one thing at a time and the machine is idle most of the day. Tune concurrency by hand and the number is wrong the moment anything changes.

HIPPO's future is that none of this is anyone's problem. A command is prefixed with a guard, and the guard decides — from real host evidence rather than a guess — whether there is room, how much of the machine this work may have, and what to do when pressure arrives. Work that cannot run yet waits in a fair queue instead of failing. Work that must be shed is chosen deliberately, newest and least valuable first, and it exits with a code that says _retry me_ rather than _something broke_.

The measure of success is that the guard is invisible. Nobody reads its output on a good day. The commands are the same commands; they simply stop fighting each other.

## What That Rules Out

**Knowing what it is guarding.** HIPPO takes a command and runs it. It does not know what a test suite is, what a build is, or which task runner a repository uses. A default that suited one repository's layout would be a default that is wrong somewhere else, so there are none.

**Owning anything it did not create.** The guard supervises exactly the child process group it started. It never signals a process it does not own, on any machine, for any reason.

**Being load-bearing.** A repository that adopts HIPPO must still work without it. The guard arbitrates; it does not become a dependency of the thing it guards, which is also why nothing in this repository's own gates runs under it.

**Guessing.** Where the host cannot be measured, the answer is a refusal with a stable exit code, not an estimate. A guard that guessed would be a guard whose clean run means nothing.

## Directory Map

This directory holds only this document.
