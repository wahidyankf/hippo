package application

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/wahidyankf/hippo/internal/policy"
)

// swapAllowanceWarning is the one stderr line a run writes the first time swap in
// use exceeds its allowance. The state does not change, so nothing is shed for it.
const swapAllowanceWarning = "HIPPO warning: swap use exceeds its allowance; not shedding for swap alone."

// SwapCountingCollector attaches the swap allowance to every sample its collector
// reads when counting is on, and passes the reading through unchanged when it is off.
type SwapCountingCollector struct {
	inner   policy.Collector
	enabled bool
}

// Collect reads through the wrapped collector and, when counting is on, attaches
// the swap allowance to the sample it returns.
func (collector SwapCountingCollector) Collect(ctx context.Context, previous policy.CPUState, diskPath string) (policy.Reading, error) {
	reading, err := collector.inner.Collect(ctx, previous, diskPath)
	if err != nil || !collector.enabled {
		return reading, err
	}
	reading.Sample = policy.WithSwapAllowance(reading.Sample, true)

	return reading, nil
}

// WithSwapCounting wraps collector so that, when coordination.countSwap is on, every
// sample carries the allowance the policy counts. When the setting is off the wrapper
// reads exactly what collector reads, so a host without the setting samples as before.
func WithSwapCounting(collector policy.Collector, enabled bool) SwapCountingCollector {
	return SwapCountingCollector{inner: collector, enabled: enabled}
}

// swapAllowanceNotice writes swapAllowanceWarning once per run, the first time an
// assessment reports swap use beyond the allowance.
type swapAllowanceNotice struct {
	written bool
}

func (notice *swapAllowanceNotice) observe(stderr io.Writer, assessment policy.Assessment) {
	if notice.written || !slices.Contains(assessment.Advisories, policy.SwapOverAllowanceAdvisory) {
		return
	}
	notice.written = true
	_, _ = fmt.Fprintln(stderr, swapAllowanceWarning)
}
