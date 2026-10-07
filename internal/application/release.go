package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

// ReleaseLimits bounds one release evidence stream.
type ReleaseLimits = evidence.Limits

// HealthProbe supplies host and service observations for release monitoring.
type HealthProbe interface {
	ServiceRSS(ctx context.Context, ports []int) int64
	Local(ctx context.Context, target string) (int, float64)
	Routed(ctx context.Context, target string) (int, float64)
	LoadAverage(ctx context.Context) float64
}

// ReleaseOutput owns one raw release evidence destination.
type ReleaseOutput struct {
	Append func(evidence.ReleaseSample) error
	Close  func() error
}

// ReleaseEvidence supplies storage primitives; the application owns their order.
type ReleaseEvidence interface {
	OpenReleaseOutput(config MonitorConfig) (ReleaseOutput, string, error)
	ReadReleaseSummary(path string) ([]byte, error)
	WriteReleaseSummary(path string, destination io.Writer, summary policy.ReleaseSummary) error
	CleanupRelease(root string, now time.Time, active ...string) error
}

// ReleaseTicker provides cancellable release sampling ticks.
type ReleaseTicker struct {
	Ticks <-chan time.Time
	Stop  func()
}

// ReleaseClock supplies timestamps and bounded sampling ticks.
type ReleaseClock interface {
	Wait(ctx context.Context, duration time.Duration) error
	Now() time.Time
	Ticker(interval time.Duration) ReleaseTicker
}

// ReleaseServices wires the capabilities used by release use cases.
type ReleaseServices struct {
	Configuration ConfigurationProvider
	Collector     policy.Collector
	Environment   map[string]string
	Clock         ReleaseClock
	Health        HealthProbe
	Evidence      ReleaseEvidence
}

// AssessFile reads and assesses one completed release summary.
func (services ReleaseServices) AssessFile(path string) (policy.ReleaseSummary, error) {
	if services.Evidence == nil {
		return policy.ReleaseSummary{}, errors.New("release evidence repository is required")
	}
	data, err := services.Evidence.ReadReleaseSummary(path)
	if err != nil {
		return policy.ReleaseSummary{}, err
	}
	return Assess(bytes.NewReader(data))
}

// The ways a release stability check turns a release down. Each is a verdict
// on the host rather than a fault in the check, so the command line can name
// the limit that stopped the release; any other error is the check failing.
var (
	// ErrMemoryHeadroom is memory pressure that leaves no safe release headroom.
	ErrMemoryHeadroom = errors.New("memory pressure does not leave safe release headroom")
	// ErrDiskReserve is free disk below the release reserve.
	ErrDiskReserve = errors.New("release disk reserve is unavailable")
	// ErrCPUHeadroom is CPU use that never settled inside the release budget.
	ErrCPUHeadroom = errors.New("CPU use does not leave release and safety headroom")
	// ErrHeadroomExhausted is a well-formed release summary whose overlap left
	// the release envelope. Every other Assess error means the summary itself
	// could not be used.
	ErrHeadroomExhausted = evidence.ErrReleaseHeadroomExhausted
)

// Check requires consecutive CPU samples plus release memory and disk reserves.
func Check(ctx context.Context, collector policy.Collector, diskPath string, pause func(time.Duration)) error {
	return CheckWithPolicy(ctx, collector, diskPath, pause, policy.DefaultPolicy())
}

// CheckWithPolicy requires consecutive samples inside one resolved strict profile.
func CheckWithPolicy(ctx context.Context, collector policy.Collector, diskPath string, pause func(time.Duration), resourcePolicy policy.Policy) error {
	return checkWithPolicyWait(ctx, collector, diskPath, resourcePolicy, func(waitContext context.Context, duration time.Duration) error {
		return waitForContext(waitContext, duration, pause)
	})
}

func checkWithPolicyWait(ctx context.Context, collector policy.Collector, diskPath string, resourcePolicy policy.Policy, wait func(context.Context, time.Duration) error) error {
	if collector == nil {
		return errors.New("host collector is required")
	}

	var previous policy.CPUState
	consecutive := 0

	for attempt := range 31 {
		if err := ctx.Err(); err != nil {
			return err
		}

		reading, err := collector.Collect(ctx, previous, diskPath)
		if err != nil {
			return err
		}

		previous = reading.CPUState

		if policy.MemoryState(reading.Sample, resourcePolicy) != policy.StateNormal {
			return ErrMemoryHeadroom
		}
		if reading.Sample.DiskFreeBytes == nil || *reading.Sample.DiskFreeBytes < resourcePolicy.DiskWarningBytes {
			return ErrDiskReserve
		}

		if policy.CPUAdmissionReady(reading.Sample, resourcePolicy) {
			consecutive++
		} else {
			consecutive = 0
		}

		if consecutive >= 3 {
			return nil
		}

		if attempt < 30 {
			if err := wait(ctx, 500*time.Millisecond); err != nil {
				return err
			}
		}
	}

	return ErrCPUHeadroom
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

// Assess validates one completed release summary from a stream.
func Assess(reader io.Reader) (policy.ReleaseSummary, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return policy.ReleaseSummary{}, err
	}

	var summary policy.ReleaseSummary
	if err = json.Unmarshal(data, &summary); err != nil {
		return policy.ReleaseSummary{}, err
	}

	if err := evidence.ValidateReleaseSummary(summary); err != nil {
		return summary, err
	}

	return summary, nil
}

// MonitorConfig describes one bounded release-monitoring session.
type MonitorConfig struct {
	OutputPath, SummaryPath, DeploymentRoot string
	RawOutput, SummaryOutput                io.Writer
	HealthURL, RoutedOrigin                 string
	ServicePorts                            []int
	Collector                               policy.Collector
	Interval                                time.Duration
	EvidenceLimits                          ReleaseLimits
	Now                                     func() time.Time
	ServiceRSS                              func(context.Context) int64
	Health                                  func(context.Context) (int, float64)
	RoutedHealth                            func(context.Context) (int, float64)
	LoadAverage                             func(context.Context) float64
}

func localHealth(probe HealthProbe, target string) (func(context.Context) (int, float64), error) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, monitorInputError("HTTP(S) health URL is required for release monitoring")
	}

	return func(ctx context.Context) (int, float64) { return probe.Local(ctx, target) }, nil
}

func routedHealth(probe HealthProbe, origin string) (func(context.Context) (int, float64), error) {
	parsed, err := url.Parse(origin)
	if err != nil ||
		parsed.Scheme != "https" ||
		parsed.Host == "" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" ||
		parsed.Path != "" && parsed.Path != "/" {
		return nil, monitorInputError("bare HTTPS routed origin is required for release monitoring")
	}

	target := strings.TrimSuffix(origin, "/") + "/"

	return func(ctx context.Context) (int, float64) { return probe.Routed(ctx, target) }, nil
}

// ErrMonitorInput marks a monitor configuration the caller gave: a missing
// destination, deployment root, health URL, or routed origin. The command line
// reports it as a usage mistake rather than a supervision failure.
var ErrMonitorInput = errors.New("release monitor input")

// monitorInputError carries the caller-facing sentence and matches
// ErrMonitorInput, so the sentence is not prefixed with the sentinel's text.
type monitorInputError string

func (failure monitorInputError) Error() string { return string(failure) }

func (monitorInputError) Is(target error) bool { return target == ErrMonitorInput }

func (services ReleaseServices) normalizeMonitorConfig(config MonitorConfig) (MonitorConfig, error) {
	if err := validateMonitorDestinations(config); err != nil {
		return MonitorConfig{}, err
	}
	if config.Collector == nil {
		return MonitorConfig{}, errors.New("host collector is required")
	}
	if config.Interval < 0 {
		return MonitorConfig{}, errors.New("release monitor interval must be nonnegative")
	}
	needsHealthProbe := config.ServiceRSS == nil || config.Health == nil || config.RoutedHealth == nil || config.LoadAverage == nil

	if config.ServiceRSS == nil {
		config.ServiceRSS = func(ctx context.Context) int64 { return services.Health.ServiceRSS(ctx, config.ServicePorts) }
	}
	if config.Health == nil {
		probe, err := localHealth(services.Health, config.HealthURL)
		if err != nil {
			return MonitorConfig{}, err
		}
		config.Health = probe
	}
	if config.RoutedHealth == nil {
		probe, err := routedHealth(services.Health, config.RoutedOrigin)
		if err != nil {
			return MonitorConfig{}, err
		}
		config.RoutedHealth = probe
	}

	if services.Health == nil && needsHealthProbe {
		return MonitorConfig{}, errors.New("release health probe is required")
	}
	if config.LoadAverage == nil {
		config.LoadAverage = services.Health.LoadAverage
	}
	if services.Clock == nil {
		return MonitorConfig{}, errors.New("release clock is required")
	}
	if config.Interval == 0 {
		config.Interval = time.Second
	}
	if config.Now == nil {
		config.Now = services.Clock.Now
	}

	return config, nil
}

func captureReleaseSamples(
	ctx context.Context,
	config MonitorConfig,
	clock ReleaseClock,
	output ReleaseOutput,
	accumulator *evidence.ReleaseAccumulator,
) error {
	var previous policy.CPUState

	sample := func() error {
		reading, collectError := config.Collector.Collect(ctx, previous, config.DeploymentRoot)
		if collectError != nil {
			return collectError
		}

		previous = reading.CPUState
		status, latency := config.Health(ctx)
		routedStatus, routedLatency := config.RoutedHealth(ctx)
		value := evidence.ReleaseSample{
			Sample:                 reading.Sample,
			OneMinuteLoad:          config.LoadAverage(ctx),
			ServiceRSSBytes:        config.ServiceRSS(ctx),
			HealthStatus:           status,
			HealthLatencyMs:        latency,
			RoutedJourneyStatus:    routedStatus,
			RoutedJourneyLatencyMs: routedLatency,
		}

		if err := output.Append(value); err != nil {
			return err
		}

		accumulator.Add(value)

		return nil
	}

	if err := sample(); err != nil {
		return err
	}

	ticker := clock.Ticker(config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.Ticks:
			if ctx.Err() != nil {
				return nil
			}
			if err := sample(); err != nil {
				if ctx.Err() != nil {
					return nil
				}

				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// RunMonitor records release overlap samples until its context is cancelled.
func (services ReleaseServices) RunMonitor(ctx context.Context, config MonitorConfig) error {
	// Inputs are validated before the deadline is consulted: a missing health URL
	// is the caller's answer on any host, and a short or already passed deadline
	// must not replace it with a cancellation.
	var err error
	config, err = services.normalizeMonitorConfig(config)
	if err != nil {
		return err
	}

	if services.Evidence == nil {
		return errors.New("release evidence repository is required")
	}

	if err = ctx.Err(); err != nil {
		return err
	}

	output, root, err := services.Evidence.OpenReleaseOutput(config)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = output.Close()
		}
	}()

	accumulator := evidence.NewReleaseAccumulator()
	if err := captureReleaseSamples(ctx, config, services.Clock, output, accumulator); err != nil {
		return err
	}

	if err := output.Close(); err != nil {
		return err
	}
	closed = true

	if err := services.Evidence.WriteReleaseSummary(config.SummaryPath, config.SummaryOutput, accumulator.Result()); err != nil {
		return err
	}

	if root != "" {
		return services.Evidence.CleanupRelease(root, config.Now())
	}

	return nil
}

func validateMonitorDestinations(config MonitorConfig) error {
	hasRawOutput := config.OutputPath != "" || config.RawOutput != nil
	hasSummaryOutput := config.SummaryPath != "" || config.SummaryOutput != nil
	if !hasRawOutput || !hasSummaryOutput || config.DeploymentRoot == "" {
		return monitorInputError("output, summary, and deployment root are required")
	}

	hasDuplicateRawOutput := config.OutputPath != "" && config.RawOutput != nil
	hasDuplicateSummaryOutput := config.SummaryPath != "" && config.SummaryOutput != nil
	if hasDuplicateRawOutput || hasDuplicateSummaryOutput {
		return monitorInputError("output and summary each require exactly one destination")
	}

	return nil
}
