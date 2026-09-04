package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wahidyankf/resource-guard/internal/evidence"
	"github.com/wahidyankf/resource-guard/internal/policy"
)

const maximumProbeLatencyMs = 3000.0

// Check requires consecutive CPU samples plus release memory and disk reserves.
func Check(ctx context.Context, collector policy.Collector, diskPath string, pause func(time.Duration)) error {
	return CheckWithPolicy(ctx, collector, diskPath, pause, policy.DefaultPolicy())
}

// CheckWithPolicy requires consecutive samples inside one resolved strict profile.
func CheckWithPolicy(ctx context.Context, collector policy.Collector, diskPath string, pause func(time.Duration), resourcePolicy policy.Policy) error {
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
			return errors.New("memory pressure does not leave safe release headroom")
		}
		if reading.Sample.DiskFreeBytes == nil || *reading.Sample.DiskFreeBytes < resourcePolicy.DiskWarningBytes {
			return errors.New("release disk reserve is unavailable")
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
			if err := waitForContext(ctx, 500*time.Millisecond, pause); err != nil {
				return err
			}
		}
	}

	return errors.New("CPU use does not leave release and safety headroom")
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

// AssessFile validates one completed release summary.
func AssessFile(path string) (policy.ReleaseSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return policy.ReleaseSummary{}, err
	}
	defer func() { _ = file.Close() }()

	return Assess(file)
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

	if summary.SchemaVersion < 2 || summary.SchemaVersion > 5 || summary.SampleCount <= 0 || summary.AvailableParallelism <= 0 {
		return summary, errors.New("resource evidence summary is invalid")
	}
	if !policy.ReleaseHeadroomAvailable(summary) {
		return summary, errors.New("release overlap exhausted resource or routed responsiveness headroom")
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
	EvidenceLimits                          evidence.Limits
	Now                                     func() time.Time
	ServiceRSS                              func(context.Context) int64
	Health                                  func(context.Context) (int, float64)
	RoutedHealth                            func(context.Context) (int, float64)
	LoadAverage                             func(context.Context) float64
}

type releaseSample struct {
	policy.Sample

	OneMinuteLoad          float64 `json:"oneMinuteLoad"`
	ServiceRSSBytes        int64   `json:"serviceRssBytes"`
	HealthStatus           int     `json:"healthStatus"`
	HealthLatencyMs        float64 `json:"healthLatencyMs"`
	RoutedJourneyStatus    int     `json:"routedJourneyStatus"`
	RoutedJourneyLatencyMs float64 `json:"routedJourneyLatencyMs"`
}

func output(ctx context.Context, command string, arguments ...string) string {
	value, err := exec.CommandContext(ctx, command, arguments...).Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(value))
}

func serviceRSSBytes(ctx context.Context, ports []int) int64 {
	pids := map[string]bool{}

	for _, port := range ports {
		for pid := range strings.FieldsSeq(output(ctx, "lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-t")) {
			pids[pid] = true
		}
	}

	if len(pids) == 0 {
		return 0
	}

	list := make([]string, 0, len(pids))
	for pid := range pids {
		list = append(list, pid)
	}

	var total int64
	for field := range strings.FieldsSeq(output(ctx, "ps", "-o", "rss=", "-p", strings.Join(list, ","))) {
		value, _ := strconv.ParseInt(field, 10, 64)
		total += value * 1024
	}

	return total
}

func probeHTTP(ctx context.Context, target string) (int, float64) {
	return probeHTTPWithRedirects(ctx, target, false)
}

func probeRoutedHTTP(ctx context.Context, target string) (int, float64) {
	return probeHTTPWithRedirects(ctx, target, true)
}

func probeHTTPWithRedirects(ctx context.Context, target string, followRedirects bool) (int, float64) {
	arguments := HTTPProbeArguments(target, followRedirects)
	value := output(ctx, "curl", arguments...)
	fields := strings.Fields(value)

	if len(fields) != 2 {
		return 0, maximumProbeLatencyMs
	}
	status, _ := strconv.Atoi(fields[0])
	seconds, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, maximumProbeLatencyMs
	}

	return status, seconds * 1000
}

// HTTPProbeArguments returns the bounded curl arguments for one health or routed probe.
func HTTPProbeArguments(target string, followRedirects bool) []string {
	arguments := []string{"-sS"}

	if followRedirects {
		arguments = append(arguments, "--location", "--max-redirs", "3")
	}

	arguments = append(arguments, "--max-time", "3", "-o", "/dev/null", "-w", "%{http_code} %{time_total}", target)

	return arguments
}

func localHealth(target string) (func(context.Context) (int, float64), error) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("HTTP(S) health URL is required for release monitoring")
	}

	return func(ctx context.Context) (int, float64) { return probeHTTP(ctx, target) }, nil
}

func routedHealth(origin string) (func(context.Context) (int, float64), error) {
	parsed, err := url.Parse(origin)
	if err != nil ||
		parsed.Scheme != "https" ||
		parsed.Host == "" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" ||
		parsed.Path != "" && parsed.Path != "/" {
		return nil, errors.New("bare HTTPS routed origin is required for release monitoring")
	}

	target := strings.TrimSuffix(origin, "/") + "/"

	return func(ctx context.Context) (int, float64) { return probeRoutedHTTP(ctx, target) }, nil
}

func oneMinuteLoad(ctx context.Context) float64 {
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if value, parseError := strconv.ParseFloat(fields[0], 64); parseError == nil {
				return value
			}
		}
	}

	fields := strings.Fields(strings.Trim(output(ctx, "sysctl", "-n", "vm.loadavg"), "{} "))
	if len(fields) == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}

	return value
}

func normalizeMonitorConfig(config MonitorConfig) (MonitorConfig, error) {
	hasRawOutput := config.OutputPath != "" || config.RawOutput != nil
	hasSummaryOutput := config.SummaryPath != "" || config.SummaryOutput != nil
	if !hasRawOutput || !hasSummaryOutput || config.DeploymentRoot == "" {
		return MonitorConfig{}, errors.New("output, summary, and deployment root are required")
	}

	hasDuplicateRawOutput := config.OutputPath != "" && config.RawOutput != nil
	hasDuplicateSummaryOutput := config.SummaryPath != "" && config.SummaryOutput != nil
	if hasDuplicateRawOutput || hasDuplicateSummaryOutput {
		return MonitorConfig{}, errors.New("output and summary each require exactly one destination")
	}
	if config.Collector == nil {
		return MonitorConfig{}, errors.New("host collector is required")
	}
	if config.Interval < 0 {
		return MonitorConfig{}, errors.New("release monitor interval must be nonnegative")
	}

	if config.ServiceRSS == nil {
		config.ServiceRSS = func(ctx context.Context) int64 { return serviceRSSBytes(ctx, config.ServicePorts) }
	}
	if config.Health == nil {
		probe, err := localHealth(config.HealthURL)
		if err != nil {
			return MonitorConfig{}, err
		}
		config.Health = probe
	}
	if config.RoutedHealth == nil {
		probe, err := routedHealth(config.RoutedOrigin)
		if err != nil {
			return MonitorConfig{}, err
		}
		config.RoutedHealth = probe
	}

	if config.LoadAverage == nil {
		config.LoadAverage = oneMinuteLoad
	}
	if config.Interval == 0 {
		config.Interval = time.Second
	}
	if config.Now == nil {
		config.Now = time.Now
	}

	return config, nil
}

type monitorOutput struct {
	file   *evidence.Writer
	stream io.Writer
}

func (output *monitorOutput) AppendJSON(value any) error {
	if output.file != nil {
		return output.file.AppendJSON(value)
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')
	written, err := output.stream.Write(encoded)
	if err == nil && written != len(encoded) {
		return io.ErrShortWrite
	}

	return err
}

func (output *monitorOutput) Close() error {
	if output.file != nil {
		return output.file.Close()
	}

	return nil
}

func openSampleOutput(config MonitorConfig) (*monitorOutput, string, error) {
	if config.RawOutput != nil {
		return &monitorOutput{stream: config.RawOutput}, "", nil
	}

	root := filepath.Dir(config.OutputPath)
	if err := evidence.Cleanup(root, config.Now(), config.OutputPath, config.SummaryPath); err != nil {
		return nil, "", err
	}

	output, err := evidence.NewWriter(config.OutputPath, config.EvidenceLimits)

	return &monitorOutput{file: output}, root, err
}

func captureReleaseSamples(
	ctx context.Context,
	config MonitorConfig,
	output *monitorOutput,
	accumulator *releaseAccumulator,
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
		value := releaseSample{
			Sample:                 reading.Sample,
			OneMinuteLoad:          config.LoadAverage(ctx),
			ServiceRSSBytes:        config.ServiceRSS(ctx),
			HealthStatus:           status,
			HealthLatencyMs:        latency,
			RoutedJourneyStatus:    routedStatus,
			RoutedJourneyLatencyMs: routedLatency,
		}

		if err := output.AppendJSON(value); err != nil {
			return err
		}

		accumulator.add(value)

		return nil
	}

	if err := sample(); err != nil {
		return err
	}

	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
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
func RunMonitor(ctx context.Context, config MonitorConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var err error
	config, err = normalizeMonitorConfig(config)
	if err != nil {
		return err
	}

	output, root, err := openSampleOutput(config)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = output.Close()
		}
	}()

	accumulator := newReleaseAccumulator()
	if err := captureReleaseSamples(ctx, config, output, accumulator); err != nil {
		return err
	}

	if err := output.Close(); err != nil {
		return err
	}
	closed = true

	if err := writeSummary(config.SummaryPath, config.SummaryOutput, accumulator.result()); err != nil {
		return err
	}

	if root != "" {
		return evidence.Cleanup(root, config.Now())
	}

	return nil
}

type releaseAccumulator struct {
	summary       policy.ReleaseSummary
	first         *policy.Sample
	last          *policy.Sample
	cpu           *evidence.Histogram
	healthLatency *evidence.Histogram
	routedLatency *evidence.Histogram
}

func newReleaseAccumulator() *releaseAccumulator {
	return &releaseAccumulator{
		summary: policy.ReleaseSummary{
			SchemaVersion:                          5,
			CompressorAvailableAll:                 true,
			MemoryPressureLevelMax:                 0,
			AvailableNonCompressedEstimateMinBytes: math.MaxInt64,
			DiskFreeMinBytes:                       math.MaxInt64,
			SwapFreeMinBytes:                       math.MaxInt64,
		},
		cpu:           evidence.NewHistogram(100, 0.01),
		healthLatency: evidence.NewHistogram(maximumProbeLatencyMs, 0.1),
		routedLatency: evidence.NewHistogram(maximumProbeLatencyMs, 0.1),
	}
}

func (accumulator *releaseAccumulator) add(sample releaseSample) {
	if accumulator.first == nil {
		first := sample.Sample
		accumulator.first = &first
		accumulator.summary.Platform = sample.Platform
		accumulator.summary.Capabilities = append([]string(nil), sample.Capabilities...)
		accumulator.summary.AvailableParallelism = sample.AvailableParallelism
	}

	last := sample.Sample
	accumulator.last = &last
	accumulator.summary.SampleCount++

	available := sample.AvailableMemoryBytes
	if available == nil {
		available = sample.AvailableNonCompressedEstimateBytes
	}

	if available != nil {
		accumulator.summary.AvailableNonCompressedEstimateMinBytes = min(accumulator.summary.AvailableNonCompressedEstimateMinBytes, *available)
	}
	if sample.MemoryPressureLevel != nil {
		accumulator.summary.MemoryPressureLevelMax = max(accumulator.summary.MemoryPressureLevelMax, *sample.MemoryPressureLevel)
	}
	if sample.CompressorAvailable == nil || !*sample.CompressorAvailable {
		accumulator.summary.CompressorAvailableAll = false
	}
	if sample.CompressorPayloadBytes != nil {
		accumulator.summary.CompressorPayloadPeakBytes = max(accumulator.summary.CompressorPayloadPeakBytes, *sample.CompressorPayloadBytes)
	}
	if sample.CPUUtilizationPercent != nil {
		accumulator.cpu.Add(*sample.CPUUtilizationPercent)
	}
	if sample.DiskFreeBytes != nil {
		accumulator.summary.DiskFreeMinBytes = min(accumulator.summary.DiskFreeMinBytes, *sample.DiskFreeBytes)
	}
	if sample.SwapFreeBytes != nil {
		accumulator.summary.SwapFreeMinBytes = min(accumulator.summary.SwapFreeMinBytes, *sample.SwapFreeBytes)
	}

	accumulator.summary.ServiceRSSPeakBytes = max(accumulator.summary.ServiceRSSPeakBytes, sample.ServiceRSSBytes)
	accumulator.summary.RoutedJourneyLatencyMaxMs = max(accumulator.summary.RoutedJourneyLatencyMaxMs, sample.RoutedJourneyLatencyMs)
	accumulator.healthLatency.Add(min(sample.HealthLatencyMs, maximumProbeLatencyMs))
	accumulator.routedLatency.Add(min(sample.RoutedJourneyLatencyMs, maximumProbeLatencyMs))

	if sample.HealthStatus != 200 {
		accumulator.summary.HealthFailures++
	}
	if sample.RoutedJourneyStatus != 200 {
		accumulator.summary.RoutedJourneyFailures++
	}
}

func (accumulator *releaseAccumulator) result() policy.ReleaseSummary {
	summary := accumulator.summary
	if accumulator.first == nil || accumulator.last == nil {
		return summary
	}

	summary.PhysicalMemoryBytes = accumulator.first.PhysicalMemoryBytes
	if accumulator.first.SwapIns != nil && accumulator.last.SwapIns != nil {
		summary.SwapInsDelta = max(0, *accumulator.last.SwapIns-*accumulator.first.SwapIns)
	}
	if accumulator.first.SwapOuts != nil && accumulator.last.SwapOuts != nil {
		summary.SwapOutsDelta = max(0, *accumulator.last.SwapOuts-*accumulator.first.SwapOuts)
	}

	if value, ok := accumulator.cpu.Quantile(0.95); ok {
		summary.CPUUtilizationP95Percent = value
	}
	if value, ok := accumulator.healthLatency.Quantile(0.95); ok {
		summary.HealthLatencyP95Ms = value
	}
	if value, ok := accumulator.routedLatency.Quantile(0.95); ok {
		summary.RoutedJourneyLatencyP95Ms = value
	}

	return summary
}

func writeSummary(path string, destination io.Writer, summary policy.ReleaseSummary) error {
	if summary.SampleCount == 0 {
		return errors.New("release monitor has no samples")
	}

	if destination != nil {
		encoded, err := json.Marshal(summary)
		if err != nil {
			return err
		}

		written, err := destination.Write(append(encoded, '\n'))
		if err == nil && written != len(encoded)+1 {
			return io.ErrShortWrite
		}

		return err
	}

	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = file.Write(append(encoded, '\n'))

	return err
}
