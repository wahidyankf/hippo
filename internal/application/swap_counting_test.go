package application

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/wahidyankf/hippo/internal/policy"
)

// fakeSwapCollector returns one fixed reading or error, and counts its calls.
type fakeSwapCollector struct {
	reading policy.Reading
	err     error
	calls   int
}

func (fake *fakeSwapCollector) Collect(_ context.Context, _ policy.CPUState, _ string) (policy.Reading, error) {
	fake.calls++

	return fake.reading, fake.err
}

func swapCountingReading() policy.Reading {
	allowance := policy.Sample{
		EffectiveMemoryLimitBytes: 4 * policy.GiB,
		PhysicalMemoryBytes:       4 * policy.GiB,
		Platform:                  "linux",
		SwapState:                 "idle",
		SwapTotalBytes:            new(8 * policy.GiB),
		SwapUsedBytes:             new(int64(0)),
	}

	return policy.Reading{Sample: allowance}
}

func TestWithSwapCountingLeavesTheCollectorAloneWhenOff(t *testing.T) {
	inner := &fakeSwapCollector{reading: swapCountingReading()}
	if WithSwapCounting(inner, false) != policy.Collector(inner) {
		t.Fatal("counting off wrapped the collector instead of returning it unchanged")
	}
}

func TestWithSwapCountingAttachesTheAllowanceToEachSample(t *testing.T) {
	inner := &fakeSwapCollector{reading: swapCountingReading()}
	reading, err := WithSwapCounting(inner, true).Collect(context.Background(), nil, ".")
	if err != nil {
		t.Fatal(err)
	}
	if inner.calls != 1 {
		t.Fatalf("inner collector ran %d times, want once", inner.calls)
	}
	if reading.Sample.SwapAllowanceBytes == nil || *reading.Sample.SwapAllowanceBytes != 4*policy.GiB {
		t.Fatalf("allowance = %v, want 4 GiB", reading.Sample.SwapAllowanceBytes)
	}
}

func TestWithSwapCountingPassesCollectionErrorsThrough(t *testing.T) {
	failure := errors.New("host unreadable")
	inner := &fakeSwapCollector{err: failure}
	if _, err := WithSwapCounting(inner, true).Collect(context.Background(), nil, "."); !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the collector's own error", err)
	}
}

func TestSwapAllowanceNoticeIsWrittenOncePerRun(t *testing.T) {
	var stderr bytes.Buffer
	var notice swapAllowanceNotice
	within := policy.Assessment{State: policy.StateNormal, Reason: "normal"}
	over := policy.Assessment{State: policy.StateNormal, Reason: "normal", Advisories: []string{"swap-over-allowance"}}

	notice.observe(&stderr, within)
	if stderr.Len() != 0 {
		t.Fatalf("within the allowance wrote %q", stderr.String())
	}
	notice.observe(&stderr, over)
	notice.observe(&stderr, over)
	if got := stderr.String(); got != swapAllowanceWarning+"\n" {
		t.Fatalf("stderr = %q, want the warning once", got)
	}
}

func TestObservationSamplesKeepTheSwapAllowance(t *testing.T) {
	sample := swapCountingReading().Sample
	sample.SwapAllowanceBytes = new(4 * policy.GiB)
	clone := cloneObservationSample(sample)
	if clone.SwapAllowanceBytes == nil || *clone.SwapAllowanceBytes != 4*policy.GiB {
		t.Fatalf("clone allowance = %v, want 4 GiB", clone.SwapAllowanceBytes)
	}
	if clone.SwapAllowanceBytes == sample.SwapAllowanceBytes {
		t.Fatal("clone shares the allowance pointer with its source")
	}
}
