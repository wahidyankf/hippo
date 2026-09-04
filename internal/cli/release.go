package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
	releaseguard "github.com/wahidyankf/hippo/internal/release"
)

func (application Application) releaseCheck(ctx context.Context, options releaseCheckOptions) (int, error) {
	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return policy.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
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

	if err := releaseguard.CheckWithPolicy(ctx, application.Collector, options.diskPath, application.Sleep, resolution.Policy); err != nil {
		_, _ = fmt.Fprintln(application.Stderr, err)

		return guard.CapacityDeferredExitCode, nil
	}

	return 0, nil
}

func (application Application) releaseAssess(_ context.Context, options releaseAssessOptions) (int, error) {
	if options.summaryPath == "" {
		return 1, errors.New("--summary is required")
	}

	if _, configError := application.loadConfig(options.configPath); configError != nil {
		return policy.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
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
		_, _ = fmt.Fprintln(application.Stderr, err)

		return guard.CapacityDeferredExitCode, nil
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
	if options.durationMs < 0 {
		return 1, errors.New("duration-ms must be nonnegative")
	}
	if options.durationMs > math.MaxInt64/int64(time.Millisecond) {
		return 1, errors.New("duration-ms exceeds the supported range")
	}
	if err := validateServicePorts(options.servicePorts); err != nil {
		return 1, err
	}
	if options.outputPath == "-" && options.summaryPath == "-" {
		return 1, errors.New("raw evidence and summary cannot both use standard output")
	}

	if _, configError := application.loadConfig(options.configPath); configError != nil {
		return policy.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
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
	if err != nil {
		return 1, err
	}

	return 0, nil
}
