# Host

Host adapters normalize macOS and Linux operating-system evidence into portable policy samples.

Linux reads kernel files with the caller's context, using one read-only, nonblocking, close-on-exec descriptor and
synchronous polls bounded to 10 ms. Cancellation discards partial bytes and returns after the descriptor's close
attempt; close failures remain errors. Actual read EOF ends successful acquisition. A writerless FIFO waits for
readiness instead of inventing initial EOF, and optional evidence reads stop when cancellation is observed.

`SystemCollector.ReadFileContext` takes precedence over the compatible legacy `ReadFile` callback. Legacy callbacks
remain synchronous and cannot be preempted if they ignore cancellation. `RootedContextFileReader` retains the legacy
reader's cleaned absolute path mapping for isolated test evidence. Darwin collection retains supervised command probes.
The rooted Darwin file adapter preserves metadata errno and uses the descriptor loop for ordinary files. Named pipes use
a fixed `/bin/cat` with an absolute operand, private stdout and discarded stderr. Every successful start has one joined
wait; cancellation kills and reaps only that child before returning. This preserves actual empty and content EOF despite
Darwin's missing FIFO writer-close readiness events. Narrow integration tests keep writers held until reader
cancellation and retirement, and verify writerless child retirement separately.

Nonblocking flags do not preempt arbitrary regular filesystem or device kernel stalls. The metadata preflight retains
the existing trusted-host pathname replacement assumption; it is not a symlink sandbox.
