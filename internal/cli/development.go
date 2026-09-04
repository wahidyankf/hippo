package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/policy"
)

func (application Application) version(options versionOptions) (int, error) {
	if options.jsonOutput {
		encoded, err := json.Marshal(struct {
			SchemaVersion int    `json:"schemaVersion"`
			Version       string `json:"version"`
			Commit        string `json:"commit"`
		}{
			SchemaVersion: 1,
			Version:       application.Version,
			Commit:        application.Commit,
		})
		if err != nil {
			return 1, fmt.Errorf("encode version JSON: %w", err)
		}

		_, err = fmt.Fprintln(application.Stdout, string(encoded))

		return 0, err
	}

	_, err := fmt.Fprintf(application.Stdout, "%s (%s)\n", application.Version, application.Commit)

	return 0, err
}

func withAssessmentDecision(resolution policy.Resolution, assessment policy.Assessment) policy.Resolution {
	if resolution.ExitCode != 0 {
		return resolution
	}

	if assessment.StorageBlocked {
		resolution.Decision, resolution.ExitCode = policy.DecisionCleanup, guard.StorageBlockedExitCode
	} else if assessment.State != policy.StateNormal {
		resolution.Decision, resolution.ExitCode, resolution.Retryable = policy.DecisionWait, guard.CapacityDeferredExitCode, true
	}

	return resolution
}

func (application Application) status(ctx context.Context, options statusOptions) (int, error) {
	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return policy.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}

	first, err := application.Collector.Collect(ctx, nil, options.diskPath)
	if err != nil {
		return 1, err
	}

	if err := waitForContext(ctx, time.Second, application.Sleep); err != nil {
		return 1, err
	}

	second, err := application.Collector.Collect(ctx, first.CPUState, options.diskPath)
	if err != nil {
		return 1, err
	}

	resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, policy.TaskEphemeral, second.Sample)
	if resolveError != nil {
		return policy.ReplanRequiredExitCode, resolveError
	}

	assessment := policy.ResourceAssessment([]policy.Sample{first.Sample, second.Sample}, resolution.Policy)
	resolution = withAssessmentDecision(resolution, assessment)

	if options.jsonOutput {
		payload := struct {
			policy.Sample

			Resource   policy.Assessment `json:"resource"`
			Profile    policy.Resolution `json:"profile"`
			ConfigHash string            `json:"configHash,omitempty"`
		}{second.Sample, assessment, resolution, configuration.Hash}
		encoded, marshalError := json.Marshal(payload)
		if marshalError != nil {
			return 1, fmt.Errorf("encode status JSON: %w", marshalError)
		}

		_, err = fmt.Fprintln(application.Stdout, string(encoded))

		return 0, err
	}

	available, disk, cpu := unavailableValue, unavailableValue, unavailableValue
	availableBytes := second.Sample.AvailableMemoryBytes
	if availableBytes == nil {
		availableBytes = second.Sample.AvailableNonCompressedEstimateBytes
	}

	if availableBytes != nil {
		available = fmt.Sprintf("%.2f", float64(*availableBytes)/float64(policy.GiB))
	}
	if second.Sample.DiskFreeBytes != nil {
		disk = fmt.Sprintf("%.2f", float64(*second.Sample.DiskFreeBytes)/float64(policy.GiB))
	}
	if second.Sample.CPUUtilizationPercent != nil {
		cpu = fmt.Sprintf("%.1f%%", *second.Sample.CPUUtilizationPercent)
	}

	_, err = fmt.Fprintf(
		application.Stdout,
		"state=%s reason=%s profile=%s concurrency=%d swap=%s availableGiB=%s diskFreeGiB=%s cpu=%s\n",
		assessment.State,
		assessment.Reason,
		resolution.ResolvedProfile,
		resolution.Concurrency,
		second.Sample.SwapState,
		available,
		disk,
		cpu,
	)

	return 0, err
}

func waitForContext(ctx context.Context, duration time.Duration, pause func(time.Duration)) error {
	if pause != nil {
		pause(duration)

		return ctx.Err()
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func writeMonitorTransition(
	destination io.Writer,
	jsonOutput bool,
	sample policy.Sample,
	assessment policy.Assessment,
	resolution policy.Resolution,
) error {
	if !jsonOutput {
		_, err := fmt.Fprintf(
			destination,
			"%s state=%s reason=%s profile=%s swap=%s\n",
			sample.MeasuredAt,
			assessment.State,
			assessment.Reason,
			resolution.ResolvedProfile,
			sample.SwapState,
		)

		return err
	}

	encoded, err := json.Marshal(struct {
		SchemaVersion int          `json:"schemaVersion"`
		MeasuredAt    string       `json:"measuredAt"`
		State         policy.State `json:"state"`
		Reason        string       `json:"reason"`
		Profile       string       `json:"profile"`
		SwapState     string       `json:"swapState"`
	}{1, sample.MeasuredAt, assessment.State, assessment.Reason, resolution.ResolvedProfile, sample.SwapState})
	if err != nil {
		return fmt.Errorf("encode monitor transition JSON: %w", err)
	}

	_, err = fmt.Fprintln(destination, string(encoded))

	return err
}

func (application Application) monitor(ctx context.Context, options monitorOptions) (int, error) {
	if options.interval <= 0 {
		return 1, errors.New("interval must be positive")
	}

	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return policy.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}

	var previous policy.CPUState

	samples := []policy.Sample{}
	prior := ""

	observe := func() error {
		reading, err := application.Collector.Collect(ctx, previous, options.diskPath)
		if err != nil {
			return err
		}

		previous = reading.CPUState
		samples = append(samples, reading.Sample)
		if len(samples) > 17 {
			samples = samples[len(samples)-17:]
		}

		resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, policy.TaskEphemeral, reading.Sample)
		if resolveError != nil {
			return resolveError
		}

		assessment := policy.ResourceAssessment(samples, resolution.Policy)
		state := string(assessment.State) + ":" + assessment.Reason + ":" + resolution.ResolvedProfile

		if state != prior {
			if err := writeMonitorTransition(application.Stdout, options.jsonOutput, reading.Sample, assessment, resolution); err != nil {
				return err
			}

			prior = state
		}

		return nil
	}

	if err := observe(); err != nil {
		return 1, err
	}

	ticker := time.NewTicker(options.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return 0, nil
		case <-ticker.C:
			if err := observe(); err != nil {
				return 1, err
			}
		}
	}
}

func (application Application) run(ctx context.Context, options runOptions) (int, error) {
	if options.workingDir != "" {
		absolute, err := filepath.Abs(options.workingDir)
		if err != nil {
			return 1, err
		}

		options.workingDir = absolute
	}

	root := host.DefaultEvidenceRoot(environmentMap(application.Environment))
	if root == "" {
		return 1, errors.New("resource evidence root is unavailable")
	}

	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return policy.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}

	probeDiskPath := options.diskPath
	if probeDiskPath == "" {
		probeDiskPath = options.workingDir
	}
	if probeDiskPath == "" {
		probeDiskPath = "."
	}

	probe, collectError := application.Collector.Collect(ctx, nil, probeDiskPath)
	if collectError != nil {
		return 1, collectError
	}

	taskClass := policy.TaskClass(options.class)
	resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, taskClass, probe.Sample)
	if resolveError != nil {
		return policy.ReplanRequiredExitCode, resolveError
	}
	if resolution.ExitCode != 0 {
		_, _ = fmt.Fprintf(
			application.Stderr,
			"HIPPO decision=%s requested=%s resolved=%s.\n",
			resolution.Decision,
			resolution.RequestedProfile,
			resolution.ResolvedProfile,
		)

		return resolution.ExitCode, nil
	}

	return guard.Run(ctx, guard.RunConfig{
		Command:                options.command[0],
		Arguments:              options.command[1:],
		TaskClass:              taskClass,
		WorkingDirectory:       options.workingDir,
		Environment:            application.Environment,
		EvidenceRoot:           root,
		DiskPath:               options.diskPath,
		LeasePort:              options.leasePort,
		LeaseOwner:             options.leaseOwner,
		LeaseMinimum:           options.leaseMinimum,
		LeaseMaximum:           options.leaseMaximum,
		ConcurrencyEnvironment: options.concurrencyEnvironment,
		Collector:              application.Collector,
		Policy:                 resolution.Policy,
		Resolution:             resolution,
		ConfigHash:             configuration.Hash,
		Sleep:                  application.Sleep,
		Now:                    application.Now,
		ChildStdin:             application.Stdin,
		ChildStdout:            application.Stdout,
		ChildStderr:            application.Stderr,
		Stderr:                 application.Stderr,
	})
}
