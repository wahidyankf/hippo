package support

import (
	"slices"

	"github.com/wahidyankf/resource-guard/tests/contract"
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
		driver.executionBindings(),
		driver.publicCLIBindings(),
		driver.releaseBindings(),
		driver.artifactBindings(),
		driver.qualityGateBindings(),
		driver.portabilityBindings(),
	)
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
		step(`^an admitted child that ignores termination$`, driver.stubbornChild),
		step(`^the guard is interrupted$`, driver.interruptGuard),
		step(`^the child is signalled once and force-stopped within the grace$`, driver.requireForceStopped),
		step(`^an admitted ephemeral child encounters critical pressure$`, driver.criticalChild),
		step(`^the guard observes the critical sample$`, driver.observeCritical),
		step(`^the guard terminates its child and exits with code 75$`, driver.requireShed),
		step(`^an admitted degraded ephemeral child encounters growing compressor pressure$`, driver.degradedGrowthChild),
		step(`^the guard observes warning through the grace$`, driver.observeDegradedWarning),
		step(`^the degraded child starts and is terminated with exit 75$`, driver.requireDegradedShed),
	}
}

func (driver *Driver) publicCLIBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^the compiled resource guard binary$`, driver.compiledBinary),
		step(`^JSON version is requested$`, driver.jsonVersion),
		step(`^version schema identifies the release and commit$`, driver.requireVersion),
		step(`^JSON status is requested for an existing path$`, driver.jsonStatus),
		step(`^status returns schema version 3 with profile and capability evidence$`, driver.requireStatus),
		step(`^root command help is requested$`, driver.rootHelp),
		step(`^help lists the public command tree and exits successfully$`, driver.requireHelp),
		step(`^release command help is requested$`, driver.releaseHelp),
		step(`^help lists the release command tree and exits successfully$`, driver.requireReleaseHelp),
		step(`^Zsh completion is requested$`, driver.zshCompletion),
		step(`^a Zsh completion script is emitted$`, driver.requireZshCompletion),
		step(`^an unknown command is requested$`, driver.unknownCommand),
		step(`^Cobra reports the command and exits with code 1$`, driver.requireCobraDiagnostic),
		step(`^run is requested without a command separator$`, driver.invalidRun),
		step(`^the command fails with a useful validation error$`, driver.requireValidation),
		step(`^a healthy release summary file$`, func() error { return driver.summary(0) }),
		step(`^release summary assessment is requested$`, driver.assessSummary),
		step(`^the release evidence is accepted$`, driver.requireAccepted),
		step(`^release monitor output paths without endpoint inputs$`, driver.releasePathsWithoutEndpoints),
		step(`^release monitoring is requested$`, driver.requestReleaseMonitoring),
		step(`^the command rejects a missing generic health URL$`, driver.requireMissingHealthURL),
		step(`^an explicit resource guard config with an unknown field$`, driver.invalidExplicitConfig),
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
	return []contract.StepBinding{
		step(`^build artifact tracking is inspected$`, driver.inspectBuildCaching),
		step(`^generated release binaries are ignored$`, driver.requireBuildCacheDisabled),
		step(`^its compiled binary lifecycle is inspected$`, driver.inspectE2ELifecycle),
		step(`^the end-to-end binary is removed after the run$`, driver.requireE2ECleanup),
		step(`^four historical bootstrap generations$`, driver.historicalGenerations),
		step(`^the current bootstrap generation runs$`, driver.runCurrentBootstrap),
		step(`^only the current and two recent generations remain$`, driver.requireRetention),
		step(`^tracked and ignored paths are inspected$`, driver.inspectArtifactPolicy),
		step(`^local config and generated artifacts are ignored and untracked$`, driver.requirePrivateArtifacts),
		step(`^the local config example remains tracked$`, driver.requireExampleTracked),
		step(`^the standalone layout has no Nx metadata$`, driver.requireApplicationLayout),
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
		step(`^every configured behavior exemption has a nonempty reason$`, driver.requireApprovedExemptions),
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
