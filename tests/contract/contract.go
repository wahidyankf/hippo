package contract

import (
	"context"
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
	privateLedgerReason         = "requires injected private ledger corruption and byte-level inspection"
	repositoryStateBoundary     = "repository state"
	releaseArtifactsBoundary    = "release artifacts"
	releaseWorkflowBoundary     = "release workflow"
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
		{Scenario: "Balanced work falls back on a small runner", Boundary: hostEvidenceBoundary, Reason: "requires synthetic host capacity that cannot be injected through the compiled binary"},
		{Scenario: "Minimal work still runs on a tiny machine", Boundary: hostEvidenceBoundary, Reason: "requires synthetic host capacity that cannot be injected through the compiled binary"},
		{Scenario: "Exhausted storage requires cleanup", Boundary: hostEvidenceBoundary, Reason: "requires synthetic disk evidence that cannot be injected through the compiled binary"},
		{Scenario: "Swap-out growth is normalized to the policy window", Boundary: hostEvidenceBoundary, Reason: "requires synthetic swap counters that cannot be injected through the compiled binary"},
		{Scenario: "Compressor growth requires both payload and growth", Boundary: hostEvidenceBoundary, Reason: "requires synthetic compressor counters that cannot be injected through the compiled binary"},
		{Scenario: "A strict transaction does not silently downgrade", Boundary: "task capacity", Reason: "requires a synthetic capacity mismatch that cannot be injected through the compiled binary"},
		{Scenario: "Automatic reservations divide capacity by profile owner shares", Boundary: reservationCapacityBoundary, Reason: "requires deterministic synthetic CPU and memory capacity unavailable through the compiled binary"},
		{Scenario: "Maximum-width automatic shares do not overflow ceiling division", Boundary: reservationCapacityBoundary, Reason: "requires maximum-width synthetic CPU and memory capacity unavailable through the compiled binary"},
		{Scenario: "Every development class consumes reservation capacity", Boundary: leaseOwnershipBoundary, Reason: "requires concurrent private reservation ownership and deterministic capacity"},
		{Scenario: "Explicit reservations may be smaller than automatic shares", Boundary: reservationCapacityBoundary, Reason: "requires deterministic synthetic capacity and direct allocation inspection"},
		{Scenario: "Reservation floors reject unsafe requests", Boundary: reservationCapacityBoundary, Reason: "requires direct pre-enqueue allocation inspection"},
		{Scenario: "Vector admission is atomic", Boundary: leaseOwnershipBoundary, Reason: "requires deterministic asymmetric capacity exhaustion and private ledger inspection"},
		{Scenario: "Impossible reservation requires replanning", Boundary: reservationCapacityBoundary, Reason: "requires deterministic synthetic total capacity"},
		{Scenario: "Temporary exhaustion uses the bounded wait", Boundary: leaseOwnershipBoundary, Reason: "requires deterministic live owner timing and private waiter inspection"},
		{Scenario: "FIFO head cannot be bypassed by a smaller request", Boundary: leaseOwnershipBoundary, Reason: "requires deterministic concurrent waiter ordering and private queue inspection"},
		{Scenario: "Exhausted FIFO sequence resets only after a stale epoch empties", Boundary: leaseOwnershipBoundary, Reason: "requires injected maximum sequence state and private liveness identities"},
		{Scenario: "Inherited sessions reuse their fixed allocation", Boundary: "session inheritance", Reason: "requires direct private allocation and nested ownership inspection"},
		{Scenario: "Fixed allocations clamp concurrency mappings", Boundary: processControlBoundary, Reason: "requires deterministic allocation injection and isolated child environment inspection"},
		{Scenario: "Stale identity cannot retain or inherit capacity", Boundary: leaseOwnershipBoundary, Reason: "requires synthetic process identities and private owner records"},
		{Scenario: "Active exclusive compatibility blocks reservation takeover", Boundary: leaseOwnershipBoundary, Reason: "requires injected live compatibility ownership and private marker inspection"},
		{Scenario: "Malformed compatibility state remains fail closed during takeover", Boundary: leaseOwnershipBoundary, Reason: "requires injected malformed private compatibility state"},
		{Scenario: "Unsupported compatibility heavy-owner schema remains actionable", Boundary: leaseOwnershipBoundary, Reason: "requires injected private compatibility state plus direct diagnostic and byte-level inspection"},
		{Scenario: "Malformed compatibility service session remains fail closed during takeover", Boundary: leaseOwnershipBoundary, Reason: "requires injected unverifiable private compatibility session state"},
		{Scenario: "Unreadable compatibility session inventory blocks reservation takeover", Boundary: leaseOwnershipBoundary, Reason: "requires structurally unreadable private legacy inventory and byte-preservation inspection"},
		{Scenario: "Failed stale heavy cleanup blocks reservation takeover", Boundary: leaseOwnershipBoundary, Reason: "requires a permission-denied private legacy cleanup boundary and marker inspection"},
		{Scenario: "Host pressure thresholds remain authoritative", Boundary: hostEvidenceBoundary, Reason: syntheticHostPressureReason},
		{Scenario: "Shedding selects one newest revocable owner", Boundary: processControlBoundary, Reason: "requires multiple controlled owners and synthetic critical pressure"},
		{Scenario: "Service shedding follows exhausted ephemeral owners", Boundary: processControlBoundary, Reason: "requires multiple controlled service owners and synthetic critical pressure"},
		{Scenario: "Remote shedding completes one victim before another election", Boundary: processControlBoundary, Reason: "requires multiple controlled process groups and bounded signal escalation"},
		{Scenario: "Remote observation never signals a replaced identity", Boundary: processControlBoundary, Reason: "requires controlled process-group identity replacement during remote observation"},
		{Scenario: "Remote selectors never signal another owner's child", Boundary: processControlBoundary, Reason: "requires a controlled unresponsive remote owner and process-group signal observation"},
		{Scenario: "Remote observation remains bounded by its deadline", Boundary: processControlBoundary, Reason: "requires holding the private coordination advisory lock across a deterministic deadline"},
		{Scenario: "Owner cancellation remains bounded when release coordination is held", Boundary: processControlBoundary, Reason: "requires cancelling a controlled child while holding its private release lock"},
		{Scenario: "Cancelled FIFO waiters use a fresh cleanup deadline", Boundary: leaseOwnershipBoundary, Reason: "requires cancelling a queued private waiter while holding and releasing its coordination lock"},
		{Scenario: "Supervisor death cannot release a live child reservation", Boundary: processControlBoundary, Reason: "requires killing only a compiled guard while retaining and inspecting its isolated live child group"},
		{Scenario: "Status rejects overflowing aggregate waiter demand", Boundary: leaseOwnershipBoundary, Reason: "requires constructing private individually valid waiter accounting at integer boundaries"},
		{Scenario: "Development summaries retain owners between host samples", Boundary: leaseOwnershipBoundary, Reason: "requires deterministic private admission and release events shorter than one host sample"},
		{Scenario: "Custom profiles inherit reservation shares and fixed concurrency", Boundary: reservationCapacityBoundary, Reason: "requires deterministic synthetic CPU capacity plus private child environment and summary inspection"},
		{Scenario: "Owner-side shedding preserves the selected exit", Boundary: processControlBoundary, Reason: "requires injected storage-shedding selection during controlled owner supervision"},
		{Scenario: "Transactional owners are never shed after admission", Boundary: processControlBoundary, Reason: "requires controlled transactional owners and synthetic critical pressure"},
		{Scenario: "Shared owner limits remain conservative across configurations", Boundary: leaseOwnershipBoundary, Reason: "requires mixed private owner and waiter configurations in one isolated shared root"},
		{Scenario: "Active reservation capacity cannot shrink below live commitments", Boundary: leaseOwnershipBoundary, Reason: "requires mixed vector caps with a live owner and FIFO waiter in one isolated shared root"},
		{Scenario: "Reservation ledger tokens are validated before mutation", Boundary: leaseOwnershipBoundary, Reason: privateLedgerReason},
		{Scenario: "Reservation ledger classes are validated before mutation", Boundary: leaseOwnershipBoundary, Reason: privateLedgerReason},
		{Scenario: "Reservation ledger sequences are validated before mutation", Boundary: leaseOwnershipBoundary, Reason: privateLedgerReason},
		{Scenario: "Reservation ledger vectors are validated before mutation", Boundary: leaseOwnershipBoundary, Reason: privateLedgerReason},
		{Scenario: "Reservation ledger arithmetic rejects overflow before mutation", Boundary: leaseOwnershipBoundary, Reason: "requires maximum-width private vectors and byte-level ledger inspection"},
		{Scenario: "Maximum-width admission cannot wrap vector capacity", Boundary: reservationCapacityBoundary, Reason: "requires maximum-width synthetic capacity and private allocation inspection"},
		{Scenario: "Reservation ledger totals are validated before mutation", Boundary: leaseOwnershipBoundary, Reason: privateLedgerReason},
		{Scenario: "Reservation ledger structure is validated before mutation", Boundary: leaseOwnershipBoundary, Reason: privateLedgerReason},
		{Scenario: "Unknown identity errors never prune reservation accounting", Boundary: leaseOwnershipBoundary, Reason: "requires an injected identity-filesystem error and private byte-level inspection"},
		{Scenario: "Missing live reservation accounting cannot initialize empty", Boundary: leaseOwnershipBoundary, Reason: "requires removing private accounting while retaining a live kernel identity lock"},
		{Scenario: "Runtime inputs remain canonical across consumer commands", Boundary: consumerHarnessBoundary, Reason: "requires temporary checkout symlinks and inherited process environment control"},
		{Scenario: "Consumer coordination never inherits the conformance caller session", Boundary: consumerHarnessBoundary, Reason: "requires one private live caller reservation plus independent overlapping consumer ownership"},
		{Scenario: "A validated checkout directory cannot be replaced between phases", Boundary: consumerHarnessBoundary, Reason: "requires replacing a private checkout directory with an identical Git state between harness phases"},
		{Scenario: "The created shared root cannot be replaced between phases", Boundary: consumerHarnessBoundary, Reason: "requires replacing a private runtime directory with a distinct directory or symlink between harness phases"},
		{Scenario: "Force-stop reap failure remains bounded", Boundary: consumerHarnessBoundary, Reason: "requires injected post-KILL wait failure while retaining private checkout reconciliation"},
		{Scenario: "Verified binary cleanup failures are reported", Boundary: consumerHarnessBoundary, Reason: "requires making private verified-binary storage temporarily unremovable"},
		{Scenario: "Cancellation before a sequential command prevents its start", Boundary: consumerHarnessBoundary, Reason: "requires deterministic cancellation exactly between private sequential command starts"},
		{Scenario: "A verified HIPPO binary cannot change between command phases", Boundary: consumerHarnessBoundary, Reason: "requires replacing a temporary executable between private harness phases"},
		{Scenario: "Commands execute one pinned verified HIPPO identity", Boundary: consumerHarnessBoundary, Reason: "requires replacing a temporary executable after verification while a consumer command invokes the provided identity"},
		{Scenario: "A HIPPO binary FIFO is rejected boundedly", Boundary: consumerHarnessBoundary, Reason: "requires a private named pipe and direct validation-deadline observation"},
		{Scenario: "A HIPPO binary directory is rejected", Boundary: consumerHarnessBoundary, Reason: "requires a private directory in the executable identity position"},
		{Scenario: "A HIPPO binary must be executable", Boundary: consumerHarnessBoundary, Reason: "requires private executable-mode inspection outside the compiled public surface"},
		{Scenario: "Missing command errors remain private", Boundary: consumerHarnessBoundary, Reason: "requires an absolute missing executable and direct error-surface inspection"},
		{Scenario: "Invalid checkout command errors remain private", Boundary: consumerHarnessBoundary, Reason: "requires removing a private checkout between command phases"},
		{Scenario: "Independent bootstrap lanes run concurrently behind one phase barrier", Boundary: consumerHarnessBoundary, Reason: "requires deterministic timing and multiple failures across four temporary checkout processes"},
		{Scenario: "Development summaries retain the lifetime peak owner count", Boundary: leaseOwnershipBoundary, Reason: "requires deterministic concurrent private ownership changes during child supervision"},
		{Scenario: "Reservation protocol files survive evidence retention", Boundary: evidenceFilesystemBoundary, Reason: "requires aged private protocol files and direct retention inspection"},
		{Scenario: "Atomic coordination writes survive evidence retention", Boundary: evidenceFilesystemBoundary, Reason: "requires synthetic aged live atomic-write temp files and direct retention inspection"},
		{Scenario: "Concurrent evidence disappearance does not abort guarded work", Boundary: evidenceFilesystemBoundary, Reason: "requires concurrent private evidence finalization and retention snapshot churn"},
		{Scenario: "Four manifest consumers preserve their checkout state", Boundary: consumerHarnessBoundary, Reason: "repository-owned manifest orchestration is outside the compiled HIPPO binary"},
		{Scenario: "Checkout aliases cannot name one consumer twice", Boundary: consumerHarnessBoundary, Reason: "requires temporary checkout symlinks and private manifest inspection"},
		{Scenario: "Every checkout is reconciled after any command failure", Boundary: consumerHarnessBoundary, Reason: "requires intentionally mutating temporary checkouts across failing manifest phases"},
		{Scenario: "Linux cgroup memory limits host capacity", Boundary: hostEvidenceBoundary, Reason: "requires synthetic proc and cgroup files instead of the public host filesystem"},
		{Scenario: "Linux without swap remains usable", Boundary: hostEvidenceBoundary, Reason: "requires synthetic swap capabilities instead of the public host state"},
		{Scenario: "Linux PSI detects active memory contention", Boundary: hostEvidenceBoundary, Reason: "requires synthetic PSI evidence instead of the public host state"},
		{Scenario: "An exclusive session advertises compatibility coordination", Boundary: leaseOwnershipBoundary, Reason: "requires deterministic inspection of private coordination ownership unavailable through the compiled binary"},
		{Scenario: "A live heavy lease defers a second owner", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "A long-lived service never holds the heavy-work lease", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "Concurrent services keep their own inheritable sessions", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "An inherited session runs without reacquiring the lease", Boundary: "session inheritance", Reason: "requires synthetic inherited session state unavailable to an isolated binary fixture"},
		{Scenario: "A failed child keeps its own exit code", Boundary: processControlBoundary, Reason: "requires deterministic admission and child process control unavailable to the public-host fixture"},
		{Scenario: "Canonical concurrency remains ecosystem neutral", Boundary: processControlBoundary, Reason: "requires deterministic admission and inspection of an isolated child environment"},
		{Scenario: "Consumers select their concurrency environment mappings", Boundary: processControlBoundary, Reason: "requires deterministic admission and inspection of an isolated child environment"},
		{Scenario: "Invalid concurrency mappings fail before child execution", Boundary: processControlBoundary, Reason: "requires deterministic admission setup and inspection proving that an isolated child never starts"},
		{Scenario: "Guarded child streams remain composable", Boundary: processControlBoundary, Reason: "requires deterministic admission and isolated standard streams unavailable to the public-host fixture"},
		{Scenario: "An interrupted guard signals once and then force-stops the child", Boundary: processControlBoundary, Reason: "requires deterministic interrupt timing and process signaling unavailable to the public-host fixture"},
		{Scenario: "A service port held by a live owner defers the contender", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "Stopping an exited but unreaped child group is not a supervision failure", Boundary: processControlBoundary, Reason: "requires an unreaped child process group held open by the fixture itself, which no compiled binary invocation can arrange"},
		{Scenario: "An aggressive termination grace still confirms a forced stop", Boundary: processControlBoundary, Reason: "requires an impatient termination policy and deterministic interrupt timing unavailable to the public-host fixture"},
		{Scenario: "A supervision failure reaps the guarded child before releasing ownership", Boundary: processControlBoundary, Reason: "requires an injected collector failure while controlling and inspecting the child process lifecycle"},
		{Scenario: "Critical pressure sheds eligible work", Boundary: processControlBoundary, Reason: "requires synthetic critical pressure while controlling the child process lifecycle"},
		{Scenario: "Worsening warning sheds degraded work", Boundary: processControlBoundary, Reason: "requires synthetic warning growth while controlling the child process lifecycle"},
		{Scenario: "Active evidence rotates without truncating its lifetime summary", Boundary: evidenceFilesystemBoundary, Reason: "requires test-sized rotation limits and direct inspection of private runtime files"},
		{Scenario: "A shared root admits at most twenty live evidence streams", Boundary: evidenceFilesystemBoundary, Reason: "requires concurrent private writer ownership and direct inspection of the shared runtime root"},
		{Scenario: "Inactive evidence is pruned to the shared storage budget", Boundary: evidenceFilesystemBoundary, Reason: "requires synthetic sparse evidence files and direct inspection of private retention state"},
		{Scenario: "Release admission preserves the requested capacity envelope", Boundary: hostEvidenceBoundary, Reason: "requires synthetic release capacity that cannot be injected through the compiled binary"},
		{Scenario: "Development monitor emits machine-readable transitions", Boundary: hostEvidenceBoundary, Reason: "requires deterministic repeated and changing host states that cannot be injected through the compiled binary"},
		{Scenario: "Release raw evidence streams to standard output", Boundary: hostEvidenceBoundary, Reason: "requires injected health probes and deterministic cancellation unavailable to the compiled binary fixture"},
		{Scenario: "Release summary streams to standard output", Boundary: hostEvidenceBoundary, Reason: "requires injected health probes and deterministic cancellation unavailable to the compiled binary fixture"},
		{Scenario: "Release streaming propagates downstream failure", Boundary: "output stream", Reason: "requires an injected failing writer unavailable through the compiled binary process boundary"},
		{Scenario: "Release builds stay outside repository history", Boundary: repositoryStateBoundary, Reason: "Git ignore policy is outside the compiled binary boundary"},
		{Scenario: "End-to-end binaries are temporary", Boundary: "test harness", Reason: "binary cleanup is owned by the harness outside the compiled binary boundary"},
		{Scenario: "Bootstrap cache retention is bounded", Boundary: "bootstrap wrapper", Reason: "cache retention is owned by the wrapper outside the compiled binary boundary"},
		{Scenario: "Lint gate wiring is exhaustive and module scoped", Boundary: "repository configuration", Reason: "lint configuration is outside the compiled binary boundary"},
		{Scenario: "Behavior adapter wiring is complete", Boundary: "test harness", Reason: "adapter registration is outside the compiled binary boundary"},
		{Scenario: "Contributor gate wiring is complete", Boundary: "repository configuration", Reason: "hooks and CI configuration are outside the compiled binary boundary"},
		{Scenario: "Machine-local configuration and binaries stay private", Boundary: repositoryStateBoundary, Reason: "Git index and ignore policy are outside the compiled binary boundary"},
		{Scenario: "Release versions use exact semantic syntax", Boundary: repositoryStateBoundary, Reason: "requires isolated clean Git history and pre-output release builder inspection"},
		{Scenario: "Release commits are full lowercase real commits", Boundary: repositoryStateBoundary, Reason: "requires isolated valid and invalid Git object identities"},
		{Scenario: "Release commit equals checkout HEAD", Boundary: repositoryStateBoundary, Reason: "requires isolated history with a real non-HEAD commit"},
		{Scenario: "Release checkout has no tracked changes", Boundary: repositoryStateBoundary, Reason: "requires an isolated tracked-dirty checkout"},
		{Scenario: "Release checkout has no untracked changes", Boundary: repositoryStateBoundary, Reason: "requires an isolated untracked-dirty checkout"},
		{Scenario: "Every invalid release identity creates no output", Boundary: repositoryStateBoundary, Reason: "requires direct pre-output filesystem boundary inspection"},
		{Scenario: "Release builds use only exact committed source", Boundary: repositoryStateBoundary, Reason: "requires isolated ignored and excluded build inputs plus exact-commit materialization inspection"},
		{Scenario: "Release policy inventories every version-four source", Boundary: repositoryStateBoundary, Reason: "repository source inventory is outside the compiled binary boundary"},
		{Scenario: "Release assets have one exact platform set", Boundary: releaseArtifactsBoundary, Reason: "requires a complete cross-platform archive build and checksum inventory"},
		{Scenario: "Every release archive has one executable member", Boundary: releaseArtifactsBoundary, Reason: "requires direct cross-platform archive member and mode inspection"},
		{Scenario: "Release archive members cannot redirect through unsafe types", Boundary: releaseArtifactsBoundary, Reason: "requires crafted link and special-file archives plus pre-extraction metadata inspection"},
		{Scenario: "Release binaries carry one clean source identity", Boundary: releaseArtifactsBoundary, Reason: "requires cross-platform VCS metadata inspection and native execution"},
		{Scenario: "Release workflow peels annotated and lightweight tags", Boundary: releaseWorkflowBoundary, Reason: "requires isolated annotated and lightweight tag histories"},
		{Scenario: "Release workflow requires origin main ancestry", Boundary: releaseWorkflowBoundary, Reason: "requires isolated reachable and unreachable remote histories"},
		{Scenario: "Release workflow adds no Actions storage", Boundary: releaseWorkflowBoundary, Reason: "workflow actions and permissions are outside the compiled binary boundary"},
		{Scenario: "Empty HIPPO binary input cannot resolve from the manifest directory", Boundary: consumerHarnessBoundary, Reason: "requires direct raw-manifest path validation before compiled command execution"},
		{Scenario: "Empty shared-root input cannot resolve from the manifest directory", Boundary: consumerHarnessBoundary, Reason: "requires direct raw-manifest path validation before compiled command execution"},
		{Scenario: "Empty consumer path cannot resolve to its manifest checkout", Boundary: consumerHarnessBoundary, Reason: "requires a manifest located inside a private temporary checkout"},
		{Scenario: "Group retirement waits for the host to reap an already killed group", Boundary: consumerHarnessBoundary, Reason: "requires an unreaped process group and an injected liveness clock that no compiled manifest run can arrange"},
		{Scenario: "Cancellation waits for a leader-first descendant group before reconciliation", Boundary: consumerHarnessBoundary, Reason: "requires controlled signal ordering and a private late checkout mutation"},
		{Scenario: "Completed commands retire background descendants before reconciliation", Boundary: consumerHarnessBoundary, Reason: "requires controlled zero and nonzero leaders with private late-writing descendants"},
		{Scenario: "A consumer cannot replace its provided verified HIPPO binary between commands", Boundary: consumerHarnessBoundary, Reason: "requires deliberate mutation of private per-command executable storage"},
		{Scenario: "Concurrent consumers cannot replace a previously verified HIPPO binary", Boundary: consumerHarnessBoundary, Reason: "requires synchronized private gate processes and executable replacement"},
		{Scenario: "Capacity skip never hides verified-binary integrity failures", Boundary: consumerHarnessBoundary, Reason: "requires a synthetic exit 75 joined with private executable integrity and cleanup faults"},
		{Scenario: "Conformance setup filesystem errors remain private", Boundary: consumerHarnessBoundary, Reason: "requires unavailable private manifest and shared-root filesystem paths"},
		{Scenario: "HIPPO protocol names cannot be consumer concurrency mappings", Boundary: processControlBoundary, Reason: "requires deterministic reservation mapping validation before isolated child execution"},
		{Scenario: "Failed cancelled-waiter cleanup retains verifiable FIFO ownership", Boundary: leaseOwnershipBoundary, Reason: "requires holding private coordination across cancellation and a following FIFO waiter"},
		{Scenario: "JSON status waits out a busy coordination root", Boundary: leaseOwnershipBoundary, Reason: "requires a peer holding the shared coordination lock for a bounded window, which no compiled binary invocation can arrange"},
		{Scenario: "Shared-root contention defers instead of failing admitted work", Boundary: leaseOwnershipBoundary, Reason: "requires a peer holding the shared coordination lock across a guard's activation and sampling, which no compiled binary invocation can arrange"},
		{Scenario: "Cancellation retains ownership when post-kill retirement is unconfirmed", Boundary: processControlBoundary, Reason: "requires injected bounded child-retirement failure with private reservation and port ownership"},
		{Scenario: "Pressure shedding preserves its stable exit while retirement is unconfirmed", Boundary: processControlBoundary, Reason: "requires injected storage and capacity shedding with unconfirmed child retirement"},
		{Scenario: "Port ownership survives supervisor-only death", Boundary: processControlBoundary, Reason: "requires killing only a supervisor while a private child group retains a real port lease"},
		{Scenario: "Tokenized port identity corruption remains fail closed", Boundary: leaseOwnershipBoundary, Reason: "requires missing and replaced private token identity paths"},
		{Scenario: "A stale same-process port handle cannot release a replacement lease", Boundary: leaseOwnershipBoundary, Reason: "requires private same-PID token ABA handles"},
		{Scenario: "Legacy tokenless port markers use conservative PID compatibility", Boundary: leaseOwnershipBoundary, Reason: "requires direct legacy private marker characterization"},
		{Scenario: "A successful leader cannot retire a live background descendant", Boundary: processControlBoundary, Reason: "requires a controlled no-descriptor descendant outliving its leader"},
		{Scenario: "An inherited session cannot bypass background descendant retirement", Boundary: processControlBoundary, Reason: "requires direct inherited-session child-group supervision"},
		{Scenario: "A stalled lifetime activation handshake is bounded", Boundary: processControlBoundary, Reason: "requires a deliberately stalled private launcher handshake"},
		{Scenario: "Payloads cannot inherit lifetime identity descriptors", Boundary: processControlBoundary, Reason: "requires compiled descriptor inspection by ordinary and inherited payloads"},
		{Scenario: "Failed activation reporting retains launcher-owned identity", Boundary: processControlBoundary, Reason: "requires a controlled report failure with a live no-descriptor descendant"},
		{Scenario: "Payloads cannot forge the launcher activation report", Boundary: processControlBoundary, Reason: "requires a compiled payload attempting to write the private report descriptor"},
		{Scenario: "Reservation identity path corruption remains fail closed", Boundary: leaseOwnershipBoundary, Reason: "requires missing and replaced private owner and waiter identity paths"},
		{Scenario: "An embedded guard abandons only its local lifetime handles", Boundary: processControlBoundary, Reason: "requires an unconfirmed embedded Run and private competitor admission checks"},
		{Scenario: "Hidden lifetime launch mode requires a parent capability", Boundary: processControlBoundary, Reason: "requires direct invocation of the private launcher sentinel without its capability pipe"},
		{Scenario: "Schema-one ownership survives supervisor-only death", Boundary: processControlBoundary, Reason: "requires killing schema-one heavy and service supervisors while child groups remain live"},
		{Scenario: "Schema-one embedded ownership retires independently of its caller PID", Boundary: processControlBoundary, Reason: "requires an embedded schema-one Run with unconfirmed holder retirement"},
		{Scenario: "Legacy schema-one PID-only ownership remains conservative", Boundary: leaseOwnershipBoundary, Reason: "requires live and positively stale private zero-metadata compatibility records"},
		{Scenario: "Abandoned atomic coordination temporary files expire", Boundary: evidenceFilesystemBoundary, Reason: "requires synthetic aged private coordination and reservation temporary files"},
		{Scenario: "Release cleanliness inspection fails closed", Boundary: repositoryStateBoundary, Reason: "requires an injected Git status failure in isolated release history"},
		{Scenario: "Release temporary materialization is always removed", Boundary: repositoryStateBoundary, Reason: "requires controlled release staging success and failure paths"},
		{Scenario: "Release archives have normalized ownership metadata", Boundary: releaseArtifactsBoundary, Reason: "requires direct cross-platform tar header ownership inspection"},
		{Scenario: "Native release identity works from a path containing spaces", Boundary: releaseArtifactsBoundary, Reason: "requires executing a native archive member from a controlled spaced path"},
		{Scenario: "Release workflow requires the peeled tag to equal the event commit", Boundary: releaseWorkflowBoundary, Reason: "requires isolated event and peeled-tag commit identities"},
		{Scenario: "Release workflows disable implicit Go module caching", Boundary: releaseWorkflowBoundary, Reason: "workflow setup-go caching configuration is outside the compiled binary boundary"},
		{Scenario: "CI validates assets against the exact build commit", Boundary: releaseWorkflowBoundary, Reason: "CI workflow commit propagation is outside the compiled binary boundary"},
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
	tags := ""
	if adapter.ExemptionTag != "" {
		tags = "~" + adapter.ExemptionTag
	}

	return &godog.Options{
		Format: "progress",
		Paths:  []string{FeaturePath},
		Tags:   tags,
		Strict: true,
	}
}

// Run executes the canonical corpus through one strict adapter.
func Run(t *testing.T, adapter Adapter, driver Driver) {
	t.Helper()
	defer driver.Close()

	bindings := driver.Bindings()
	if err := Verify(adapter, bindings); err != nil {
		t.Fatal(err)
	}

	options := SuiteOptions(adapter)
	options.TestingT = t
	initialize := func(scenarioContext *godog.ScenarioContext) {
		scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
			driver.Reset()

			return ctx, nil
		})

		for _, binding := range bindings {
			scenarioContext.Step(binding.Pattern, binding.Handler)
		}
	}

	status := godog.TestSuite{
		Name:                "hippo-" + adapter.Name,
		ScenarioInitializer: initialize,
		Options:             options,
	}.Run()

	if status != 0 {
		t.Fatalf("%s behavior suite exited %d", adapter.Name, status)
	}
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

func countDiskFeatures() (int, error) {
	count := 0
	err := filepath.WalkDir(FeaturePath, func(path string, entry fs.DirEntry, walkError error) error {
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
	corpusOptions := &godog.Options{Paths: []string{FeaturePath}, Strict: true}
	features, err := (godog.TestSuite{Options: corpusOptions}).RetrieveFeatures()
	if err != nil {
		return fmt.Errorf("retrieve behavior corpus: %w", err)
	}

	diskFeatures, err := countDiskFeatures()
	if err != nil {
		return fmt.Errorf("walk behavior corpus: %w", err)
	}
	if diskFeatures == 0 || len(features) != diskFeatures {
		return fmt.Errorf("parsed %d of %d canonical behavior features", len(features), diskFeatures)
	}

	inspection := corpusInspection{}

	for _, feature := range features {
		mergeInspection(&inspection, inspectFeature(feature.Uri, feature.Feature, feature.Pickles))
	}

	if inspection.astScenarios == 0 || len(inspection.scenarios) == 0 {
		inspection.structuralError = append(inspection.structuralError, errors.New("behavior corpus has no scenarios"))
	}
	inspection.structuralError = append(inspection.structuralError, ValidateBindings(bindings, inspection.steps)...)
	inspection.structuralError = append(inspection.structuralError, ValidateExemptions(adapter, inspection.scenarios)...)
	if runnableScenarios(adapter, inspection.scenarios) == 0 {
		inspection.structuralError = append(inspection.structuralError, fmt.Errorf("%s adapter runs no scenarios", adapter.Name))
	}

	return combineErrors(inspection.structuralError)
}
