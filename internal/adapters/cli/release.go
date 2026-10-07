package cli

import (
	"context"
	"encoding/json"
	"fmt"

	app "github.com/wahidyankf/hippo/internal/application"
)

func (boundary Application) releaseCheck(ctx context.Context, options releaseCheckOptions) (int, error) {
	return boundary.ReleaseServices.CheckRelease(ctx, app.ReleaseCheckRequest{ConfigPath: options.configPath, DiskPath: options.diskPath, RequestedProfile: options.requestedProfile()})
}

func (boundary Application) releaseAssess(_ context.Context, options releaseAssessOptions) (int, error) {
	result, code, err := boundary.ReleaseServices.AssessRelease(app.ReleaseAssessRequest{ConfigPath: options.configPath, SummaryPath: options.summaryPath, Input: boundary.Stdin})
	if !result.Assessed {
		return code, err
	}
	encoded, marshalError := json.Marshal(map[string]any{"accepted": result.Accepted, "schemaVersion": result.SchemaVersion})
	if marshalError != nil {
		return 1, fmt.Errorf("encode release assessment JSON: %w", marshalError)
	}
	if _, writeError := fmt.Fprintln(boundary.Stdout, string(encoded)); writeError != nil {
		return 1, writeError
	}
	return code, err
}

func (boundary Application) releaseMonitor(ctx context.Context, options releaseMonitorOptions) (int, error) {
	return boundary.ReleaseServices.MonitorRelease(ctx, app.ReleaseMonitorRequest{ConfigPath: options.configPath, DurationMs: options.durationMs, Stdout: boundary.Stdout, MonitorConfig: app.MonitorConfig{OutputPath: options.outputPath, SummaryPath: options.summaryPath, DeploymentRoot: options.deploymentRoot, HealthURL: options.healthURL, RoutedOrigin: options.routedOrigin, ServicePorts: options.servicePorts}}, boundary.MonitorRelease)
}
