package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wahidyankf/resource-guard/internal/guard"
	releaseguard "github.com/wahidyankf/resource-guard/internal/release"
)

func (application Application) releaseCheck(options releaseCheckOptions) (int, error) {
	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return guard.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}

	probe, collectError := application.Collector.Collect(nil, options.diskPath)
	if collectError != nil {
		return 1, collectError
	}

	resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, "release", probe.Sample)
	if resolveError != nil {
		return guard.ReplanRequiredExitCode, resolveError
	}
	if resolution.ExitCode != 0 {
		return resolution.ExitCode, nil
	}

	if err := releaseguard.CheckWithPolicy(application.Collector, options.diskPath, application.Sleep, resolution.Policy); err != nil {
		_, _ = fmt.Fprintln(application.Stderr, err)

		return guard.CapacityDeferredExitCode, nil
	}

	return 0, nil
}

func (application Application) releaseAssess(options releaseAssessOptions) (int, error) {
	if options.summaryPath == "" {
		return 1, errors.New("--summary is required")
	}

	if _, configError := application.loadConfig(options.configPath); configError != nil {
		return guard.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}

	summary, err := releaseguard.AssessFile(options.summaryPath)
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

func (application Application) releaseMonitor(options releaseMonitorOptions) (int, error) {
	if options.durationMs < 0 {
		return 1, errors.New("duration-ms must be nonnegative")
	}
	if err := validateServicePorts(options.servicePorts); err != nil {
		return 1, err
	}

	if _, configError := application.loadConfig(options.configPath); configError != nil {
		return guard.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}

	err := releaseguard.RunMonitor(releaseguard.MonitorConfig{
		OutputPath:     options.outputPath,
		SummaryPath:    options.summaryPath,
		DeploymentRoot: options.deploymentRoot,
		HealthURL:      options.healthURL,
		RoutedOrigin:   options.routedOrigin,
		ServicePorts:   options.servicePorts,
		Duration:       time.Duration(options.durationMs) * time.Millisecond,
		Collector:      application.Collector,
	})
	if err != nil {
		return 1, err
	}

	return 0, nil
}
