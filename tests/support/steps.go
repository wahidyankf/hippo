package support

import (
	"slices"

	"github.com/wahidyankf/hippo/tests/contract"
)

var _ contract.Driver = (*Driver)(nil)

func step(pattern string, handler any) contract.StepBinding {
	return contract.StepBinding{Pattern: pattern, Handler: handler}
}

// Bindings returns the complete pattern-handler registry used by validation
// and runtime registration. Slice order is organizational, not semantic.
func (driver *Driver) Bindings() []contract.StepBinding {
	return slices.Concat(
		driver.admissionBindings(),
		driver.reservationBindings(),
		driver.executionBindings(),
		driver.terminalBindings(),
		driver.evidenceBindings(),
		driver.publicCLIBindings(),
		driver.releaseBindings(),
		driver.artifactBindings(),
		driver.qualityGateBindings(),
		driver.portabilityBindings(),
		driver.conformanceBindings(),
	)
}

func (driver *Driver) reservationBindings() []contract.StepBinding { //nolint:funlen,maintidx // The declarative inventory keeps every canonical reservation phrase explicit and auditable.
	prepare := func(name string, action func(string) error) func() error {
		return func() error { return driver.prepareReservationScenarioV04(name, action) }
	}
	assert := func(name string) func() error {
		return func() error { return driver.requireReservationScenarioV04(name) }
	}
	return []contract.StepBinding{
		step(`^healthy host capacity for automatic reservation planning$`, prepare("automatic planning", func(string) error { return requireV04Planning() })),
		step(`^automatic reservations are calculated for every built-in profile$`, driver.exerciseReservationScenarioV04),
		step(`^balanced constrained and minimal use four two and one owner shares$`, assert("automatic planning")),
		step(`^maximum representable CPU and memory capacity for balanced and constrained profiles$`, prepare("maximum automatic shares", requireV04MaximumAutomaticShares)),
		step(`^automatic fair-share reservations are calculated$`, driver.exerciseReservationScenarioV04),
		step(`^both vector dimensions retain their exact four-share and two-share ceilings$`, assert("maximum automatic shares")),
		step(`^enough capacity for one owner of each development class$`, prepare("all classes", requireV04AllClasses)),
		step(`^service ephemeral and transactional sessions are admitted$`, driver.exerciseReservationScenarioV04),
		step(`^all three allocations appear in shared reservation totals$`, assert("all classes")),
		step(`^automatic allocation larger than the immutable floors$`, prepare("explicit planning", func(string) error { return requireV04ExplicitPlanning() })),
		step(`^explicit CPU and memory reservations below that allocation are requested$`, driver.exerciseReservationScenarioV04),
		step(`^the smaller explicit vector is admitted unchanged$`, assert("explicit planning")),
		step(`^explicit requests below one CPU or 256 MiB$`, prepare("reservation floors", func(string) error { return requireV04ReservationFloors() })),
		step(`^each unsafe reservation is validated$`, driver.exerciseReservationScenarioV04),
		step(`^each request requires replanning with exit 78 before enqueue$`, assert("reservation floors")),
		step(`^remaining capacity fits only one dimension of a request$`, prepare("atomic admission", requireV04AtomicAdmission)),
		step(`^that CPU and memory vector requests admission$`, driver.exerciseReservationScenarioV04),
		step(`^neither dimension is reserved and no child starts$`, assert("atomic admission")),
		step(`^a reservation larger than total safe host capacity$`, prepare("impossible reservation", requireV04ImpossibleReservation)),
		step(`^impossible capacity is requested$`, driver.exerciseReservationScenarioV04),
		step(`^admission requires replanning with exit 78 instead of waiting$`, assert("impossible reservation")),
		step(`^a live owner temporarily consumes the remaining capacity$`, prepare("temporary exhaustion", requireV04TemporaryExhaustion)),
		step(`^another fitting reservation waits through its deadline$`, driver.exerciseReservationScenarioV04),
		step(`^the waiter remains FIFO head through its deadline and is deferred with exit 75$`, assert("temporary exhaustion")),
		step(`^a large waiter is ahead of a smaller waiter$`, prepare("strict FIFO", requireV04FIFO)),
		step(`^capacity becomes sufficient only for the smaller waiter$`, driver.exerciseReservationScenarioV04),
		step(`^neither waiter bypasses the live FIFO head$`, assert("strict FIFO")),
		step(`^a live reserved session with an inheritable token$`, prepare("inheritance", requireV04Inheritance)),
		step(`^a nested guarded command uses that token$`, driver.exerciseReservationScenarioV04),
		step(`^no second reservation is created and allocation cannot expand$`, assert("inheritance")),
		step(`^a session allocated two CPU units and 512 MiB$`, prepare("fixed environment", func(string) error { return requireV04Environment() })),
		step(`^missing lower higher and malformed concurrency mappings are evaluated$`, driver.exerciseReservationScenarioV04),
		step(`^canonical values are fixed lower values survive higher values clamp and malformed values replan$`, assert("fixed environment")),
		step(`^mappings for HIPPO root reserved memory config and default config plus one arbitrary worker name$`, prepare("reserved HIPPO mappings", requireV04ReservedHIPPOEnvironmentMappings)),
		step(`^every mapping is validated against a fixed reservation allocation$`, driver.exerciseReservationScenarioV04),
		step(`^every HIPPO mapping replans before child execution and the arbitrary worker mapping remains valid$`, assert("reserved HIPPO mappings")),
		step(`^stale owner records including a reused diagnostic PID$`, prepare("stale identity", requireV04StaleIdentity)),
		step(`^reservation ownership is reconciled$`, driver.exerciseReservationScenarioV04),
		step(`^only a matching live process identity retains capacity or inheritance$`, assert("stale identity")),
		step(`^a live schema one exclusive compatibility session$`, prepare("exclusive bridge", requireV04Bridge)),
		step(`^reservation-mode admission is requested$`, driver.exerciseReservationScenarioV04),
		step(`^it defers with exit 75 without changing compatibility state$`, assert("exclusive bridge")),
		step(`^malformed compatibility heavy state without a mode marker$`, driver.malformedCompatibilityV04),
		step(`^reservation-mode admission inspects that state$`, driver.inspectMalformedCompatibilityV04),
		step(`^it defers with exit 75 and leaves the malformed state unchanged$`, driver.requireMalformedCompatibilityDeferredV04),
		step(`^compatibility heavy state with an unsupported owner schema$`, driver.unsupportedCompatibilityV04),
		step(`^exclusive admission and the heavy-lease diagnostic inspect that state$`, driver.inspectUnsupportedCompatibilityV04),
		step(`^admission defers without mutation and the diagnostic reports private recovery guidance$`, driver.requireUnsupportedCompatibilityV04),
		step(`^an unverifiable compatibility service session record without a mode marker$`, driver.malformedServiceCompatibilityV04),
		step(`^reservation-mode admission inspects the service session$`, driver.inspectMalformedServiceCompatibilityV04),
		step(`^it defers with exit 75 and leaves the service record unchanged$`, driver.requireMalformedServiceCompatibilityV04),
		step(`^compatibility session inventory cannot be enumerated$`, prepare("unreadable compatibility inventory", requireV04UnreadableSessionInventory)),
		step(`^reservation admission attempts to take over the shared root$`, driver.exerciseReservationScenarioV04),
		step(`^admission defers and preserves the session inventory with private recovery guidance$`, assert("unreadable compatibility inventory")),
		step(`^positively stale compatibility heavy state cannot be removed$`, prepare("failed stale heavy cleanup", requireV04FailedStaleHeavyCleanup)),
		step(`^admission defers without writing a reservation marker or changing heavy state$`, assert("failed stale heavy cleanup")),
		step(`^vector capacity fits under a threshold-blocked host sample$`, prepare("threshold authority", requireV04ThresholdAuthority)),
		step(`^reservation admission evaluates that sample$`, driver.exerciseReservationScenarioV04),
		step(`^threshold pressure defers work without changing the ledger$`, assert("threshold authority")),
		step(`^transactional service and ephemeral owners under critical pressure$`, prepare("newest ephemeral", requireV04NewestEphemeral)),
		step(`^a pressure victim is elected atomically$`, driver.exerciseReservationScenarioV04),
		step(`^only the newest ephemeral is selected before any service$`, assert("newest ephemeral")),
		step(`^service owners and no eligible ephemeral owner$`, prepare("newest service", requireV04NewestService)),
		step(`^only the newest service is selected$`, assert("newest service")),
		step(`^a selected remote ephemeral owner that ignores termination$`, driver.remoteSheddingOwnerV04),
		step(`^its owning guard performs bounded shedding$`, driver.exerciseRemoteSheddingV04),
		step(`^it escalates to a kill and no additional victim is selected before owned release$`, driver.requireRemoteSheddingV04),
		step(`^selected ownership disappears while its process group remains live$`, driver.replacedRemoteOwnerV04),
		step(`^the selecting guard observes that disappearance$`, driver.exerciseReplacedRemoteOwnerV04),
		step(`^no signal reaches the unowned process group and disappearance ends observation$`, driver.requireReplacedRemoteOwnerV04),
		step(`^a selected remote owner whose owning guard is temporarily unresponsive$`, driver.unresponsiveRemoteOwnerV04),
		step(`^the selecting guard observes that owner through a bounded deadline$`, driver.exerciseUnresponsiveRemoteOwnerV04),
		step(`^the remote child is not signaled and remains the global no-cascade barrier$`, driver.requireUnresponsiveRemoteOwnerV04),
		step(`^a selected remote owner and a held coordination mutation lock$`, prepare("bounded remote observation", requireV04BoundedRemoteObservation)),
		step(`^the selecting guard waits through its observation deadline$`, driver.exerciseReservationScenarioV04),
		step(`^it returns boundedly without signaling and preserves the selected barrier$`, assert("bounded remote observation")),
		step(`^a peer holding the shared coordination lock while a guard activates and supervises its child$`,
			prepare("shared-root contention", requireV04ContentionDefersInsteadOfFailing)),
		step(`^the guard activates its reservation and then samples through the held lock$`, driver.exerciseReservationScenarioV04),
		step(`^activation returns the retryable deferral exit and supervision keeps its healthy child$`, assert("shared-root contention")),
		step(`^a running reserved owner and a coordination mutation lock held by another goroutine in the same process$`, prepare("bounded owner cancellation", requireV04BoundedOwnerCancellation)),
		step(`^its owning guard is cancelled and reaps the child$`, driver.exerciseReservationScenarioV04),
		step(`^cleanup defers boundedly without a caller failure, with exact ledger bytes and an externally locked identity until a later atomic retry after lock release$`, assert("bounded owner cancellation")),
		step(`^a queued reservation waiter and a held coordination mutation lock$`, prepare("cancelled waiter cleanup", requireV04CancelledWaiterCleanup)),
		step(`^its acquisition context is cancelled before the lock is released$`, driver.exerciseReservationScenarioV04),
		step(`^bounded cleanup removes the waiter without blocking the FIFO queue$`, assert("cancelled waiter cleanup")),
		step(`^a queued reservation waiter whose coordination lock remains held past its fresh cleanup deadline$`, prepare("failed cancelled waiter cleanup", requireV04FailedCancelledWaiterCleanup)),
		step(`^its acquisition context is cancelled and bounded cleanup cannot mutate the ledger$`, driver.exerciseReservationScenarioV04),
		step(`^the waiter identity and exact FIFO bytes remain and a following FIFO waiter admits only after automatic head cleanup$`, assert("failed cancelled waiter cleanup")),
		step(`^a compiled guard supervising a long-lived child in an isolated process group$`, prepare("supervisor death ownership", requireV04SupervisorDeathOwnership)),
		step(`^only the guard is killed and reservation liveness is reconciled$`, driver.exerciseReservationScenarioV04),
		step(`^the running child keeps its reservation until the child group exits$`, assert("supervisor death ownership")),
		step(`^a reserved child whose TERM and KILL waits remain unconfirmed$`, prepare("bounded cancellation retirement", requireV04BoundedGuardCancellation)),
		step(`^its owning guard is cancelled$`, driver.exerciseReservationScenarioV04),
		step(`^cancellation returns boundedly while reservation and port competitors remain deferred until later retirement and then admit$`, assert("bounded cancellation retirement")),
		step(`^selected reserved owners for storage and non-storage pressure whose KILL waits remain unconfirmed$`, prepare("bounded shedding retirement", requireV04BoundedGuardShedding)),
		step(`^each owning guard performs bounded shedding$`, driver.exerciseReservationScenarioV04),
		step(`^each returns exit 73 or 75 respectively while reservation and port competitors defer until retirement and then admit$`, assert("bounded shedding retirement")),
		step(`^a guarded child holding a real leased port through a kernel lifetime identity$`, prepare("port supervisor lifetime", requireV04PortSupervisorLifetime)),
		step(`^only its HIPPO supervisor is killed$`, driver.exerciseReservationScenarioV04),
		step(`^a competitor cannot reclaim the port until the child group exits$`, assert("port supervisor lifetime")),
		step(`^a tokenized live port lease whose identity file is (missing|replaced)$`, func(fault string) error {
			return driver.prepareReservationScenarioV04("port identity "+fault, func(string) error {
				return requireV04PortIdentityCorruption(fault)
			})
		}),
		step(`^another owner requests that port in the same supervisor process$`, driver.exerciseReservationScenarioV04),
		step(`^the existing marker is preserved and the port remains unavailable$`, func() error {
			if err := driver.requireReservationScenarioV04("port identity missing"); err == nil {
				return nil
			}

			return driver.requireReservationScenarioV04("port identity replaced")
		}),
		step(`^an old and replacement port lease handle for the same process owner$`, prepare("port handle ABA", requireV04PortHandleABA)),
		step(`^the old handle attempts to release the replacement marker$`, driver.exerciseReservationScenarioV04),
		step(`^token authentication rejects the stale handle and preserves the replacement lease$`, assert("port handle ABA")),
		step(`^live and positively stale schema-one port markers without identity tokens$`, prepare("legacy port identity", requireV04LegacyPortCompatibility)),
		step(`^a v0\.4 owner requests each legacy port$`, driver.exerciseReservationScenarioV04),
		step(`^the live PID remains unavailable and only the positively stale PID is reclaimed$`, assert("legacy port identity")),
		step(`^a reserved leader that exits zero after spawning a background descendant without an inherited unknown file descriptor$`, prepare("background descendant lifetime", requireV04BackgroundDescendantLifetime)),
		step(`^the direct leader has been reaped$`, driver.exerciseReservationScenarioV04),
		step(`^reservation and port ownership remain until the process group retires and are later reclaimed safely$`, assert("background descendant lifetime")),
		step(`^an inherited reserved session that spawns a background descendant without an inherited unknown file descriptor$`, prepare("inherited descendant lifetime", requireV04InheritedDescendantLifetime)),
		step(`^the inherited direct leader exits zero$`, driver.exerciseReservationScenarioV04),
		step(`^its reservation and port evidence remain until the inherited process group retires$`, assert("inherited descendant lifetime")),
		step(`^a lifetime launcher that remains stopped before reporting payload activation$`, prepare("bounded lifetime handshake", requireV04BoundedLifetimeHandshake)),
		step(`^its owning guard reaches the activation deadline$`, driver.exerciseReservationScenarioV04),
		step(`^the guard returns boundedly without releasing the launcher's detectable reservation and port ownership$`, assert("bounded lifetime handshake")),
		step(`^a compiled (ordinary|inherited) reserved child with reservation and port identity locks$`, func(mode string) error {
			return driver.prepareReservationScenarioV04("payload identity isolation", func(string) error {
				return requireV04PayloadIdentityIsolation(mode)
			})
		}),
		step(`^the payload enumerates descriptors and attempts to unlock every inherited file$`, driver.exerciseReservationScenarioV04),
		step(`^no private identity is observable or unlockable and competitors remain blocked through group retirement$`, assert("payload identity isolation")),
		step(`^a payload leader exits after starting a no-identity-descriptor descendant but its launcher cannot report activation$`, prepare("failed activation identity", requireV04FailedActivationIdentity)),
		step(`^failed-launch cleanup reaches its bounded retirement decision$`, driver.exerciseReservationScenarioV04),
		step(`^no payload inherits identity and reservation plus port evidence remain until positive group retirement$`, assert("failed activation identity")),
		step(`^a compiled reserved payload and a launcher-owned activation channel$`, prepare("activation report isolation", requireV04ActivationReportIsolation)),
		step(`^the payload writes a forged process group to the inherited report descriptor$`, driver.exerciseReservationScenarioV04),
		step(`^the descriptor is closed in the payload and only the launcher report reaches the supervisor$`, assert("activation report isolation")),
		step(`^a live reservation (owner|waiter) whose token identity path is (missing|replaced)$`, func(record, fault string) error {
			return driver.prepareReservationScenarioV04("reservation identity path corruption", func(string) error {
				return requireV04ReservationIdentityPathCorruption(record, fault)
			})
		}),
		step(`^status and competing admission reconcile that ledger$`, driver.exerciseReservationScenarioV04),
		step(`^exact owner capacity or FIFO position remains until the original identity retires and later reconciliation succeeds$`, assert("reservation identity path corruption")),
		step(`^a long-lived caller whose guarded run returns before child-group retirement is confirmed$`, prepare("abandoned local handles", requireV04AbandonedLocalHandles)),
		step(`^the lifetime holder retires later without the caller process exiting$`, driver.exerciseReservationScenarioV04),
		step(`^the same caller can safely reclaim both reservation capacity and the leased port$`, assert("abandoned local handles")),
		step(`^a direct HIPPO invocation with the internal environment and launcher argument but no capability pipes$`, prepare("lifetime capability", requireV04LifetimeCapability)),
		step(`^the public command boundary interprets the invocation$`, driver.exerciseReservationScenarioV04),
		step(`^no payload bypasses admission and the invocation is rejected as an ordinary invalid command$`, assert("lifetime capability")),
		step(`^a compiled schema-one (heavy|service) guard with a live child group$`, func(class string) error {
			return driver.prepareReservationScenarioV04("schema one supervisor lifetime", func(string) error {
				return requireV04SchemaOneSupervisorLifetime(class)
			})
		}),
		step(`^only the compatibility supervisor is killed and ownership is reconciled$`, driver.exerciseReservationScenarioV04),
		step(`^reservation takeover remains deferred until the compatibility child group retires$`, assert("schema one supervisor lifetime")),
		step(`^a long-lived schema-one caller whose guarded group retirement is initially unconfirmed$`, prepare("schema one embedded retirement", requireV04SchemaOneEmbeddedRetirement)),
		step(`^its launcher and group later retire without the caller exiting$`, driver.exerciseReservationScenarioV04),
		step(`^compatibility ownership becomes reclaimable without treating the live caller PID as the owner$`, assert("schema one embedded retirement")),
		step(`^live and positively stale zero-metadata legacy (heavy|service) ownership records$`, func(class string) error {
			return driver.prepareReservationScenarioV04("legacy schema one ownership", func(string) error {
				return requireV04LegacySchemaOneOwnership(class)
			})
		}),
		step(`^compatibility liveness and reservation takeover reconcile each shared root$`, driver.exerciseReservationScenarioV04),
		step(`^the live PID record is retained and defers takeover while only the positively stale record is reclaimed$`, assert("legacy schema one ownership")),
		step(`^a reserved owner selected for storage shedding$`, driver.storageSheddingOwnerV04),
		step(`^its own guard observes the mark and terminates its child$`, driver.exerciseStorageOwnerSheddingV04),
		step(`^the child is reaped and the guard returns storage-blocked exit 73 before release$`, driver.requireStorageOwnerSheddingV04),
		step(`^only transactional owners remain under critical pressure$`, prepare("transactional protection", requireV04TransactionalProtection)),
		step(`^no victim is selected and new admission remains blocked$`, assert("transactional protection")),
		step(`^valid schema one and schema two local configurations$`, prepare("configuration schemas", func(root string) error {
			if driver.mode == e2eMode {
				return driver.requireCompiledConfigurationV04(root)
			}
			return requireV04Configuration(root)
		})),
		step(`^both configuration documents are loaded$`, driver.exerciseReservationScenarioV04),
		step(`^schema one selects exclusive mode and schema two validates reservation limits$`, assert("configuration schemas")),
		step(`^maximum representable and first overflowing MiB values for CLI and configuration$`, prepare("checked MiB conversion", func(root string) error {
			return driver.requireCheckedMiBConversionV04(root)
		})),
		step(`^reservation memory inputs are parsed$`, driver.exerciseReservationScenarioV04),
		step(`^the maximum stays exact and overflow requires replanning before multiplication$`, assert("checked MiB conversion")),
		step(`^a schema two custom profile extending constrained without an artificial concurrency cap$`, prepare("custom profile reservation", requireV04CustomProfileReservation)),
		step(`^its automatic reservation and guarded summary are produced$`, driver.exerciseReservationScenarioV04),
		step(`^it inherits two shares and reports the fixed allocated CPU as concurrency$`, assert("custom profile reservation")),
		step(`^live owners and waiters with different maximum-owner settings$`, driver.mixedOwnerLimitsV04),
		step(`^a looser configuration requests another reservation$`, driver.requestLooserOwnerLimitV04),
		step(`^the strictest live shared-root owner limit remains authoritative$`, driver.requireConservativeOwnerLimitV04),
		step(`^a larger-capacity epoch with a live owner and FIFO waiter$`, prepare("active epoch capacity", requireV04ActiveEpochCapacity)),
		step(`^a caller with a lower CPU or memory cap requests admission$`, driver.exerciseReservationScenarioV04),
		step(`^it defers without changing the epoch and the lower cap starts only after idle$`, assert("active epoch capacity")),
		step(`^reservation epochs at the maximum FIFO sequence with live and stale identities$`, prepare("FIFO sequence exhaustion", requireV04SequenceExhaustion)),
		step(`^another reservation requests admission in each epoch$`, driver.exerciseReservationScenarioV04),
		step(`^the live epoch stays byte-unchanged while the stale empty epoch restarts safely$`, assert("FIFO sequence exhaustion")),
		step(`^a reservation ledger with an invalid or duplicate owner token$`, func() error { return driver.prepareCorruptReservationLedgerV04(tokenField) }),
		step(`^a reservation ledger with an unknown workload class$`, func() error { return driver.prepareCorruptReservationLedgerV04(classField) }),
		step(`^a reservation ledger with duplicate or nonmonotonic sequences$`, func() error { return driver.prepareCorruptReservationLedgerV04("sequence") }),
		step(`^a reservation ledger with negative or inconsistent resource vectors$`, func() error { return driver.prepareCorruptReservationLedgerV04("vector") }),
		step(`^a reservation ledger whose valid individual vectors overflow aggregate arithmetic$`, func() error { return driver.prepareCorruptReservationLedgerV04("overflow") }),
		step(`^a reservation ledger whose allocated totals exceed capacity$`, func() error { return driver.prepareCorruptReservationLedgerV04("total") }),
		step(`^a reservation ledger with impossible owner and waiter structure$`, func() error { return driver.prepareCorruptReservationLedgerV04("structure") }),
		step(`^reservation state is decoded by coordination$`, driver.inspectCorruptReservationLedgerV04),
		step(`^admission fails closed and preserves the ledger bytes$`, func() error { return driver.requireCorruptReservationLedgerV04(tokenField) }),
		step(`^admission fails closed and preserves the class-corrupt ledger bytes$`, func() error { return driver.requireCorruptReservationLedgerV04(classField) }),
		step(`^admission fails closed and preserves the sequence-corrupt ledger bytes$`, func() error { return driver.requireCorruptReservationLedgerV04("sequence") }),
		step(`^admission fails closed and preserves the vector-corrupt ledger bytes$`, func() error { return driver.requireCorruptReservationLedgerV04("vector") }),
		step(`^admission fails closed and preserves the overflow-corrupt ledger bytes$`, func() error { return driver.requireCorruptReservationLedgerV04("overflow") }),
		step(`^a live owner consumes maximum-width CPU and memory capacity$`, prepare("maximum-width admission", requireV04MaximumWidthAdmission)),
		step(`^another minimum reservation requests admission$`, driver.exerciseReservationScenarioV04),
		step(`^admission defers without wrapping or changing allocated totals$`, assert("maximum-width admission")),
		step(`^admission fails closed and preserves the total-corrupt ledger bytes$`, func() error { return driver.requireCorruptReservationLedgerV04("total") }),
		step(`^admission fails closed and preserves the structure-corrupt ledger bytes$`, func() error { return driver.requireCorruptReservationLedgerV04("structure") }),
		step(`^a live reservation whose identity probe returns an unknown filesystem error$`, prepare("unknown identity error", requireV04UnknownIdentityError)),
		step(`^status and admission reconcile reservation liveness$`, driver.exerciseReservationScenarioV04),
		step(`^both fail closed without changing bytes or exposing the identity$`, assert("unknown identity error")),
		step(`^a reservation marker and live identity lock without its ledger$`, prepare("missing live ledger", requireV04MissingLiveLedger)),
		step(`^status and another admission inspect that root$`, driver.exerciseReservationScenarioV04),
		step(`^both fail closed without recreating or overbooking accounting$`, assert("missing live ledger")),
		step(`^privacy-safe reservation and development evidence$`, prepare("schema four summary", func(root string) error {
			if driver.mode == e2eMode {
				return driver.requireCompiledSummaryV04(root)
			}
			return requireV04SummaryAndRetention(root)
		})),
		step(`^status and the lifetime summary are encoded$`, driver.exerciseReservationScenarioV04),
		step(`^schema four reports requested allocated waited and peak totals without private inputs$`, assert("schema four summary")),
		step(`^individually valid waiters whose CPU or memory demand overflows aggregate arithmetic$`, prepare("waiter aggregate overflow", requireV04WaiterAggregateOverflow)),
		step(`^schema four reservation status aggregates the queued demand$`, driver.exerciseReservationScenarioV04),
		step(`^status fails closed with a privacy-safe coordination error$`, assert("waiter aggregate overflow")),
		step(`^a guarded child whose concurrent owner count rises after admission$`, driver.risingOwnerCountV04),
		step(`^reservation totals are sampled throughout supervision$`, driver.sampleLifetimeOwnerPeakV04),
		step(`^the schema four summary reports the highest observed owner count$`, driver.requireLifetimeOwnerPeakV04),
		step(`^a guarded child with an overlapping owner shorter than one host sampling interval$`, prepare("short overlap peak", requireV04ShortOverlapPeak)),
		step(`^the overlapping reservation is admitted and released during supervision$`, driver.exerciseReservationScenarioV04),
		step(`^the schema four summary includes that complete short-lived overlap$`, assert("short overlap peak")),
		step(`^aged live reservation waiter and coordination files$`, prepare("reservation retention", requireV04Retention)),
		step(`^reservation evidence retention is enforced$`, driver.exerciseReservationScenarioV04),
		step(`^protocol files remain while expired evidence is removed$`, assert("reservation retention")),
		step(`^fresh actively-written coordination-marker and reservation-ledger temporary files$`, prepare("atomic coordination temp retention", requireV04AtomicTempRetention)),
		step(`^recognized atomic-write temporary files remain untouched$`, assert("atomic coordination temp retention")),
		step(`^expired orphaned coordination-marker and reservation-ledger temporary files$`, prepare("orphaned atomic temp retention", requireV04OrphanedAtomicTempRetention)),
		step(`^the orphaned temporary files are reclaimed without changing stable protocol files$`, assert("orphaned atomic temp retention")),
		step(`^two parallel reserved guards and concurrently finalized evidence entries$`, prepare("concurrent evidence disappearance", requireV04ConcurrentEvidenceCleanup)),
		step(`^their evidence cleanup snapshots race with entry disappearance$`, driver.exerciseReservationScenarioV04),
		step(`^both commands complete without retry and their observed evidence remains$`, assert("concurrent evidence disappearance")),
	}
}

func (driver *Driver) terminalBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a guarded process with inherited controlling-terminal input$`, func() error {
			return driver.prepareReservationScenarioV04("terminal ownership", func(string) error { return driver.requirePendingTerminalV04() })
		}),
		step(`^the child reads from the terminal in its own process group$`, driver.exerciseReservationScenarioV04),
		step(`^it completes without SIGTTIN and original foreground ownership is restored$`, func() error {
			return driver.requireReservationScenarioV04("terminal ownership")
		}),
	}
}

func (driver *Driver) conformanceBindings() []contract.StepBinding { //nolint:funlen // The declarative inventory keeps every canonical conformance phrase explicit and auditable.
	prepare := func(name string, action func(string) error) func() error {
		return func() error { return driver.prepareReservationScenarioV04(name, action) }
	}
	assert := func(name string) func() error {
		return func() error { return driver.requireReservationScenarioV04(name) }
	}
	return []contract.StepBinding{
		step(`^a strict manifest containing four temporary consumer checkouts$`, func() error {
			return driver.prepareReservationScenarioV04("generic conformance", func(string) error { return driver.requirePendingConformanceV04() })
		}),
		step(`^the generic consumer conformance harness runs$`, driver.exerciseReservationScenarioV04),
		step(`^identity coordination gate and unchanged-checkout assertions pass without compiled product defaults$`, func() error {
			return driver.requireReservationScenarioV04("generic conformance")
		}),
		step(`^four manifest consumers including absolute and symlink aliases of one checkout$`, driver.aliasedConsumerManifestV04),
		step(`^the conformance manifest is validated$`, driver.validateAliasedConsumerManifestV04),
		step(`^canonical checkout identity is rejected without exposing its path$`, driver.requireAliasedConsumerRejectedV04),
		step(`^a conformance manifest with an empty raw HIPPO binary path$`, prepare("empty HIPPO binary input", func(root string) error {
			return requireV04EmptyConformanceInput(root, "binary")
		})),
		step(`^a conformance manifest with an empty raw shared-root path$`, prepare("empty shared-root input", func(root string) error {
			return requireV04EmptyConformanceInput(root, "shared root")
		})),
		step(`^a conformance manifest inside a valid checkout with one empty raw consumer path$`, prepare("empty consumer path input", func(root string) error {
			return requireV04EmptyConformanceInput(root, "consumer")
		})),
		step(`^the raw manifest inputs are validated$`, driver.exerciseReservationScenarioV04),
		step(`^the empty binary input is rejected privately before path resolution$`, assert("empty HIPPO binary input")),
		step(`^the empty shared-root input is rejected privately before path resolution$`, assert("empty shared-root input")),
		step(`^the empty consumer path is rejected privately before checkout commands$`, assert("empty consumer path input")),
		step(`^four clean consumers whose bootstrap coordination or gate command mutates a checkout and fails$`, driver.failingConformancePhasesV04),
		step(`^each failure phase is exercised independently$`, driver.exerciseFailingConformancePhasesV04),
		step(`^all checkouts were snapshotted before commands and every mutation is reported$`, driver.requireFailingConformanceReconciledV04),
		step(`^a manifest with relative inputs inherited HIPPO keys and retargetable checkout aliases$`, func() error {
			return driver.prepareReservationScenarioV04("canonical conformance inputs", requireV04CanonicalConformanceInputs)
		}),
		step(`^consumer commands inspect their working directory environment and frozen checkout identity$`, driver.exerciseReservationScenarioV04),
		step(`^only canonical manifest inputs are used and shared-root checkout aliases are rejected privately$`, func() error {
			return driver.requireReservationScenarioV04("canonical conformance inputs")
		}),
		step(`^the conformance caller has one live reservation session and one repository default config$`, prepare("conformance caller session isolation", requireV04ConformanceCallerSessionIsolation)),
		step(`^four consumer commands inspect their base environment and independent owners establish overlapping sessions$`, driver.exerciseReservationScenarioV04),
		step(`^no consumer receives the caller token or default config and independent owners still exhaust real capacity$`, assert("conformance caller session isolation")),
		step(`^a bootstrap replaces its validated checkout directory with a clean checkout at the same path$`, prepare("replaced conformance checkout identity", requireV04ReplacedConformanceCheckout)),
		step(`^a later gate would run in the replacement with the same commit and clean status$`, driver.exerciseReservationScenarioV04),
		step(`^conformance rejects the changed checkout identity before the gate and reconciles conservatively$`, assert("replaced conformance checkout identity")),
		step(`^a bootstrap replaces the created shared root with (an empty directory|a symlink)$`, func(replacement string) error {
			return driver.prepareReservationScenarioV04("replaced conformance shared root", func(root string) error {
				return requireV04ReplacedConformanceSharedRoot(root, replacement)
			})
		}),
		step(`^a later coordination command would use the replacement root$`, driver.exerciseReservationScenarioV04),
		step(`^conformance rejects the changed shared-root identity privately before the command starts$`, assert("replaced conformance shared root")),
		step(`^an owned command group that is signalled after its members exited and reaped only after the stop grace$`, func() error {
			return driver.prepareReservationScenarioV04("reaped group retirement", requireV04ReapedGroupRetirement)
		}),
		step(`^the conformance runner retires that group$`, driver.exerciseReservationScenarioV04),
		step(`^retirement reports no failure for a group the host had already stopped$`, func() error {
			return driver.requireReservationScenarioV04("reaped group retirement")
		}),
		step(`^a conformance leader that exits on TERM while its TERM-ignoring descendant writes a late checkout mutation$`, func() error {
			return driver.prepareReservationScenarioV04("conformance leader-first cancellation", requireV04ConformanceLeaderFirstCancellation)
		}),
		step(`^the conformance caller is cancelled$`, driver.exerciseReservationScenarioV04),
		step(`^the full owned command group retires before fresh-deadline reconciliation reports the late mutation boundedly$`, func() error {
			return driver.requireReservationScenarioV04("conformance leader-first cancellation")
		}),
		step(`^a conformance leader that exits (zero|nonzero) while leaving a TERM-ignoring descendant to write a late checkout mutation$`, func(exit string) error {
			return driver.prepareReservationScenarioV04("conformance completed descendant", func(root string) error {
				return requireV04ConformanceCompletedDescendant(root, exit)
			})
		}),
		step(`^the leader exits before its descendant$`, driver.exerciseReservationScenarioV04),
		step(`^the full owned command group retires before the completed command is reconciled$`, assert("conformance completed descendant")),
		step(`^a cancelled conformance command whose force-stop reap cannot complete$`, prepare("bounded conformance reap", requireV04BoundedConformanceReap)),
		step(`^cancellation reaches its post-kill observation deadline$`, driver.exerciseReservationScenarioV04),
		step(`^conformance returns a private bounded reap failure and still reconciles checkouts$`, assert("bounded conformance reap")),
		step(`^a consumer makes verified binary storage temporarily unremovable$`, prepare("verified binary cleanup failure", requireV04VerifiedBinaryCleanupFailure)),
		step(`^conformance completes command execution and checkout reconciliation$`, driver.exerciseReservationScenarioV04),
		step(`^cleanup failure joins the result without exposing the storage path$`, assert("verified binary cleanup failure")),
		step(`^a bootstrap sequence cancelled after its first command$`, prepare("pre-cancelled conformance command", requireV04PreCancelledConformanceCommand)),
		step(`^the next command reaches its start boundary$`, driver.exerciseReservationScenarioV04),
		step(`^the next command never starts and final identity and checkout reconciliation still run$`, assert("pre-cancelled conformance command")),
		step(`^a valid manifest whose bootstrap replaces the verified HIPPO binary identity$`, func() error {
			return driver.prepareReservationScenarioV04("conformance binary replacement", requireV04ConformanceBinaryReplacement)
		}),
		step(`^a later coordination or gate command would execute that binary$`, driver.exerciseReservationScenarioV04),
		step(`^conformance fails before changed bytes execute and still reconciles every checkout$`, func() error {
			return driver.requireReservationScenarioV04("conformance binary replacement")
		}),
		step(`^a valid HIPPO source that changes after command verification$`, prepare("pinned conformance binary", requireV04PinnedBinaryIdentity)),
		step(`^the consumer invokes HIPPO through its provided environment$`, driver.exerciseReservationScenarioV04),
		step(`^only pinned verified bytes execute and the changed source is reported$`, assert("pinned conformance binary")),
		step(`^a valid manifest whose bootstrap overwrites its provided HIPPO binary while the source stays unchanged$`, prepare("tampered pinned conformance binary", requireV04TamperedPinnedBinary)),
		step(`^a later gate would execute the provided binary$`, driver.exerciseReservationScenarioV04),
		step(`^conformance rejects the changed pinned object before unverified bytes execute$`, assert("tampered pinned conformance binary")),
		step(`^one gate pauses after verification while another gate overwrites their provided HIPPO binary$`, prepare("parallel pinned conformance binary", requireV04ParallelPinnedBinary)),
		step(`^the paused gate invokes its already-verified binary path$`, driver.exerciseReservationScenarioV04),
		step(`^only verified bytes execute and the replacement attempt remains observable$`, assert("parallel pinned conformance binary")),
		step(`^an allow-capacity-skip coordination check that exits 75 after (pinned binary tamper|verified cleanup failure)$`, func(failure string) error {
			return driver.prepareReservationScenarioV04("capacity skip integrity", func(string) error {
				return requireV04CapacitySkipIntegrity(failure)
			})
		}),
		step(`^conformance classifies the joined coordination outcome$`, driver.exerciseReservationScenarioV04),
		step(`^the integrity failure remains fatal and private instead of being skipped$`, assert("capacity skip integrity")),
		step(`^a manifest whose HIPPO binary identity is a FIFO$`, prepare("conformance binary FIFO", requireV04ConformanceBinaryFIFO)),
		step(`^conformance validates the binary identity$`, driver.exerciseReservationScenarioV04),
		step(`^validation rejects the special file without blocking or exposing its path$`, assert("conformance binary FIFO")),
		step(`^a manifest whose HIPPO binary identity is a directory$`, prepare("conformance binary directory", requireV04ConformanceBinaryDirectory)),
		step(`^validation rejects the directory without exposing its path$`, assert("conformance binary directory")),
		step(`^a manifest whose HIPPO binary is a non-executable regular file$`, prepare("conformance binary executable mode", requireV04ConformanceBinaryMode)),
		step(`^validation rejects the non-executable file without exposing its path$`, assert("conformance binary executable mode")),
		step(`^a consumer command with a missing absolute executable$`, prepare("conformance missing command privacy", requireV04MissingCommandPrivacy)),
		step(`^conformance attempts to start the command$`, driver.exerciseReservationScenarioV04),
		step(`^the missing-command error names only the consumer phase and safe failure category$`, assert("conformance missing command privacy")),
		step(`^a consumer command removes its checkout before the next command boundary$`, prepare("conformance checkout command privacy", requireV04InvalidCheckoutPrivacy)),
		step(`^conformance revalidates the checkout identity after that command$`, driver.exerciseReservationScenarioV04),
		step(`^the invalid-checkout error names only the consumer phase and safe identity category$`, assert("conformance checkout command privacy")),
		step(`^a conformance (unavailable manifest|uncreatable shared root) containing a unique private path sentinel$`, func(surface string) error {
			return driver.prepareReservationScenarioV04("conformance setup privacy", func(string) error {
				return requireV04ConformanceSetupPrivacy(surface)
			})
		}),
		step(`^conformance attempts filesystem setup$`, driver.exerciseReservationScenarioV04),
		step(`^the setup error names only its safe category and never the private path$`, assert("conformance setup privacy")),
		step(`^four consumers with sequential local bootstrap commands and multiple failing lanes$`, func() error {
			return driver.prepareReservationScenarioV04("parallel conformance bootstrap", requireV04ParallelConformanceBootstrap)
		}),
		step(`^the generic harness executes the bootstrap phase$`, driver.exerciseReservationScenarioV04),
		step(`^consumer lanes overlap failures aggregate deterministically and no later phase starts$`, func() error {
			return driver.requireReservationScenarioV04("parallel conformance bootstrap")
		}),
	}
}

func (driver *Driver) admissionBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^three healthy host samples$`, driver.threeHealthy),
		step(`^development admission is assessed$`, driver.assessAdmission),
		step(`^the work is admitted$`, driver.requireAdmitted),
		step(`^a full stable Darwin warning window with safe headroom$`, driver.stableDarwinWarning),
		step(`^ephemeral work is admitted with concurrency one$`, driver.requireDegradedAdmitted),
		step(`^Darwin warning samples with excessive compressor growth$`, driver.growingDarwinWarning),
		step(`^degraded work is deferred$`, driver.requireDegradedDeferred),
		step(`^stable Darwin warning samples for a transactional task$`, driver.strictDarwinWarning),
		step(`^swap-outs grow by 128 MiB over 15 seconds$`, driver.swapGrowth),
		step(`^development pressure is assessed$`, driver.assessPressure),
		step(`^the state is warning because of swap pressure$`, func() error { return driver.requireReason("swap-warning") }),
		step(`^compressor payload is 12 GiB and grows 1 GiB over 15 seconds$`, driver.compressorGrowth),
		step(`^the state is warning because of compressor pressure$`, func() error { return driver.requireReason("compressor-warning") }),
		step(`^a healthy 5 GiB runner without swap$`, driver.smallRunner),
		step(`^the constrained profile is selected$`, driver.requireConstrained),
		step(`^a healthy 1 GiB machine without swap$`, driver.tinyMachine),
		step(`^the minimal profile is selected with concurrency one$`, driver.requireMinimal),
		step(`^a host sample below the 256 MiB disk floor$`, driver.exhaustedDisk),
		step(`^admission is storage blocked with exit 73$`, driver.requireStorageBlocked),
		step(`^a strict transactional task that does not fit its requested profile$`, driver.strictTransaction),
		step(`^admission requires replanning with exit 78$`, driver.requireReplan),
	}
}

func (driver *Driver) executionBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^an empty shared coordination root$`, driver.emptyCoordinationRoot),
		step(`^exclusive heavy work acquires a guarded session$`, driver.acquireExclusiveCompatibility),
		step(`^the shared root advertises exclusive coordination$`, driver.requireExclusiveCoordination),
		step(`^releasing its final session removes the coordination marker$`, driver.releaseFinalCoordinationSession),
		step(`^the shared root advertises reservation coordination$`, driver.reservationCoordination),
		step(`^every compatibility task class requests a guarded session$`, driver.requestEveryCompatibilityClass),
		step(`^every compatibility owner is deferred with exit 75$`, driver.requireEveryCoordinationOwnerDeferred),
		step(`^the reservation coordination marker remains unchanged$`, driver.requireReservationCoordinationUnchanged),
		step(`^another live process owns the heavy lease$`, driver.liveLease),
		step(`^a second owner waits for the lease$`, driver.waitLease),
		step(`^the second owner is deferred with exit 75$`, driver.requireDeferred),
		step(`^the deferral names the process holding the lease$`, driver.requireDeferralNamesHolder),
		step(`^a live service session owns its resource lease$`, driver.liveServiceLease),
		step(`^heavy work requests the lease$`, driver.heavyRequestsLease),
		step(`^the heavy owner acquires the lease immediately$`, driver.requireHeavyAcquired),
		step(`^two live service sessions on separate ports$`, driver.twoServiceSessions),
		step(`^each service child validates its inherited session$`, driver.validateInheritedSessions),
		step(`^both inherited sessions remain valid$`, driver.requireInheritedSessionsValid),
		step(`^a valid inherited resource session$`, driver.inherited),
		step(`^a guarded child exits successfully$`, driver.successfulChild),
		step(`^the child exits successfully and the inherited session remains owned$`, driver.requirePreserved),
		step(`^an admitted guarded command$`, driver.givenAdmitted),
		step(`^the guarded child exits with code 17$`, driver.child17),
		step(`^the guard exits with code 17$`, driver.require17),
		step(`^an admitted command without consumer concurrency mappings$`, driver.admittedWithoutConcurrencyMappings),
		step(`^an admitted command with explicit consumer concurrency mappings$`, driver.admittedWithConcurrencyMappings),
		step(`^the guarded child inspects its environment$`, driver.inspectGuardedEnvironment),
		step(`^only canonical HIPPO concurrency variables are added$`, driver.requireCanonicalConcurrencyOnly),
		step(`^missing mappings receive resolved concurrency and caller values remain unchanged$`, driver.requireMappedConcurrency),
		step(`^invalid and reserved consumer concurrency mappings$`, driver.invalidConcurrencyMappings),
		step(`^each mapped guarded command is requested$`, driver.requestInvalidConcurrencyMappings),
		step(`^every command is rejected before its child starts$`, driver.requireInvalidMappingsRejected),
		step(`^an admitted guarded child with piped input and separate output streams$`, driver.admittedGuardedStreams),
		step(`^the guarded child copies all three standard streams$`, driver.copyGuardedStreams),
		step(`^stdin reaches the child and stdout and stderr remain separate$`, driver.requireComposableStreams),
		step(`^an admitted child that ignores termination$`, driver.stubbornChild),
		step(`^the guard is interrupted$`, driver.interruptGuard),
		step(`^the child is signalled once and force-stopped within the grace$`, driver.requireForceStopped),
		step(`^an admitted ephemeral child encounters critical pressure$`, driver.criticalChild),
		step(`^the guard observes the critical sample$`, driver.observeCritical),
		step(`^the guard terminates its child and exits with code 75$`, driver.requireShed),
		step(`^an admitted degraded ephemeral child encounters growing compressor pressure$`, driver.degradedGrowthChild),
		step(`^the guard observes warning through the grace$`, driver.observeDegradedWarning),
		step(`^the degraded child starts and is terminated with exit 75$`, driver.requireDegradedShed),
		step(`^a service port already leased by a live owner$`, func() error {
			return driver.prepareReservationScenarioV04("held service port", requireV04HeldPortDefersContender)
		}),
		step(`^another service requests that same port$`, driver.exerciseReservationScenarioV04),
		step(`^the contender is deferred with exit 75 instead of failing$`, func() error {
			return driver.requireReservationScenarioV04("held service port")
		}),
		step(`^a guarded child process group whose members exited without being reaped$`, func() error {
			return driver.prepareReservationScenarioV04("exited unreaped group stop", requireV04ExitedUnreapedGroupStop)
		}),
		step(`^its owning guard stops that process group$`, driver.exerciseReservationScenarioV04),
		step(`^the stop reports no supervision failure$`, func() error {
			return driver.requireReservationScenarioV04("exited unreaped group stop")
		}),
		step(`^a killed child group that retires after an aggressive termination grace$`, func() error {
			return driver.prepareReservationScenarioV04("aggressive grace retirement", requireV04AggressiveGraceRetirement)
		}),
		step(`^its owning guard waits for that retirement$`, driver.exerciseReservationScenarioV04),
		step(`^retirement is confirmed instead of leaving the reservation owned$`, func() error {
			return driver.requireReservationScenarioV04("aggressive grace retirement")
		}),
		step(`^an admitted child that ignores termination during a collector failure$`, driver.stubbornCollectorFailureChild),
		step(`^guarded supervision loses host evidence$`, driver.loseHostEvidence),
		step(`^the child is reaped before its resource lease is released$`, driver.requireReapedBeforeRelease),
	}
}

func (driver *Driver) evidenceBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a guarded evidence stream with small test chunks$`, driver.smallEvidenceStream),
		step(`^the stream exceeds all retained raw chunks$`, driver.overflowEvidenceChunks),
		step(`^only the bounded newest raw chunks remain$`, driver.requireBoundedEvidenceChunks),
		step(`^the evidence summary counts every recorded sample$`, driver.requireLifetimeEvidenceSummary),
		step(`^twenty live evidence streams in one shared root$`, driver.twentyLiveEvidenceStreams),
		step(`^another evidence stream starts$`, driver.startExcessEvidenceStream),
		step(`^the new stream is rejected before raw evidence is created$`, driver.requireExcessEvidenceRejected),
		step(`^inactive evidence above the shared storage budget$`, driver.inactiveEvidenceAboveBudget),
		step(`^evidence retention is enforced$`, driver.enforceEvidenceRetention),
		step(`^the oldest inactive evidence is removed below the budget$`, driver.requireInactiveEvidencePruned),
	}
}

func (driver *Driver) publicCLIBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^the compiled HIPPO binary$`, driver.compiledBinary),
		step(`^JSON version is requested$`, driver.jsonVersion),
		step(`^version schema identifies the release and commit$`, driver.requireVersion),
		step(`^JSON status is requested for an existing path$`, driver.jsonStatus),
		step(`^status returns schema version 4 with profile capability and coordination evidence$`, driver.requireStatus),
		step(`^the compiled HIPPO binary with corrupt reservation coordination state$`, driver.corruptStatusCoordinationV04),
		step(`^JSON status is requested for that coordination root$`, driver.requestCorruptCoordinationStatusV04),
		step(`^status reports the coordination error instead of schema four zero totals$`, driver.requireCorruptCoordinationStatusV04),
		step(`^schema one and schema two configs paired with opposite active markers$`, driver.statusMarkerPrecedenceV04),
		step(`^JSON status is requested for each marked and empty root$`, driver.requestStatusMarkerPrecedenceV04),
		step(`^active marker modes win symmetrically and empty roots use configured modes$`, driver.requireStatusMarkerPrecedenceV04),
		step(`^the compiled HIPPO binary with individually valid overflowing reservation waiters$`, driver.compiledWaiterOverflowStatusV04),
		step(`^JSON status aggregates the queued reservation demand$`, driver.requestCompiledWaiterOverflowStatusV04),
		step(`^status reports a privacy-safe coordination error instead of wrapped totals$`, driver.requireCompiledWaiterOverflowStatusV04),
		step(`^root command help is requested$`, driver.rootHelp),
		step(`^help lists the public command tree and exits successfully$`, driver.requireHelp),
		step(`^help expands HIPPO as Host Infrastructure Pressure and Process Orchestrator$`, driver.requireHIPPOExpansion),
		step(`^release command help is requested$`, driver.releaseHelp),
		step(`^help lists the release command tree and exits successfully$`, driver.requireReleaseHelp),
		step(`^Zsh completion is requested$`, driver.zshCompletion),
		step(`^a Zsh completion script is emitted$`, driver.requireZshCompletion),
		step(`^a coordination root whose lock is briefly held by a peer$`, func() error {
			return driver.prepareReservationScenarioV04("busy status root", requireV04StatusWaitsOutContention)
		}),
		step(`^JSON status is requested while that peer still holds the lock$`, driver.exerciseReservationScenarioV04),
		step(`^status reports its coordination totals instead of a contention error$`, func() error {
			return driver.requireReservationScenarioV04("busy status root")
		}),
		step(`^a runtime failure and a usage error are requested$`, driver.requestRuntimeFailureAndUsageError),
		step(`^only the usage error prints the command usage block$`, driver.requireUsageOnlyForUsageErrors),
		step(`^an unknown command is requested$`, driver.unknownCommand),
		step(`^Cobra reports the command and exits with code 1$`, driver.requireCobraDiagnostic),
		step(`^run is requested without a command separator$`, driver.invalidRun),
		step(`^the command fails with a useful validation error$`, driver.requireValidation),
		step(`^operand-free commands are requested with positional arguments$`, driver.requestOperandFreeCommandsWithArguments),
		step(`^every command rejects the unexpected argument$`, driver.requireOperandFreeCommandsRejected),
		step(`^a healthy release summary file$`, func() error { return driver.summary(0) }),
		step(`^release summary assessment is requested$`, driver.assessSummary),
		step(`^the release evidence is accepted$`, driver.requireAccepted),
		step(`^a healthy release summary on standard input$`, driver.healthySummaryInput),
		step(`^release summary assessment is requested from standard input$`, driver.assessSummaryInput),
		step(`^the release evidence is accepted on standard output$`, driver.requireAcceptedOutput),
		step(`^repeated and changing resource states$`, driver.repeatedChangingStates),
		step(`^JSON development monitoring is requested$`, driver.monitorJSON),
		step(`^one valid JSON record is emitted for each state transition$`, driver.requireJSONTransitions),
		step(`^release monitor output paths without endpoint inputs$`, driver.releasePathsWithoutEndpoints),
		step(`^a bounded release monitor with raw standard output$`, driver.boundedReleaseRawOutput),
		step(`^a bounded release monitor with summary standard output$`, driver.boundedReleaseSummaryOutput),
		step(`^release monitoring completes$`, driver.completeReleaseMonitor),
		step(`^raw JSON lines use standard output and the summary remains a file$`, driver.requireRawStandardOutput),
		step(`^the final summary uses standard output and raw evidence remains a file$`, driver.requireSummaryStandardOutput),
		step(`^release raw evidence and summary both target standard output$`, driver.mixedReleaseStandardOutput),
		step(`^release monitoring is requested$`, driver.requestReleaseMonitoring),
		step(`^the command rejects mixed standard output before collecting evidence$`, driver.requireMixedOutputRejected),
		step(`^a release raw stream whose downstream writer fails$`, driver.failingReleaseRawOutput),
		step(`^release monitoring writes its first sample$`, driver.completeReleaseMonitor),
		step(`^monitoring fails without closing the caller-owned stream$`, driver.requireStreamFailure),
		step(`^the command rejects a missing generic health URL$`, driver.requireMissingHealthURL),
		step(`^an explicit HIPPO config with an unknown field$`, driver.invalidExplicitConfig),
		step(`^JSON status is requested with that config$`, driver.statusWithConfig),
		step(`^configuration fails with exit 78$`, driver.requireConfigExit),
	}
}

func (driver *Driver) releaseBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a release host below its requested balanced capacity$`, driver.releaseHost),
		step(`^release admission is assessed$`, driver.assessRelease),
		step(`^the release requires replanning instead of automatic fallback$`, driver.requireReleaseCPU),
		step(`^a release summary with one health failure$`, driver.failedSummary),
		step(`^release overlap evidence is assessed$`, driver.assessSummary),
		step(`^the release evidence is rejected$`, driver.requireRejected),
		step(`^a release summary outside the routed responsiveness budget$`, driver.slowRoutedSummary),
	}
}

func (driver *Driver) artifactBindings() []contract.StepBinding {
	prepare := func(name string, action func(string) error) func() error {
		return func() error { return driver.prepareReservationScenarioV04(name, action) }
	}
	assert := func(name string) func() error {
		return func() error { return driver.requireReservationScenarioV04(name) }
	}
	return []contract.StepBinding{
		step(`^build artifact tracking is inspected$`, driver.inspectBuildCaching),
		step(`^generated release binaries are ignored$`, driver.requireBuildCacheDisabled),
		step(`^its compiled binary lifecycle is inspected$`, driver.inspectE2ELifecycle),
		step(`^the end-to-end binary is removed after the run$`, driver.requireE2ECleanup),
		step(`^four historical bootstrap generations$`, driver.historicalGenerations),
		step(`^the current bootstrap generation runs$`, driver.runCurrentBootstrap),
		step(`^only the current and two recent generations remain$`, driver.requireRetention),
		step(`^tracked and ignored paths are inspected$`, driver.inspectArtifactPolicy),
		step(`^local config, generated artifacts, and generated-reports are ignored and untracked$`, driver.requirePrivateArtifacts),
		step(`^the local config example remains tracked$`, driver.requireExampleTracked),
		step(`^the standalone layout has no Nx metadata$`, driver.requireApplicationLayout),
		step(`^malformed and exact semantic release versions$`, prepare("release version syntax", requireV04ReleaseVersionSyntax)),
		step(`^each release version is validated independently$`, driver.exerciseReservationScenarioV04),
		step(`^only exact v-prefixed semantic versions pass$`, assert("release version syntax")),
		step(`^short uppercase nonexistent and full real release commits$`, prepare("release commit identity", requireV04ReleaseCommitIdentity)),
		step(`^each release commit is validated independently$`, driver.exerciseReservationScenarioV04),
		step(`^only a full lowercase real commit passes$`, assert("release commit identity")),
		step(`^a clean fixture repository with a real non-HEAD commit$`, prepare("release HEAD equality", requireV04ReleaseHeadEquality)),
		step(`^a release is requested for the non-HEAD commit$`, driver.exerciseReservationScenarioV04),
		step(`^the release is rejected before output$`, assert("release HEAD equality")),
		step(`^a fixture repository with one tracked change$`, prepare("release tracked cleanliness", requireV04ReleaseTrackedClean)),
		step(`^its release identity is validated$`, driver.exerciseReservationScenarioV04),
		step(`^tracked dirt rejects the release$`, assert("release tracked cleanliness")),
		step(`^a fixture repository with one untracked file$`, prepare("release untracked cleanliness", requireV04ReleaseUntrackedClean)),
		step(`^untracked dirt rejects the release$`, assert("release untracked cleanliness")),
		step(`^invalid release identity (version syntax|commit syntax|nonexistent commit|non-HEAD commit|tracked dirt|untracked dirt|git status failure)$`, func(identity string) error {
			return driver.prepareReservationScenarioV04("release pre-output boundary", func(root string) error {
				return requireV04ReleaseNoOutputClass(root, identity)
			})
		}),
		step(`^its output boundary is inspected after rejection$`, driver.exerciseReservationScenarioV04),
		step(`^no release output directory exists$`, assert("release pre-output boundary")),
		step(`^a clean release identity whose git status command fails$`, prepare("release status failure", requireV04ReleaseStatusFailure)),
		step(`^release cleanliness is inspected$`, driver.exerciseReservationScenarioV04),
		step(`^the release fails before creating output$`, assert("release status failure")),
		step(`^clean committed source shadowed by ignored global and repository-excluded Go files$`, prepare("release exact committed source", requireV04ReleaseExactSource)),
		step(`^release binaries are built from the checkout identity$`, driver.exerciseReservationScenarioV04),
		step(`^only exact committed source affects assets and isolated build state is removed$`, assert("release exact committed source")),
		step(`^a controlled temporary directory and a release build that (succeeds|build fails|second allocation fails)$`, func(result string) error {
			return driver.prepareReservationScenarioV04("release temporary cleanup", func(root string) error {
				return requireV04ReleaseTempCleanup(root, result)
			})
		}),
		step(`^the release build finishes$`, driver.exerciseReservationScenarioV04),
		step(`^no source or binary staging directory remains$`, assert("release temporary cleanup")),
		step(`^the version-four runtime source and conformance inventory$`, prepare("release source inventory", requireV04ReleaseInventory)),
		step(`^repository artifact policy is executed$`, driver.exerciseReservationScenarioV04),
		step(`^every version-four source and example is explicitly protected$`, assert("release source inventory")),
		step(`^one clean exact release identity$`, prepare("release exact asset set", requireV04ReleaseAssetSet)),
		step(`^its platform asset names and checksums are validated$`, driver.exerciseReservationScenarioV04),
		step(`^exactly four supported archives and four matching checksums exist$`, assert("release exact asset set")),
		step(`^one release archive for every supported platform$`, prepare("release archive member", requireV04ReleaseArchiveMember)),
		step(`^each archive member and mode is inspected$`, driver.exerciseReservationScenarioV04),
		step(`^each archive contains only an executable hippo member$`, assert("release archive member")),
		step(`^release archives with nonzero numeric or noncanonical named ownership$`, prepare("release archive ownership", requireV04ReleaseArchiveOwnership)),
		step(`^each archive header is inspected without extraction$`, driver.exerciseReservationScenarioV04),
		step(`^ownership must be uid zero gid zero and root root$`, assert("release archive ownership")),
		step(`^one-member release archives whose hippo entries are symbolic links hard links and special files$`, prepare("release archive link", requireV04ReleaseArchiveLink)),
		step(`^archive metadata is validated before extraction$`, driver.exerciseReservationScenarioV04),
		step(`^every unsafe member is rejected without following its host target$`, assert("release archive link")),
		step(`^cross-platform release binaries from one clean commit$`, prepare("release binary identity", requireV04ReleaseBinaryIdentity)),
		step(`^VCS metadata and the native version command are inspected$`, driver.exerciseReservationScenarioV04),
		step(`^every binary has that revision and clean state and the native JSON matches$`, assert("release binary identity")),
		step(`^valid release assets in a directory whose path contains spaces$`, prepare("release quoted native path", requireV04ReleaseSpacedPath)),
		step(`^the native version command is executed$`, driver.exerciseReservationScenarioV04),
		step(`^its quoted binary path emits the exact release identity$`, assert("release quoted native path")),
		step(`^reachable annotated and lightweight release tags$`, prepare("release tag peeling", requireV04ReleaseTagPeeling)),
		step(`^the release workflow resolves each tag identity$`, driver.exerciseReservationScenarioV04),
		step(`^each tag is compared by its peeled commit$`, assert("release tag peeling")),
		step(`^a reachable release tag and a different reachable event commit$`, prepare("release event commit equality", requireV04ReleaseEventCommitMismatch)),
		step(`^the release workflow compares their identities$`, driver.exerciseReservationScenarioV04),
		step(`^the mismatched event commit is rejected before publishing$`, assert("release event commit equality")),
		step(`^reachable and unreachable peeled release commits$`, prepare("release main reachability", requireV04ReleaseMainReachability)),
		step(`^the release workflow applies its ancestry gate$`, driver.exerciseReservationScenarioV04),
		step(`^only the origin-main-reachable commit passes$`, assert("release main reachability")),
		step(`^the repository Actions storage policy$`, prepare("release storage neutrality", requireV04ReleaseStorageNeutrality)),
		step(`^release workflow actions and permissions are inspected$`, driver.exerciseReservationScenarioV04),
		step(`^release publishing adds no artifact package or cache storage$`, assert("release storage neutrality")),
		step(`^release asset jobs that configure Go$`, prepare("release cache disabled", requireV04ReleaseCacheDisabled)),
		step(`^their setup-go storage configuration is inspected$`, driver.exerciseReservationScenarioV04),
		step(`^each release asset job explicitly disables Go caching$`, assert("release cache disabled")),
		step(`^the CI release asset build and validation steps$`, prepare("CI release commit identity", requireV04CIValidatorCommit)),
		step(`^their command arguments are compared$`, driver.exerciseReservationScenarioV04),
		step(`^the validator receives the same exact commit as the builder$`, assert("CI release commit identity")),
	}
}

func (driver *Driver) qualityGateBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^lint gate wiring is inspected$`, driver.inspectLintEnforcement),
		step(`^the configuration enables all linters and unlimited findings$`, driver.requireExhaustiveLint),
		step(`^exported documentation diagnostics remain errors$`, driver.requirePackageDocumentation),
		step(`^the quick gate invokes module-local lint$`, driver.requireModuleScopedLint),
		step(`^behavior adapter wiring is inspected$`, driver.inspectBehaviourCoverage),
		step(`^each registered adapter uses strict step resolution$`, driver.requireStrictAdapters),
		step(`^each registered pattern owns a function handler$`, driver.requirePairedBindings),
		step(`^the unit adapter permits no behavior exemptions$`, driver.requireNoUnitExemptions),
		step(`^every configured behavior exemption names a concrete boundary and reason$`, driver.requireApprovedExemptions),
		step(`^the quick gate invokes each compliance adapter serially$`, driver.requireSerialCompliance),
		step(`^compiled end-to-end behavior runs only in the full gate$`, driver.requireE2EPlacement),
		step(`^contributor gate wiring is inspected$`, driver.inspectContributorEnforcement),
		step(`^the commit hook and CI invoke conventional commit validation$`, driver.requireConventionalCommits),
		step(`^pre-commit invokes staged formatting for supported files$`, driver.requireStagedFormatting),
		step(`^pre-push invokes the direct quick gate without Nx$`, driver.requirePushQuickGate),
		step(`^the quick gate invokes deterministic core coverage at 99 percent$`, driver.requireCoreCoverage),
	}
}

func (driver *Driver) portabilityBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^Linux host and cgroup memory evidence$`, driver.linuxCgroupCapacity),
		step(`^the Linux evidence is collected$`, driver.collectLinuxEvidence),
		step(`^effective memory is 4 GiB$`, driver.requireFourGiB),
		step(`^Linux reports no usable swap$`, driver.linuxWithoutSwap),
		step(`^swap is unavailable without causing critical pressure$`, driver.requireSwapUnavailable),
		step(`^Linux memory PSI some average is 10 percent$`, driver.linuxPSIWarning),
		step(`^the state is warning because of memory PSI$`, driver.requirePSIWarning),
	}
}
