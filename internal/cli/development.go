package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/wahidyankf/resource-guard/internal/guard"
	"github.com/wahidyankf/resource-guard/internal/host"
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

func withAssessmentDecision(resolution guard.Resolution, assessment guard.Assessment) guard.Resolution {
	if resolution.ExitCode != 0 {
		return resolution
	}

	if assessment.StorageBlocked {
		resolution.Decision, resolution.ExitCode = "cleanup", guard.StorageBlockedExitCode
	} else if assessment.State != "normal" {
		resolution.Decision, resolution.ExitCode, resolution.Retryable = "wait", guard.CapacityDeferredExitCode, true
	}

	return resolution
}

func (application Application) status(options statusOptions) (int, error) {
	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return guard.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}

	first, err := application.Collector.Collect(nil, options.diskPath)
	if err != nil {
		return 1, err
	}

	application.Sleep(time.Second)

	second, err := application.Collector.Collect(first.CPUState, options.diskPath)
	if err != nil {
		return 1, err
	}

	resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, "ephemeral", second.Sample)
	if resolveError != nil {
		return guard.ReplanRequiredExitCode, resolveError
	}

	assessment := guard.ResourceAssessment([]guard.Sample{first.Sample, second.Sample}, resolution.Policy)
	resolution = withAssessmentDecision(resolution, assessment)

	if options.jsonOutput {
		payload := struct {
			guard.Sample

			Resource   guard.Assessment `json:"resource"`
			Profile    guard.Resolution `json:"profile"`
			ConfigHash string           `json:"configHash,omitempty"`
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
		available = fmt.Sprintf("%.2f", float64(*availableBytes)/float64(guard.GiB))
	}
	if second.Sample.DiskFreeBytes != nil {
		disk = fmt.Sprintf("%.2f", float64(*second.Sample.DiskFreeBytes)/float64(guard.GiB))
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

func (application Application) monitor(options monitorOptions) (int, error) {
	if options.interval <= 0 {
		return 1, errors.New("interval must be positive")
	}

	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return guard.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}

	var previous guard.CPUState

	samples := []guard.Sample{}
	prior := ""

	observe := func() error {
		reading, err := application.Collector.Collect(previous, options.diskPath)
		if err != nil {
			return err
		}

		previous = reading.CPUState
		samples = append(samples, reading.Sample)
		if len(samples) > 17 {
			samples = samples[len(samples)-17:]
		}

		resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, "ephemeral", reading.Sample)
		if resolveError != nil {
			return resolveError
		}

		assessment := guard.ResourceAssessment(samples, resolution.Policy)
		state := assessment.State + ":" + assessment.Reason + ":" + resolution.ResolvedProfile

		if state != prior {
			_, writeError := fmt.Fprintf(
				application.Stdout,
				"%s state=%s reason=%s profile=%s swap=%s\n",
				reading.Sample.MeasuredAt,
				assessment.State,
				assessment.Reason,
				resolution.ResolvedProfile,
				reading.Sample.SwapState,
			)
			if writeError != nil {
				return writeError
			}

			prior = state
		}

		return nil
	}

	if err := observe(); err != nil {
		return 1, err
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	ticker := time.NewTicker(options.interval)
	defer ticker.Stop()

	for {
		select {
		case <-signals:
			return 0, nil
		case <-ticker.C:
			if err := observe(); err != nil {
				return 1, err
			}
		}
	}
}

func (application Application) run(options runOptions) (int, error) {
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
		return guard.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}

	probeDiskPath := options.diskPath
	if probeDiskPath == "" {
		probeDiskPath = options.workingDir
	}
	if probeDiskPath == "" {
		probeDiskPath = "."
	}

	probe, collectError := application.Collector.Collect(nil, probeDiskPath)
	if collectError != nil {
		return 1, collectError
	}

	resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, options.class, probe.Sample)
	if resolveError != nil {
		return guard.ReplanRequiredExitCode, resolveError
	}
	if resolution.ExitCode != 0 {
		_, _ = fmt.Fprintf(
			application.Stderr,
			"Resource guard decision=%s requested=%s resolved=%s.\n",
			resolution.Decision,
			resolution.RequestedProfile,
			resolution.ResolvedProfile,
		)

		return resolution.ExitCode, nil
	}

	return guard.Run(guard.RunConfig{
		Command:          options.command[0],
		Arguments:        options.command[1:],
		TaskClass:        options.class,
		WorkingDirectory: options.workingDir,
		Environment:      application.Environment,
		EvidenceRoot:     root,
		DiskPath:         options.diskPath,
		LeasePort:        options.leasePort,
		LeaseOwner:       options.leaseOwner,
		LeaseMinimum:     options.leaseMinimum,
		LeaseMaximum:     options.leaseMaximum,
		Collector:        application.Collector,
		Policy:           resolution.Policy,
		Resolution:       resolution,
		ConfigHash:       configuration.Hash,
		Sleep:            application.Sleep,
		Now:              application.Now,
		Stderr:           application.Stderr,
	})
}
