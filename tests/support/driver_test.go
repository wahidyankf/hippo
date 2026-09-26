package support //nolint:testpackage // The readiness seam is private to the behaviour driver it synchronises.

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
)

func TestSequenceCollectorRunsBeforeFailureHook(t *testing.T) {
	var events []string
	collector := &sequenceCollector{
		failureFrom: 1,
		samples:     []policy.Sample{healthySample(time.Now())},
		beforeFailure: func() error {
			events = append(events, "ready")

			return nil
		},
	}
	if _, err := collector.Collect(context.Background(), policy.CPUState{}, ""); err != nil {
		t.Fatalf("first sample: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("readiness hook ran before the failure boundary: %v", events)
	}

	_, err := collector.Collect(context.Background(), policy.CPUState{}, "")
	if err == nil || err.Error() != "injected host evidence failure" {
		t.Fatalf("failure boundary returned %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("readiness hook did not run once before the injected failure: %v", events)
	}
}

func TestSequenceCollectorReturnsReadinessErrorWithinBound(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "child.pid")
	collector := &sequenceCollector{
		failureFrom:   1,
		samples:       []policy.Sample{healthySample(time.Now())},
		beforeFailure: childPIDReadiness(missing, 10*time.Millisecond),
	}
	if _, err := collector.Collect(context.Background(), policy.CPUState{}, ""); err != nil {
		t.Fatalf("first sample: %v", err)
	}

	_, err := collector.Collect(context.Background(), policy.CPUState{}, "")
	if !errors.Is(err, errChildPIDNotReady) {
		t.Fatalf("missing readiness returned %v instead of the readiness error", err)
	}
}
