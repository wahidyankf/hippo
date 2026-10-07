package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// ReleaseCheckRequest selects configuration and host disk evidence for a release check.
type ReleaseCheckRequest struct {
	ConfigPath, DiskPath string
	RequestedProfile     policy.ProfileName
}

// ReleaseAssessRequest supplies a completed summary from a file or standard input.
type ReleaseAssessRequest struct {
	ConfigPath, SummaryPath string
	Input                   io.Reader
}

// ReleaseAssessment is an assessment result whose rendering remains at the driving adapter.
type ReleaseAssessment struct {
	Accepted      bool
	SchemaVersion int
	Assessed      bool
}

// ReleaseMonitorRequest supplies monitor destinations and a bounded release duration.
type ReleaseMonitorRequest struct {
	MonitorConfig

	ConfigPath string
	DurationMs int64
	Stdout     io.Writer
}

// CheckRelease selects configuration and the strict policy before consecutive sampling.
func (services ReleaseServices) CheckRelease(ctx context.Context, request ReleaseCheckRequest) (int, error) {
	configuration, configError := services.Configuration.Load(request.ConfigPath, services.Environment)
	if configError != nil {
		return 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
	}

	probe, collectError := services.Collector.Collect(ctx, nil, request.DiskPath)
	if collectError != nil {
		return 1, collectError
	}

	resolution, resolveError := configuration.Catalog.Resolve(request.RequestedProfile, policy.TaskRelease, probe.Sample)
	if resolveError != nil {
		return 0, policy.Stopped(policy.ReasonReplanRequired, resolveError)
	}
	if resolution.Reason != policy.ReasonNone {
		return 0, policy.Stopped(resolution.Reason, nil)
	}

	return releaseCheckVerdict(checkWithPolicyWait(ctx, services.Collector, request.DiskPath, resolution.Policy, services.Clock.Wait))
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
	case errors.Is(err, ErrMemoryHeadroom), errors.Is(err, ErrCPUHeadroom):
		return 0, status.Fail(status.CodeLimitCapacityDeferred, "release check deferred: %v", err)
	case errors.Is(err, ErrDiskReserve):
		return 0, status.Fail(status.CodeLimitStorageBlocked, "release check blocked: %v; free space on --disk-path first", err)
	default:
		return 1, fmt.Errorf("release check: %w", err)
	}
}

// AssessRelease reads and evaluates one completed release summary.
func (services ReleaseServices) AssessRelease(request ReleaseAssessRequest) (ReleaseAssessment, int, error) {
	if request.SummaryPath == "" {
		return ReleaseAssessment{}, 0, status.Fail(status.CodeArgsInvalid, "--summary is required")
	}

	if _, configError := services.Configuration.Load(request.ConfigPath, services.Environment); configError != nil {
		return ReleaseAssessment{}, 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
	}

	var summary policy.ReleaseSummary
	var err error
	if request.SummaryPath == "-" {
		summary, err = Assess(request.Input)
	} else {
		summary, err = services.AssessFile(request.SummaryPath)
	}
	if err != nil && !errors.Is(err, ErrHeadroomExhausted) {
		// Nothing was assessed, so no verdict is printed.
		return ReleaseAssessment{}, 0, status.Fail(status.CodeEvidenceUnreadable, "release summary is unusable: %v", err)
	}
	result := ReleaseAssessment{Assessed: true, Accepted: err == nil, SchemaVersion: summary.SchemaVersion}
	if err != nil {
		return result, 0, status.Fail(status.CodeLimitReleaseEnvelopeExceeded, "release evidence rejected: %v", err)
	}
	return result, 0, nil
}

func validateServicePorts(servicePorts []int) error {
	for _, port := range servicePorts {
		if port <= 0 || port > 65_535 {
			return errors.New("service port must be between 1 and 65535")
		}
	}

	return nil
}

// MonitorRelease prepares and executes the release monitor with the existing optional test hook.
func (services ReleaseServices) MonitorRelease(ctx context.Context, request ReleaseMonitorRequest, invoke func(context.Context, MonitorConfig) error) (int, error) {
	// Every check before the configuration loads is about the invocation
	// itself, so each refusal is a usage mistake.
	if request.DurationMs < 0 {
		return 0, status.Fail(status.CodeArgsInvalid, "--duration-ms must be nonnegative")
	}
	if request.DurationMs > math.MaxInt64/int64(time.Millisecond) {
		return 0, status.Fail(status.CodeArgsInvalid, "--duration-ms exceeds the supported range")
	}
	if err := validateServicePorts(request.ServicePorts); err != nil {
		return 0, status.Fail(status.CodeArgsInvalid, "--service-port: %v", err)
	}
	if request.OutputPath == "-" && request.SummaryPath == "-" {
		return 0, status.Fail(status.CodeArgsInvalid, "raw evidence and summary cannot both use standard output")
	}

	if _, configError := services.Configuration.Load(request.ConfigPath, services.Environment); configError != nil {
		return 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
	}

	monitorContext := ctx
	cancel := func() {}
	if request.DurationMs > 0 {
		monitorContext, cancel = context.WithTimeout(ctx, time.Duration(request.DurationMs)*time.Millisecond)
	}
	defer cancel()

	monitorConfig := request.MonitorConfig
	monitorConfig.Collector = services.Collector
	if invoke == nil {
		invoke = services.RunMonitor
	}
	if request.OutputPath == "-" {
		monitorConfig.OutputPath = ""
		monitorConfig.RawOutput = request.Stdout
	}
	if request.SummaryPath == "-" {
		monitorConfig.SummaryPath = ""
		monitorConfig.SummaryOutput = request.Stdout
	}

	err := invoke(monitorContext, monitorConfig)
	if errors.Is(err, ErrMonitorInput) {
		return 0, status.Fail(status.CodeArgsInvalid, "%v", err)
	}
	if err != nil {
		return 1, err
	}

	return 0, nil
}
