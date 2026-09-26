package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
	releaseguard "github.com/wahidyankf/hippo/internal/release"
	"github.com/wahidyankf/hippo/internal/status"
)

func (application Application) releaseCheck(ctx context.Context, options releaseCheckOptions) (int, error) {
	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
	}

	probe, collectError := application.Collector.Collect(ctx, nil, options.diskPath)
	if collectError != nil {
		return 1, collectError
	}

	resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, policy.TaskRelease, probe.Sample)
	if resolveError != nil {
		return policy.ReplanRequiredExitCode, resolveError
	}
	if resolution.ExitCode != 0 {
		return resolution.ExitCode, nil
	}

	return releaseCheckVerdict(releaseguard.CheckWithPolicy(
		ctx, application.Collector, options.diskPath, application.Sleep, resolution.Policy,
	))
}

// releaseCheckVerdict turns a stability check's result into one truthful
// failure. Memory and CPU that do not settle defer the release: waiting can
// lift them. A disk below the reserve is the same storage block run reports
// for the same threshold: waiting does not free disk. Anything else is the
// check itself failing, which is not a deferral at all.
func releaseCheckVerdict(err error) (int, error) {
	switch {
	case err == nil:
		return 0, nil
	case errors.Is(err, releaseguard.ErrMemoryHeadroom), errors.Is(err, releaseguard.ErrCPUHeadroom):
		return 0, status.Fail(status.CodeLimitCapacityDeferred, "release check deferred: %v", err)
	case errors.Is(err, releaseguard.ErrDiskReserve):
		return 0, status.Fail(status.CodeLimitStorageBlocked, "release check blocked: %v; free space on --disk-path first", err)
	default:
		return 1, fmt.Errorf("release check: %w", err)
	}
}

func (application Application) releaseAssess(_ context.Context, options releaseAssessOptions) (int, error) {
	if options.summaryPath == "" {
		return 0, status.Fail(status.CodeArgsInvalid, "--summary is required")
	}

	if _, configError := application.loadConfig(options.configPath); configError != nil {
		return 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
	}

	var summary policy.ReleaseSummary
	var err error
	if options.summaryPath == "-" {
		summary, err = releaseguard.Assess(application.Stdin)
	} else {
		summary, err = releaseguard.AssessFile(options.summaryPath)
	}
	accepted := err == nil

	encoded, marshalError := json.Marshal(map[string]any{"accepted": accepted, "schemaVersion": summary.SchemaVersion})
	if marshalError != nil {
		return 1, fmt.Errorf("encode release assessment JSON: %w", marshalError)
	}
	if _, writeError := fmt.Fprintln(application.Stdout, string(encoded)); writeError != nil {
		return 1, writeError
	}

	if err != nil {
		// The assessment ran and turned the evidence down; nothing was
		// deferred, so the diagnostic says what was decided and why.
		return 0, status.Fail(status.CodeLimitCapacityDeferred, "release evidence rejected: %v", err)
	}

	return 0, nil
}

func validateServicePorts(servicePorts []int) error {
	for _, port := range servicePorts {
		if port <= 0 || port > 65_535 {
			return errors.New("service port must be between 1 and 65535")
		}
	}

	return nil
}

func (application Application) releaseMonitor(ctx context.Context, options releaseMonitorOptions) (int, error) {
	// Every check before the configuration loads is about the invocation
	// itself, so each refusal is a usage mistake.
	if options.durationMs < 0 {
		return 0, status.Fail(status.CodeArgsInvalid, "--duration-ms must be nonnegative")
	}
	if options.durationMs > math.MaxInt64/int64(time.Millisecond) {
		return 0, status.Fail(status.CodeArgsInvalid, "--duration-ms exceeds the supported range")
	}
	if err := validateServicePorts(options.servicePorts); err != nil {
		return 0, status.Fail(status.CodeArgsInvalid, "--service-port: %v", err)
	}
	if options.outputPath == "-" && options.summaryPath == "-" {
		return 0, status.Fail(status.CodeArgsInvalid, "raw evidence and summary cannot both use standard output")
	}

	if _, configError := application.loadConfig(options.configPath); configError != nil {
		return 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
	}

	monitorContext := ctx
	cancel := func() {}
	if options.durationMs > 0 {
		monitorContext, cancel = context.WithTimeout(ctx, time.Duration(options.durationMs)*time.Millisecond)
	}
	defer cancel()

	monitorConfig := releaseguard.MonitorConfig{
		OutputPath:     options.outputPath,
		SummaryPath:    options.summaryPath,
		DeploymentRoot: options.deploymentRoot,
		HealthURL:      options.healthURL,
		RoutedOrigin:   options.routedOrigin,
		ServicePorts:   options.servicePorts,
		Collector:      application.Collector,
	}
	if options.outputPath == "-" {
		monitorConfig.OutputPath = ""
		monitorConfig.RawOutput = application.Stdout
	}
	if options.summaryPath == "-" {
		monitorConfig.SummaryPath = ""
		monitorConfig.SummaryOutput = application.Stdout
	}

	err := application.MonitorRelease(monitorContext, monitorConfig)
	if errors.Is(err, releaseguard.ErrMonitorInput) {
		return 0, status.Fail(status.CodeArgsInvalid, "%v", err)
	}
	if err != nil {
		return 1, err
	}

	return 0, nil
}
