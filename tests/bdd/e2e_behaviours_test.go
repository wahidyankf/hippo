package bdd_test

import (
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
)

// The end-to-end adapter skips every @e2e-exempt scenario, so a feature that holds one is
// referenced scenario by scenario: a whole-file `*` would mark each exempt scenario stale (B06).
// Each exempt scenario is covered by its exemption comment in the feature file instead.
// @scenario specs/behaviours/admission.feature: Unreadable host evidence names its own reason
// @scenario specs/behaviours/conformance.feature: A signal ends a conformance run with the signal status
// @scenario specs/behaviours/evidence.feature: A refused evidence root stops a run before launch
// @scenario specs/behaviours/evidence.feature: A refused coordination write stops a run before launch
// @scenario specs/behaviours/evidence.feature: Status reports a coordination lock it cannot open as a refused write
// @scenario specs/behaviours/evidence.feature: A refused never-started receipt is reported rather than hidden by the signal
// @scenario specs/behaviours/evidence.feature: A passed run records the CPU and wall time its child used
// @scenario specs/behaviours/evidence.feature: A history row carries no command, argument, working directory, or path after a passed run
// @scenario specs/behaviours/evidence.feature: A waited grandchild counts toward the run's child CPU
// @scenario specs/behaviours/evidence.feature: A failed run still records the CPU its child used
// @scenario specs/behaviours/evidence.feature: Compacting a summary keeps its child usage
// @scenario specs/behaviours/evidence.feature: A summary written before child usage was recorded still reads
// @scenario specs/behaviours/execution.feature: Reservation coordination rejects every compatibility class as a protocol mismatch
// @scenario specs/behaviours/execution.feature: A port-lease root that refuses writes stops a run before launch
// @scenario specs/behaviours/execution.feature: A child-owned reserved exit stays a child failure
// @scenario specs/behaviours/execution.feature: Exclusive coordination refuses an admission wait it cannot apply
// @scenario specs/behaviours/execution.feature: Explicit display labels are visible before reservation ownership is published
// @scenario specs/behaviours/execution.feature: Queued work publishes its display label before admission
// @scenario specs/behaviours/execution.feature: A labeled waiter keeps its original resource demand and FIFO position
// @scenario specs/behaviours/execution.feature: A default display label uses only the executable basename
// @scenario specs/behaviours/execution.feature: Invalid explicit display labels fail before registration
// @scenario specs/behaviours/execution.feature: An exclusive run publishes its display label
// @scenario specs/behaviours/execution.feature: Publishing a label leaves exclusive coordination and the session schema unchanged
// @scenario specs/behaviours/execution.feature: Inherited runs preserve the original owner's display label
// @scenario specs/behaviours/execution.feature: A display annotation is removed when its run ends
// @scenario specs/behaviours/execution.feature: A failed annotation write or removal leaves the run outcome unchanged
// @scenario specs/behaviours/execution.feature: A failed annotation write or removal is reported without private paths
// @scenario specs/behaviours/execution.feature: Display annotation cleanup removes only the annotation of a proven stale owner
// @scenario specs/behaviours/execution.feature: Owners without display annotations stay visible with unavailable labels
// @scenario specs/behaviours/execution.feature: Owners of mixed versions keep their admission accounting and FIFO order
// @scenario specs/behaviours/public-cli.feature: Version identifies the exact build
// @scenario specs/behaviours/public-cli.feature: JSON status exposes the stable evidence schema
// @scenario specs/behaviours/public-cli.feature: Status exposes live exclusive compatibility owners
// @scenario specs/behaviours/public-cli.feature: Status classifies invalid exclusive compatibility state
// @scenario specs/behaviours/public-cli.feature: History lists a value this version does not know as recorded
// @scenario specs/behaviours/public-cli.feature: A signal ends watch with the signal status
// @scenario specs/behaviours/public-cli.feature: JSON status fails closed on corrupt coordination state
// @scenario specs/behaviours/public-cli.feature: JSON status gives active coordination markers precedence
// @scenario specs/behaviours/public-cli.feature: JSON status rejects overflowing queued demand
// @scenario specs/behaviours/public-cli.feature: Command discovery lists the public commands in help
// @scenario specs/behaviours/public-cli.feature: Help identifies the HIPPO acronym
// @scenario specs/behaviours/public-cli.feature: Help states the exit-code contract
// @scenario specs/behaviours/public-cli.feature: Release discovery uses grouped help
// @scenario specs/behaviours/public-cli.feature: Requested help wins over every other flag
// @scenario specs/behaviours/public-cli.feature: A flag mistake without help remains a usage mistake
// @scenario specs/behaviours/public-cli.feature: Shell completion is generated on demand
// @scenario specs/behaviours/public-cli.feature: An unknown command names itself and exits as a usage mistake
// @scenario specs/behaviours/public-cli.feature: A command group refuses an unknown or missing subcommand
// @scenario specs/behaviours/public-cli.feature: A flag value a command cannot accept is a usage mistake
// @scenario specs/behaviours/public-cli.feature: Arguments after the separator belong to the guarded command
// @scenario specs/behaviours/public-cli.feature: The release monitor output flag is not the global output format
// @scenario specs/behaviours/public-cli.feature: A failure body names the command wherever the global flags are placed
// @scenario specs/behaviours/public-cli.feature: An identity file that is present and invalid is named as invalid
// @scenario specs/behaviours/public-cli.feature: A run label without any source is a usage mistake
// @scenario specs/behaviours/public-cli.feature: Only usage errors print the command usage block
// @scenario specs/behaviours/public-cli.feature: Invalid explicit configuration is actionable
// @scenario specs/behaviours/public-cli.feature: JSON status says whether the resolved profile may use degraded admission
// @scenario specs/behaviours/public-cli.feature: Run validates its command boundary
// @scenario specs/behaviours/public-cli.feature: Commands without operands reject positional arguments
// @scenario specs/behaviours/public-cli.feature: Release summary assessment accepts healthy evidence
// @scenario specs/behaviours/public-cli.feature: Release summary assessment names rejected evidence as its own limit
// @scenario specs/behaviours/public-cli.feature: Release summary assessment reports an unusable summary as unreadable evidence
// @scenario specs/behaviours/public-cli.feature: Release summary assessment accepts standard input
// @scenario specs/behaviours/public-cli.feature: Release monitoring requires generic health inputs
// @scenario specs/behaviours/public-cli.feature: Release monitoring refuses a usage mistake before collecting evidence
// @scenario specs/behaviours/public-cli.feature: Release output formats cannot share standard output
// @scenario specs/behaviours/public-cli.feature: The removed TUI command is an unknown command
// @scenario specs/behaviours/release.feature: Release overlap rejects failed health evidence
// @scenario specs/behaviours/release.feature: Release overlap rejects an unresponsive routed journey
// @scenario specs/behaviours/release.feature: Release overlap accepts a two-CPU host with busy CPU and healthy routed journeys
// @scenario specs/behaviours/release.feature: Release overlap rejects a two-CPU host whose routed journey is outside the budget
// @scenario specs/behaviours/release.feature: Release overlap keeps the CPU ceiling for a two-CPU host without routed evidence
// @scenario specs/behaviours/release.feature: Release overlap keeps the CPU ceiling for a four-CPU host
// @scenario specs/behaviours/release.feature: Release overlap accepts swap-ins with low available memory
// @scenario specs/behaviours/release.feature: Release overlap judges swap-outs against the memory-scaled threshold
// @scenario specs/behaviours/reservations.feature: A coordination lock held through the whole wait still leaves a never-started receipt
// @scenario specs/behaviours/reservations.feature: A signal while a run waits for the coordination lock leaves a never-started receipt
// @scenario specs/behaviours/reservations.feature: A signal to a queued run ends it with the signal status and keeps its receipt
// @scenario specs/behaviours/reservations.feature: A queued run stopped by a signal writes no lifetime summary
// @scenario specs/behaviours/reservations.feature: Schema two configuration preserves schema one compatibility
// @scenario specs/behaviours/reservations.feature: A memory request too large to represent requires replanning
// @scenario specs/behaviours/reservations.feature: Status and development summaries expose schema five reservation totals
// @scenario specs/behaviours/resource-tiers.feature: *
// @scenario specs/behaviours/terminal.feature: *

func TestE2EBehaviours(t *testing.T) { runBehaviours(t, contract.E2E) }
