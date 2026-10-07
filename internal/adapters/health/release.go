package health

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

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

// Probe implements the release health capabilities.
type Probe struct{}

// ServiceRSS observes listeners at the selected ports.
func (Probe) ServiceRSS(ctx context.Context, ports []int) int64 { return serviceRSSBytes(ctx, ports) }

// Local observes the health endpoint without following redirects.
func (Probe) Local(ctx context.Context, target string) (int, float64) { return probeHTTP(ctx, target) }

// Routed observes a routed journey through bounded redirects.
func (Probe) Routed(ctx context.Context, target string) (int, float64) {
	return probeRoutedHTTP(ctx, target)
}

// LoadAverage observes the host one-minute load.
func (Probe) LoadAverage(ctx context.Context) float64 { return oneMinuteLoad(ctx) }
