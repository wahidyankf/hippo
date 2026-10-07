# Conformance Environment

This adapter validates runtime-supplied manifests and owns checkout identities, verified binary copies, receipt reads,
private deferral fixtures, and process-group execution. It implements the ports consumed by
`application.ConformanceService`; phase ordering, concurrency, capacity classification, and reconciliation remain in the
application.

Private tests preserve process lifecycle and signal assertions. Corpus tests bind the real application and this
environment through `tests/support/conformance_wiring.go`.
