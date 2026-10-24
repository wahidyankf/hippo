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

// swapCountingCollector attaches the swap allowance to every sample its collector reads.
type swapCountingCollector struct {
	inner policy.Collector
}

func (collector swapCountingCollector) Collect(ctx context.Context, previous policy.CPUState, diskPath string) (policy.Reading, error) {
	reading, err := collector.inner.Collect(ctx, previous, diskPath)
	if err != nil {
		return reading, err
	}
	reading.Sample = policy.WithSwapAllowance(reading.Sample, true)

	return reading, nil
}

// WithSwapCounting returns collector unchanged when coordination.countSwap is off,
// so a host without the setting samples exactly as it did before. When the setting
// is on, every sample carries the allowance the policy counts.
func WithSwapCounting(collector policy.Collector, enabled bool) policy.Collector {
	if !enabled {
		return collector
	}

	return swapCountingCollector{inner: collector}
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
