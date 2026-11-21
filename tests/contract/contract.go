package contract

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	messages "github.com/cucumber/messages/go/v34"
)

const (
	// FeaturePath points every adapter at the canonical recursive Gherkin corpus.
	FeaturePath = "../../specs/behaviours"
	// Unit identifies the deterministic in-process adapter.
	Unit = "unit"
	// Integration identifies the local filesystem and process adapter.
	Integration = "integration"
	// E2E identifies the compiled public-binary adapter.
	E2E = "e2e"
	// syntheticHostPressureReason documents scenarios that require deterministic host samples.
	syntheticHostPressureReason = "requires synthetic samples that cannot be injected through the compiled binary"
	hostEvidenceBoundary        = "host evidence"
	leaseOwnershipBoundary      = "lease ownership"
	processControlBoundary      = "process control"
	evidenceFilesystemBoundary  = "evidence filesystem"
	consumerHarnessBoundary     = "consumer harness"
	repositoryStateBoundary     = "repository state"
	releaseArtifactsBoundary    = "release artifacts"
	releaseWorkflowBoundary     = "release workflow"
	testHarnessBoundary         = "test harness"
)

// Reason sentences shared by many exemption rows, so each is written once.
const (
	reasonCoordinationState = "a compiled binary run cannot plant, hold, or time the coordination records that owners and waiters share"
	reasonCapacityPlan      = "a compiled binary plans with the real host CPU and memory, which cannot be fixed to a chosen value"
	reasonProcessControl    = "a compiled binary run cannot hold, signal, kill, or time its own child process groups on demand"
	reasonEvidenceFiles     = "a compiled binary run cannot age, seed, or count evidence files in its shared root"
	reasonConsumerHarness   = "a consumer manifest and its checkouts are driven by the harness, outside any one compiled binary run"
	reasonHostEvidence      = "a compiled binary reads the real host, so its samples, counters, and timing cannot be set to a chosen value"
	reasonRepositoryHistory = "a compiled binary run cannot be given an isolated Git history with a chosen commit, tag, or dirty state"
	reasonReleaseArchives   = "a compiled binary run cannot build or open a set of cross-platform release archives on demand"
)

// StepBinding keeps one canonical Godog expression adjacent to its handler.
type StepBinding struct {
	Pattern string
	Handler any
}

// Adapter describes one behavior layer and its exemption tag.
type Adapter struct {
	Name         string
	ExemptionTag string
}

// Exemption documents the boundary and reason that prevent one scenario from
// running through an adapter.
type Exemption struct {
	Scenario string
	Boundary string
	Reason   string
}

// liveLeaseExemption explains scenarios that cannot mutate real lease ownership.
const (
	liveLeaseExemption          = "would replace or contend with live process ownership outside the isolated binary fixture"
	reservationCapacityBoundary = "reservation capacity"
	// abandonedOwnerReason documents scenarios that need a guard to be gone while
	// the payload it launched is still running.
	// saturatedDeferralReason documents the loaded-gate deferral scenarios, which
	// assert on the end-to-end fixtures' own acceptance decision.
	saturatedDeferralReason = "the deferral decision belongs to the end-to-end fixtures outside the compiled binary boundary"
	abandonedOwnerReason    = "requires a reservation whose guard liveness is controlled independently of its process group, which cannot be staged through the compiled binary"
)

// ApprovedExemptions is the reviewed, exact adapter exemption inventory.
var ApprovedExemptions = map[string][]Exemption{
	Unit:        {},
	Integration: {},
	E2E: {
		{Scenario: "Healthy consecutive samples admit work", Boundary: hostEvidenceBoundary, Reason: syntheticHostPressureReason},
		{Scenario: "Stable macOS warning admits degraded work", Boundary: hostEvidenceBoundary, Reason: syntheticHostPressureReason},
		{Scenario: "Growing pressure defers degraded work", Boundary: hostEvidenceBoundary, Reason: "requires synthetic compressor counters that cannot be injected through the compiled binary"},
		{Scenario: "Strict work never uses degraded admission", Boundary: hostEvidenceBoundary, Reason: syntheticHostPressureReason},
		{Scenario: "A configured profile derived from balanced admits degraded work", Boundary: hostEvidenceBoundary, Reason: syntheticHostPressureReason},
		{Scenario: "A configured profile outside balanced's lineage never uses degraded admission", Boundary: hostEvidenceBoundary, Reason: syntheticHostPressureReason},
		{Scenario: "Balanced work falls back on a small runner", Boundary: hostEvidenceBoundary, Reason: "requires synthetic host capacity that cannot be injected through the compiled binary"},
		{Scenario: "The last-resort floor follows the minimal lineage", Boundary: hostEvidenceBoundary, Reason: "requires synthetic host capacity that cannot be injected through the compiled binary"},
		{Scenario: "Exhausted storage requires cleanup", Boundary: hostEvidenceBoundary, Reason: "requires synthetic disk evidence that cannot be injected through the compiled binary"},
		{Scenario: "Swap-out growth is normalized to the policy window", Boundary: hostEvidenceBoundary, Reason: "requires synthetic swap counters that cannot be injected through the compiled binary"},
		{Scenario: "Compressor growth requires both payload and growth", Boundary: hostEvidenceBoundary, Reason: "requires synthetic compressor counters that cannot be injected through the compiled binary"},
		{Scenario: "A strict transaction does not silently downgrade", Boundary: "task capacity", Reason: "requires a synthetic capacity mismatch that cannot be injected through the compiled binary"},
		{Scenario: "Automatic reservations divide capacity by profile owner shares", Boundary: reservationCapacityBoundary, Reason: "requires deterministic synthetic CPU and memory capacity unavailable through the compiled binary"},
		{Scenario: "Automatic owner shares follow the profile lineage", Boundary: reservationCapacityBoundary, Reason: "requires deterministic synthetic CPU and memory capacity, and the binary always plans with the configuration's filled share map"},
		{Scenario: "Automatic shares at maximum CPU and memory keep their exact four-share and two-share ceilings", Boundary: reservationCapacityBoundary, Reason: "requires maximum-width synthetic CPU and memory capacity unavailable through the compiled binary"},
		{Scenario: "Automatic CPU capacity is every host CPU up to the configured maximum", Boundary: reservationCapacityBoundary, Reason: "requires synthetic host CPU counts unavailable through the compiled binary"},
		{Scenario: "Profile concurrency offers every host CPU", Boundary: reservationCapacityBoundary, Reason: "requires synthetic host CPU counts unavailable through the compiled binary"},
		{Scenario: "Every development class consumes reservation capacity", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Explicit reservations may be smaller than automatic shares", Boundary: reservationCapacityBoundary, Reason: reasonCapacityPlan},
		{Scenario: "Reservation floors reject requests below one CPU or 256 MiB before enqueue", Boundary: reservationCapacityBoundary, Reason: reasonCapacityPlan},
		{Scenario: "A memory request below the floor exits 125 naming hippo.policy.replan-required before enqueue", Boundary: reservationCapacityBoundary, Reason: reasonCapacityPlan},
		{Scenario: "A negative CPU request exits 2 naming hippo.args.invalid before enqueue", Boundary: reservationCapacityBoundary, Reason: reasonCapacityPlan},
		{Scenario: "Vector admission is atomic", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Impossible reservation requires replanning", Boundary: reservationCapacityBoundary, Reason: "requires deterministic synthetic total capacity"},
		{Scenario: "Temporary exhaustion defers the waiter through its deadline with exit 124", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A capacity deferral writes a never-started receipt even when the wait budget is nearly spent", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A request larger than a shrunk shared pool is deferred without poisoning the root", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "FIFO head cannot be bypassed by a smaller request", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A reservation queue at its highest position restarts only after its stale epoch empties", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Inherited sessions reuse their fixed allocation", Boundary: "session inheritance", Reason: reasonCoordinationState},
		{Scenario: "Fixed allocations clamp concurrency mappings", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Stale identity cannot retain or inherit capacity", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Active exclusive compatibility blocks reservation takeover", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Malformed compatibility state remains fail closed during takeover", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Unsupported compatibility heavy-owner schema remains actionable", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Malformed compatibility service session remains fail closed during takeover", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Unreadable compatibility session inventory blocks reservation takeover", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Failed stale heavy cleanup blocks reservation takeover", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Unsupported coordination marker schema is a protocol mismatch", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Unsupported reservation ledger schema is a protocol mismatch", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Schema three refuses live legacy entries as a protocol mismatch", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A resource tier refuses an admission wait it would ignore", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Host pressure thresholds remain authoritative", Boundary: hostEvidenceBoundary, Reason: syntheticHostPressureReason},
		{Scenario: "Shedding selects one newest revocable owner", Boundary: processControlBoundary, Reason: "requires multiple controlled owners and synthetic critical pressure"},
		{Scenario: "Service shedding follows exhausted ephemeral owners", Boundary: processControlBoundary, Reason: "requires multiple controlled service owners and synthetic critical pressure"},
		{Scenario: "Remote shedding completes one victim before another election", Boundary: processControlBoundary, Reason: "requires multiple controlled process groups and bounded signal escalation"},
		{Scenario: "Remote observation never signals a replaced identity", Boundary: processControlBoundary, Reason: "requires controlled process-group identity replacement during remote observation"},
		{Scenario: "Remote selectors never signal another owner's child", Boundary: processControlBoundary, Reason: "requires a controlled unresponsive remote owner and process-group signal observation"},
		{Scenario: "Remote observation remains bounded by its deadline", Boundary: processControlBoundary, Reason: reasonCoordinationState},
		{Scenario: "Cancelling a run while another holder holds the release lock bounds its cleanup", Boundary: processControlBoundary, Reason: reasonCoordinationState},
		{Scenario: "A cancelled queued run leaves the queue although the coordination lock is briefly held", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Supervisor death cannot release a live child reservation", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Status rejects overflowing aggregate waiter demand", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Development summaries retain owners between host samples", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Custom profiles inherit reservation shares and fixed concurrency", Boundary: reservationCapacityBoundary, Reason: reasonCapacityPlan},
		{Scenario: "Owner-side shedding preserves the selected exit", Boundary: processControlBoundary, Reason: "requires injected storage-shedding selection during controlled owner supervision"},
		{Scenario: "Transactional owners are protected during ordinary shedding", Boundary: processControlBoundary, Reason: "requires controlled transactional owners and synthetic critical pressure"},
		{Scenario: "Transactional owners are the final emergency victim", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "A transactional emergency stop records a started safety receipt without automatic retry", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Shared owner limits remain conservative across configurations", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Active reservation capacity cannot shrink below live commitments", Boundary: leaseOwnershipBoundary, Reason: "requires mixed vector caps with a live owner and FIFO waiter in one isolated shared root"},
		{Scenario: "A ledger with an invalid or duplicate owner token fails admission closed and keeps its bytes", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A ledger with an unknown workload class fails admission closed and keeps its bytes", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A ledger with duplicate or out-of-order sequences fails admission closed and keeps its bytes", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A ledger with negative or inconsistent vectors fails admission closed and keeps its bytes", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A ledger with vectors that overflow when added fails admission closed and keeps its bytes", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Maximum-width admission cannot wrap vector capacity", Boundary: reservationCapacityBoundary, Reason: reasonCapacityPlan},
		{Scenario: "A ledger with allocated totals above capacity fails admission closed and keeps its bytes", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A ledger with an impossible owner and waiter structure fails admission closed and keeps its bytes", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Unknown identity errors never prune reservation accounting", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Missing live reservation accounting cannot initialize empty", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Runtime inputs remain canonical across consumer commands", Boundary: consumerHarnessBoundary, Reason: "requires temporary checkout symlinks and inherited process environment control"},
		{Scenario: "Status exposes privacy-safe labeled owner rows", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "History filters current labeled summaries", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "An unreadable history archive is not reported as a refused write", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "Watch emits only changed admission snapshots", Boundary: hostEvidenceBoundary, Reason: "requires injected stable host samples and deterministic cancellation timing"},
		{Scenario: "A signal while an observer collects a sample ends it with the signal status", Boundary: hostEvidenceBoundary, Reason: "requires an injected collector that is interrupted part way through one host sample"},
		{Scenario: "Release monitoring reports a signal but not its own duration", Boundary: releaseWorkflowBoundary, Reason: "requires an injected release capture that observes its own cancellation instead of live health endpoints"},
		{Scenario: "A signal during host admission sampling keeps a never-started receipt", Boundary: hostEvidenceBoundary, Reason: "requires injected host samples that hold admission sampling open at a deterministic pause"},
		{Scenario: "A run stopped while sampling is summarized as admission-cancelled and listed by history", Boundary: hostEvidenceBoundary, Reason: "requires injected host samples that hold admission sampling open at a deterministic pause"},
		{Scenario: "A host lost after launch stays a supervision failure", Boundary: hostEvidenceBoundary, Reason: "requires injected host evidence that becomes unreadable only after the child has started"},
		{Scenario: "A run HIPPO stops before launch is summarized as admission-failed", Boundary: hostEvidenceBoundary, Reason: "requires injected host samples that fail, or change the command, at a deterministic point in admission sampling"},
		{Scenario: "An admission-failed summary carries no child usage fields", Boundary: hostEvidenceBoundary, Reason: "requires host samples that fail, or a command that stops being executable, at a deterministic point in admission sampling"},
		{Scenario: "Consumer coordination never inherits the conformance caller session", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "A validated checkout directory cannot be replaced between phases", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "The created shared root cannot be replaced between phases", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Force-stop reap failure remains bounded", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Verified binary cleanup failures are reported", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Cancellation before a sequential command prevents its start", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "A verified HIPPO binary cannot change between command phases", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Commands execute one pinned verified HIPPO identity", Boundary: consumerHarnessBoundary, Reason: "requires replacing a temporary executable after verification while a consumer command invokes the provided identity"},
		{Scenario: "A conformance manifest whose HIPPO binary is not a regular executable file is rejected without exposing its path", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Missing command errors remain private", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Invalid checkout command errors remain private", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Independent bootstrap lanes run concurrently behind one phase barrier", Boundary: consumerHarnessBoundary, Reason: "requires deterministic timing and multiple failures across four temporary checkout processes"},
		{Scenario: "Development summaries retain the lifetime peak owner count", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Reservation protocol files survive evidence retention", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "Atomic coordination writes survive evidence retention", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "Concurrent evidence disappearance does not abort guarded work", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "Four manifest consumers preserve their checkout state", Boundary: consumerHarnessBoundary, Reason: "repository-owned manifest orchestration is outside the compiled HIPPO binary"},
		{Scenario: "Checkout aliases cannot name one consumer twice", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Every checkout is reconciled after any command failure", Boundary: consumerHarnessBoundary, Reason: "requires intentionally mutating temporary checkouts across failing manifest phases"},
		{Scenario: "Linux cgroup memory limits host capacity", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Linux reclaimable file cache stays available", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Linux without swap remains usable", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Linux PSI detects active memory contention", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Linux swap counts toward the allowance when counting is on", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "The swap allowance is the smaller of memory and swap, rounded up to whole GiB", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "A host that cannot count swap has no swap allowance", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Swap is not counted when the setting is off", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Swap headroom counts as memory while the host is within its allowance", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Without swap counting the same host is critical on memory", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Swap use beyond the allowance is reported without changing the state", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Critical memory pressure stays critical within the allowance", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "The memory PSI warning band counts only beyond the swap allowance", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Swap-out growth is left to memory PSI while counting is on", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "A RAM-proportional warning fires below the percent threshold", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "A RAM-proportional warning honors its floor", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "A RAM-proportional critical fires above the fixed critical floor", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "Swap-out pressure within the swap multiple is waived", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "The swap multiple never waives critical memory pressure", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "The swap multiple keeps the over-allowance advisory", Boundary: hostEvidenceBoundary, Reason: reasonHostEvidence},
		{Scenario: "An exclusive session advertises exclusive coordination", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Releasing the final exclusive session removes the coordination marker", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A live heavy lease defers a second owner", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "A long-lived service never holds the heavy-work lease", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "Concurrent services keep their own inheritable sessions", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "An inherited session runs without reacquiring the lease", Boundary: "session inheritance", Reason: "requires synthetic inherited session state unavailable to an isolated binary fixture"},
		{Scenario: "A failed child keeps its own exit code", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Canonical concurrency remains ecosystem neutral", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Consumers select their concurrency environment mappings", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Invalid concurrency mappings fail before child execution", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Guarded child streams remain composable", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "An interrupted guard signals once and then force-stops the child", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "A service port held by a live owner defers the contender", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "A supervision failure reaps the guarded child before releasing ownership", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Critical pressure sheds eligible work", Boundary: processControlBoundary, Reason: "requires synthetic critical pressure while controlling the child process lifecycle"},
		{Scenario: "Warning that outlasts the class grace sheds eligible work", Boundary: processControlBoundary, Reason: "requires synthetic lasting warning and a shortened warning grace while controlling the child process lifecycle"},
		{Scenario: "A single registered waiter rides out exhausted capacity", Boundary: reservationCapacityBoundary, Reason: "requires capacity to be freed from inside the queue pause, which needs an injected clock the compiled binary does not expose"},
		{Scenario: "Waiting for admission still surrenders when the budget is spent", Boundary: reservationCapacityBoundary, Reason: "requires an injected clock to spend the wait budget without spending the same wall time"},
		{Scenario: "Waiting for admission never retries a decision that is not a deferral", Boundary: reservationCapacityBoundary, Reason: "shares the injected clock of the deferral scenarios it is contrasted against"},
		{Scenario: "A caller waiting for admission reports one queue and deferral receipt", Boundary: reservationCapacityBoundary, Reason: "counting queue heartbeats across many polls needs an injected clock to spend the deadline without spending the same wall time"},
		{Scenario: "Admission waiting leaves every later safety notice audible", Boundary: reservationCapacityBoundary, Reason: "degrades storage after queue admission, which needs the injected sample sequence the compiled binary does not expose"},
		{Scenario: "A consumer that waits out a capacity deferral passes its probe", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "A consumer that reads a capacity deferral as final fails its probe", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "A probe that never reaches admission cannot pass by returning early", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Status names a payload group only while its payload runs without its guard", Boundary: processControlBoundary, Reason: abandonedOwnerReason},
		{Scenario: "Worsening warning sheds degraded work", Boundary: processControlBoundary, Reason: "requires synthetic warning growth while controlling the child process lifecycle"},
		{Scenario: "Stable warning spares an ephemeral child of the balanced lineage", Boundary: processControlBoundary, Reason: "requires a synthetic stable warning while controlling the child process lifecycle"},
		{Scenario: "Unsafe pressure still sheds a balanced ephemeral child admitted under normal pressure", Boundary: processControlBoundary, Reason: "requires synthetic warning growth, critical pressure, and disk evidence while controlling the child process lifecycle"},
		{Scenario: "Stable warning still sheds work outside the exemption", Boundary: processControlBoundary, Reason: "requires a synthetic stable warning while controlling the child process lifecycle"},
		{Scenario: "A shed run records only a lower bound of its child usage", Boundary: processControlBoundary, Reason: "requires synthetic critical pressure and an unreaped grandchild that the compiled binary cannot inject"},
		{Scenario: "Active evidence keeps only the newest raw chunks", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "A lifetime summary counts every recorded sample after the raw chunks rotate", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "A shared root admits at most twenty live evidence streams", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "Inactive evidence is pruned to the shared storage budget", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "Release admission preserves the requested capacity envelope", Boundary: hostEvidenceBoundary, Reason: "requires synthetic release capacity that cannot be injected through the compiled binary"},
		{Scenario: "A failed release stability check names its own reason in one line", Boundary: hostEvidenceBoundary, Reason: "requires synthetic release pressure, disk, and collector failures that cannot be injected through the compiled binary"},
		{Scenario: "Development monitor emits machine-readable transitions", Boundary: hostEvidenceBoundary, Reason: "requires deterministic repeated and changing host states that cannot be injected through the compiled binary"},
		{Scenario: "Release raw evidence streams to standard output", Boundary: hostEvidenceBoundary, Reason: "requires injected health probes and deterministic cancellation unavailable to the compiled binary fixture"},
		{Scenario: "Release summary streams to standard output", Boundary: hostEvidenceBoundary, Reason: "requires injected health probes and deterministic cancellation unavailable to the compiled binary fixture"},
		{Scenario: "Release monitoring refuses missing health inputs before its deadline", Boundary: hostEvidenceBoundary, Reason: "requires a monitoring deadline that has already passed when the command starts, which cannot be injected through the compiled binary"},
		{Scenario: "Release streaming propagates downstream failure", Boundary: "output stream", Reason: "requires an injected failing writer unavailable through the compiled binary process boundary"},
		{Scenario: "HIPPO_HEALTH_URL and HIPPO_ROUTED_ORIGIN supply the release monitor flags they default", Boundary: "release monitor endpoints", Reason: "a compiled monitor reports no endpoint it was given, and a live health service that would show them cannot be fixed by the scenario"},
		{Scenario: "Bootstrap cache retention is bounded", Boundary: "bootstrap wrapper", Reason: "cache retention is owned by the wrapper outside the compiled binary boundary"},
		{Scenario: "Release versions use exact semantic syntax", Boundary: repositoryStateBoundary, Reason: reasonRepositoryHistory},
		{Scenario: "Release commits are full lowercase real commits", Boundary: repositoryStateBoundary, Reason: "requires isolated valid and invalid Git object identities"},
		{Scenario: "Release commit equals checkout HEAD", Boundary: repositoryStateBoundary, Reason: "requires isolated history with a real non-HEAD commit"},
		{Scenario: "A release checkout with tracked or untracked changes is rejected", Boundary: repositoryStateBoundary, Reason: "a compiled binary run cannot be given an isolated checkout with tracked or untracked dirt"},
		{Scenario: "Every invalid release identity creates no output", Boundary: repositoryStateBoundary, Reason: reasonRepositoryHistory},
		{Scenario: "Release builds use only exact committed source", Boundary: repositoryStateBoundary, Reason: reasonRepositoryHistory},
		{Scenario: "Release builds use the Go release go.mod names", Boundary: repositoryStateBoundary, Reason: "requires a recording Go toolchain and host Go settings around fixture release builds"},
		{Scenario: "Rebuilding a release commit reproduces its archives", Boundary: repositoryStateBoundary, Reason: "requires two fixture release builds from binaries stamped at controlled times"},
		{Scenario: "Release assets have one exact platform set", Boundary: releaseArtifactsBoundary, Reason: "requires a complete cross-platform archive build and checksum inventory"},
		{Scenario: "Every release archive has one executable member", Boundary: releaseArtifactsBoundary, Reason: reasonReleaseArchives},
		{Scenario: "Release archive members cannot redirect through unsafe types", Boundary: releaseArtifactsBoundary, Reason: reasonReleaseArchives},
		{Scenario: "Release binaries carry one clean source identity", Boundary: releaseArtifactsBoundary, Reason: reasonReleaseArchives},
		{Scenario: "An empty raw conformance manifest input is rejected privately before it is used", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Cancellation waits for a leader-first descendant group before reconciliation", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Completed commands retire background descendants before reconciliation", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "A consumer cannot replace its provided verified HIPPO binary between commands", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Concurrent consumers cannot replace a previously verified HIPPO binary", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "Capacity skip never hides verified-binary integrity failures", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "A protocol mismatch cannot pass as a capacity skip", Boundary: consumerHarnessBoundary, Reason: "requires an allow-capacity-skip manifest with a synthetic protocol-mismatch command"},
		{Scenario: "A never-started capacity deferral is skipped", Boundary: consumerHarnessBoundary, Reason: "requires an allow-capacity-skip manifest with a synthetic capacity-deferral command and receipt"},
		{Scenario: "A pressure shed cannot pass as a capacity skip", Boundary: consumerHarnessBoundary, Reason: "requires an allow-capacity-skip manifest with a synthetic pressure-shed command"},
		{Scenario: "Conformance setup filesystem errors remain private", Boundary: consumerHarnessBoundary, Reason: reasonConsumerHarness},
		{Scenario: "HIPPO protocol names cannot be consumer concurrency mappings", Boundary: processControlBoundary, Reason: "requires deterministic reservation mapping validation before isolated child execution"},
		{Scenario: "A cancelled queued run keeps its queue place when cleanup cannot take the lock", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "JSON status waits out a busy coordination root", Boundary: leaseOwnershipBoundary, Reason: "requires a peer holding the shared coordination lock for a bounded window, which no compiled binary invocation can arrange"},
		{Scenario: "Bounded shared-root contention preserves started work and healthy observation", Boundary: leaseOwnershipBoundary, Reason: "requires a peer holding the shared coordination lock across a guard's activation and sampling, which no compiled binary invocation can arrange"},
		{Scenario: "Stalled activation contention fails safely after launch", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Cancellation retains ownership when post-kill retirement is unconfirmed", Boundary: processControlBoundary, Reason: reasonCoordinationState},
		{Scenario: "Pressure shedding preserves its stable exit while retirement is unconfirmed", Boundary: processControlBoundary, Reason: "requires injected storage and capacity shedding with unconfirmed child retirement"},
		{Scenario: "Port ownership survives supervisor-only death", Boundary: processControlBoundary, Reason: reasonCoordinationState},
		{Scenario: "Tokenized port identity corruption remains fail closed", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Legacy tokenless port markers use conservative PID compatibility", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A successful leader cannot retire a live background descendant", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "An inherited session cannot bypass background descendant retirement", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "A stalled lifetime activation handshake is bounded", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Payloads cannot inherit lifetime identity descriptors", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Failed activation reporting retains launcher-owned identity", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Payloads cannot forge the launcher activation report", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Reservation identity path corruption remains fail closed", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A dead owner behind a changed device number reconciles", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A dead waiter behind a changed device number reconciles", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "A live owner behind a changed device number stays active", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Incomplete identity evidence after a device change stays closed", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Inheritance follows the lock across a device change", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Hidden lifetime launch mode requires a parent capability", Boundary: processControlBoundary, Reason: reasonProcessControl},
		{Scenario: "Schema-one ownership survives supervisor-only death", Boundary: processControlBoundary, Reason: "requires killing schema-one heavy and service supervisors while child groups remain live"},
		{Scenario: "Legacy schema-one PID-only ownership remains conservative", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Abandoned atomic coordination temporary files expire", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "Release cleanliness inspection fails closed", Boundary: repositoryStateBoundary, Reason: "requires an injected Git status failure in isolated release history"},
		{Scenario: "Release temporary materialization is always removed", Boundary: repositoryStateBoundary, Reason: "requires controlled release staging success and failure paths"},
		{Scenario: "Release archives have normalized ownership metadata", Boundary: releaseArtifactsBoundary, Reason: reasonReleaseArchives},
		{Scenario: "Native release identity works from a path containing spaces", Boundary: releaseArtifactsBoundary, Reason: "requires executing a native archive member from a controlled spaced path"},
		// This reviewed boundary retains the original observation scope.
		{Scenario: "A display annotation adds no field to status", Boundary: "observation fixture ownership", Reason: "requires an owned command/frame fixture that changes display annotations while retaining stable observation evidence"},
		{Scenario: "A changed display annotation does not make watch emit another snapshot", Boundary: "observation fixture ownership", Reason: "requires an owned command/frame fixture that changes display annotations while retaining stable observation evidence"},
		{Scenario: "An internal fault exits 2 naming hippo.internal.failure", Boundary: hostEvidenceBoundary, Reason: "a compiled binary cannot be made to fault inside its own host sampler, so an unhandled fault cannot be injected through it"},
		{Scenario: "Only capacity deferral and pressure shed are retryable in the failure body", Boundary: hostEvidenceBoundary, Reason: "requires synthetic disk, capacity, and critical pressure evidence that the compiled binary cannot inject"},
		{Scenario: "The bootstrap reuses its compiled binary from HIPPO_BUILD_CACHE on a second run", Boundary: releaseArtifactsBoundary, Reason: "the bootstrap wrapper that compiles the binary runs outside the compiled binary boundary"},
		{Scenario: "Completed prior-day evidence is compacted into daily raw and history files within their windows", Boundary: evidenceFilesystemBoundary, Reason: reasonEvidenceFiles},
		{Scenario: "Text status prints a summary line and one privacy-safe row per owner and waiter", Boundary: leaseOwnershipBoundary, Reason: reasonCoordinationState},
		{Scenario: "Watch pauses for its interval between snapshots", Boundary: hostEvidenceBoundary, Reason: "a compiled binary prints only changed snapshots, so a stable host shows no second snapshot whose wait can be timed from outside"},
		{Scenario: "Text monitor prints the initial state and one line per transition", Boundary: hostEvidenceBoundary, Reason: "requires deterministic repeated and changing host states that cannot be injected through the compiled binary"},
	},
}

// Scenario is the compliance view of one compiled Gherkin scenario.
type Scenario struct {
	Name  string
	Steps []string
	Tags  []string
}

// Driver is the shared contract implemented by every behavior adapter.
type Driver interface {
	Bindings() []StepBinding
	Reset()
	Close()
}

// Adapters returns the complete behavior adapter inventory in execution order.
func Adapters() []Adapter {
	return []Adapter{
		{Name: Unit},
		{Name: Integration, ExemptionTag: "@integration-exempt"},
		{Name: E2E, ExemptionTag: "@e2e-exempt"},
	}
}

// AdapterByName returns one configured adapter.
func AdapterByName(name string) (Adapter, error) {
	for _, adapter := range Adapters() {
		if adapter.Name == name {
			return adapter, nil
		}
	}

	return Adapter{}, fmt.Errorf("unknown behavior adapter %q", name)
}

// SuiteOptions returns strict Godog options shared by compliance and execution.
func SuiteOptions(adapter Adapter) *godog.Options {
	return SuiteOptionsAt(adapter, FeaturePath)
}

// Run executes the canonical corpus through one strict adapter.
func Run(t *testing.T, adapter Adapter, driver Driver) {
	t.Helper()
	RunAt(t, adapter, driver, FeaturePath)
}

// ValidateScenarioStructure requires an explicit action and outcome.
func ValidateScenarioStructure(name string, keywordTypes []string) error {
	hasAction, hasOutcome := false, false

	for _, keywordType := range keywordTypes {
		hasAction = hasAction || keywordType == "Action"
		hasOutcome = hasOutcome || keywordType == "Outcome"
	}
	if !hasAction || !hasOutcome {
		return fmt.Errorf("scenario %q requires explicit When and Then steps", name)
	}

	return nil
}

// ValidateHandlers rejects missing and non-function step handlers.
func ValidateHandlers(bindings []StepBinding) []error {
	errorsFound := []error{}

	for _, binding := range bindings {
		handler := reflect.ValueOf(binding.Handler)
		if !handler.IsValid() || handler.Kind() != reflect.Func || handler.IsNil() {
			errorsFound = append(errorsFound, fmt.Errorf("binding %q requires a nonnil function handler", binding.Pattern))
		}
	}

	return errorsFound
}

// ValidateBindings rejects invalid expressions, invalid handlers, undefined or ambiguous steps, and unused bindings.
func ValidateBindings(bindings []StepBinding, steps []string) []error {
	compiled := make([]*regexp.Regexp, len(bindings))
	errorsFound := ValidateHandlers(bindings)

	for index, binding := range bindings {
		expression, err := regexp.Compile(binding.Pattern)
		if err != nil {
			errorsFound = append(errorsFound, fmt.Errorf("invalid binding %q: %w", binding.Pattern, err))
			continue
		}
		compiled[index] = expression
	}

	used := make([]bool, len(bindings))

	for _, step := range steps {
		matches := []int{}

		for index, expression := range compiled {
			if expression != nil && expression.MatchString(step) {
				matches = append(matches, index)
			}
		}

		switch len(matches) {
		case 0:
			errorsFound = append(errorsFound, fmt.Errorf("undefined behavior step %q", step))
		case 1:
			used[matches[0]] = true
		default:
			errorsFound = append(errorsFound, fmt.Errorf("ambiguous behavior step %q", step))
		}
	}

	for index, wasUsed := range used {
		if !wasUsed && compiled[index] != nil {
			errorsFound = append(errorsFound, fmt.Errorf("unused behavior binding %q", bindings[index].Pattern))
		}
	}

	return errorsFound
}

// ValidateExemptions requires exact reviewed tags with a concrete boundary and
// reason. Adapters without an exemption tag must execute the complete corpus.
func ValidateExemptions(adapter Adapter, scenarios []Scenario) []error {
	errorsFound := []error{}
	approved := map[string]Exemption{}
	exemptions := ApprovedExemptions[adapter.Name]

	if adapter.ExemptionTag == "" && len(exemptions) > 0 {
		errorsFound = append(errorsFound, fmt.Errorf("%s adapter does not permit exemptions", adapter.Name))
	}

	for _, exemption := range exemptions {
		if _, duplicate := approved[exemption.Scenario]; duplicate {
			errorsFound = append(errorsFound, fmt.Errorf("%s exemption %q is duplicated", adapter.Name, exemption.Scenario))
		}
		if strings.TrimSpace(exemption.Boundary) == "" {
			errorsFound = append(errorsFound, fmt.Errorf("%s exemption %q has no boundary", adapter.Name, exemption.Scenario))
		}
		if strings.TrimSpace(exemption.Reason) == "" {
			errorsFound = append(errorsFound, fmt.Errorf("%s exemption %q has no reason", adapter.Name, exemption.Scenario))
		}

		approved[exemption.Scenario] = exemption
	}

	seen := map[string]bool{}
	knownTags := map[string]bool{}

	for _, configuredAdapter := range Adapters() {
		if configuredAdapter.ExemptionTag != "" {
			knownTags[configuredAdapter.ExemptionTag] = true
		}
	}

	for _, scenario := range scenarios {
		tagged := false

		for _, tag := range scenario.Tags {
			if strings.HasSuffix(tag, "-exempt") && !knownTags[tag] {
				errorsFound = append(errorsFound, fmt.Errorf("scenario %q uses unknown exemption tag %q", scenario.Name, tag))
			}
			tagged = tagged || tag == adapter.ExemptionTag
		}

		_, expected := approved[scenario.Name]
		if tagged != expected {
			errorsFound = append(errorsFound, fmt.Errorf("%s exemption mismatch for scenario %q", adapter.Name, scenario.Name))
		}
		if tagged {
			seen[scenario.Name] = true
		}
	}

	for scenario := range approved {
		if !seen[scenario] {
			errorsFound = append(errorsFound, fmt.Errorf("approved %s exemption %q is absent", adapter.Name, scenario))
		}
	}

	return errorsFound
}

type corpusInspection struct {
	scenarios       []Scenario
	steps           []string
	structuralError []error
	astScenarios    int
}

func countDiskFeaturesAt(featurePath string) (int, error) {
	count := 0
	err := filepath.WalkDir(featurePath, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if !entry.IsDir() && filepath.Ext(path) == ".feature" {
			count++
		}
		return nil
	})

	return count, err
}

func inspectScenario(scenario *messages.Scenario) error {
	keywords := make([]string, 0, len(scenario.Steps))
	for _, step := range scenario.Steps {
		keywords = append(keywords, step.KeywordType.String())
	}

	return ValidateScenarioStructure(scenario.Name, keywords)
}

func inspectFeature(uri string, feature *messages.Feature, pickles []*messages.Pickle) corpusInspection {
	inspection := corpusInspection{}
	if feature == nil {
		inspection.structuralError = append(inspection.structuralError, fmt.Errorf("%s has no Feature declaration", uri))
		return inspection
	}

	for _, child := range feature.Children {
		if scenario := child.Scenario; scenario != nil {
			inspection.astScenarios++
			if err := inspectScenario(scenario); err != nil {
				inspection.structuralError = append(inspection.structuralError, err)
			}
		}
		if rule := child.Rule; rule != nil {
			inspection.inspectRule(rule)
		}
	}

	inspection.inspectPickles(pickles)

	return inspection
}

func (inspection *corpusInspection) inspectRule(rule *messages.Rule) {
	for _, child := range rule.Children {
		if scenario := child.Scenario; scenario != nil {
			inspection.astScenarios++
			if err := inspectScenario(scenario); err != nil {
				inspection.structuralError = append(inspection.structuralError, err)
			}
		}
	}
}

func (inspection *corpusInspection) inspectPickles(pickles []*messages.Pickle) {
	for _, pickle := range pickles {
		scenario := Scenario{Name: pickle.Name}

		for _, step := range pickle.Steps {
			scenario.Steps = append(scenario.Steps, step.Text)
			inspection.steps = append(inspection.steps, step.Text)
		}
		for _, tag := range pickle.Tags {
			scenario.Tags = append(scenario.Tags, tag.Name)
		}

		inspection.scenarios = append(inspection.scenarios, scenario)
	}
}

func mergeInspection(target *corpusInspection, source corpusInspection) {
	target.scenarios = append(target.scenarios, source.scenarios...)
	target.steps = append(target.steps, source.steps...)
	target.structuralError = append(target.structuralError, source.structuralError...)
	target.astScenarios += source.astScenarios
}

func runnableScenarios(adapter Adapter, scenarios []Scenario) int {
	count := 0

	for _, scenario := range scenarios {
		exempt := false

		for _, tag := range scenario.Tags {
			exempt = exempt || tag == adapter.ExemptionTag
		}

		if !exempt {
			count++
		}
	}

	return count
}

func combineErrors(errorsFound []error) error {
	if len(errorsFound) == 0 {
		return nil
	}

	messagesFound := make([]string, 0, len(errorsFound))
	for _, found := range errorsFound {
		messagesFound = append(messagesFound, found.Error())
	}

	sort.Strings(messagesFound)

	return errors.New(strings.Join(messagesFound, "\n"))
}

// Verify validates the recursive corpus, bindings, structure, and one adapter policy.
func Verify(adapter Adapter, bindings []StepBinding) error {
	return VerifyAt(adapter, bindings, FeaturePath)
}
