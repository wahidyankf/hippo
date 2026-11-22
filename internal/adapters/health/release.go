package health

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

// output runs one command through the runner port and returns its trimmed standard output. A command that fails
// yields an empty string, so a missing tool reads as no data.
func output(ctx context.Context, runner system.ProcessRunner, command string, arguments ...string) string {
	value, err := runner.CaptureOutput(ctx, command, arguments...)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(value))
}

func serviceRSSBytes(ctx context.Context, runner system.ProcessRunner, ports []int) int64 {
	pids := map[string]bool{}

	for _, port := range ports {
		listeners := output(ctx, runner, "lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-t")
		for pid := range strings.FieldsSeq(listeners) {
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
	for field := range strings.FieldsSeq(output(ctx, runner, "ps", "-o", "rss=", "-p", strings.Join(list, ","))) {
		value, _ := strconv.ParseInt(field, 10, 64)
		total += value * 1024
	}

	return total
}

func probeHTTP(ctx context.Context, runner system.ProcessRunner, target string) (int, float64) {
	return probeHTTPWithRedirects(ctx, runner, target, false)
}

func probeRoutedHTTP(ctx context.Context, runner system.ProcessRunner, target string) (int, float64) {
	return probeHTTPWithRedirects(ctx, runner, target, true)
}

func probeHTTPWithRedirects(ctx context.Context, runner system.ProcessRunner, target string, followRedirects bool) (int, float64) {
	arguments := HTTPProbeArguments(target, followRedirects)
	value := output(ctx, runner, "curl", arguments...)
	fields := strings.Fields(value)

	if len(fields) != 2 {
		return 0, 3000.0
	}
	status, _ := strconv.Atoi(fields[0])
	seconds, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 3000.0
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

// oneMinuteLoad reads the first field of /proc/loadavg. When that file is missing or its first field is not a
// number, it asks sysctl for vm.loadavg instead, and returns zero when sysctl gives no number either.
func oneMinuteLoad(ctx context.Context, host system.System) float64 {
	if data, err := host.Files.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if value, parseError := strconv.ParseFloat(fields[0], 64); parseError == nil {
				return value
			}
		}
	}

	fields := strings.Fields(strings.Trim(output(ctx, host.Runner, "sysctl", "-n", "vm.loadavg"), "{} "))
	if len(fields) == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}

	return value
}

// Probe implements the release health capabilities over the host ports it is given.
type Probe struct {
	Host system.System
}

// ServiceRSS observes listeners at the selected ports.
func (probe Probe) ServiceRSS(ctx context.Context, ports []int) int64 {
	return serviceRSSBytes(ctx, probe.Host.Runner, ports)
}

// Local observes the health endpoint without following redirects.
func (probe Probe) Local(ctx context.Context, target string) (int, float64) {
	return probeHTTP(ctx, probe.Host.Runner, target)
}

// Routed observes a routed journey through bounded redirects.
func (probe Probe) Routed(ctx context.Context, target string) (int, float64) {
	return probeRoutedHTTP(ctx, probe.Host.Runner, target)
}

// LoadAverage observes the host one-minute load.
func (probe Probe) LoadAverage(ctx context.Context) float64 { return oneMinuteLoad(ctx, probe.Host) }
