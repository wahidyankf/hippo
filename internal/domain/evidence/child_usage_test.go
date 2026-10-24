package evidence_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

func completeWithChildUsage(t *testing.T, cpu, wall time.Duration, measured bool) evidence.RunSummary {
	t.Helper()
	accumulator := evidence.NewRunAccumulator("run")
	accumulator.SetChildUsage(cpu, wall, measured)

	return accumulator.Complete(policy.TaskEphemeral, evidence.OutcomePassed, 0)
}

func TestRunAccumulatorRoundsChildUsageToMilliseconds(t *testing.T) {
	summary := completeWithChildUsage(t, 1234*time.Millisecond+600*time.Microsecond, 2*time.Second+400*time.Microsecond, true)
	if summary.ChildCPUSeconds == nil || *summary.ChildCPUSeconds != 1.235 {
		t.Fatalf("child CPU = %v, want 1.235 seconds rounded to milliseconds", summary.ChildCPUSeconds)
	}
	if summary.ChildWallSeconds == nil || *summary.ChildWallSeconds != 2 {
		t.Fatalf("child wall = %v, want 2 seconds rounded to milliseconds", summary.ChildWallSeconds)
	}
}

func TestRunAccumulatorOmitsUnmeasuredChildUsage(t *testing.T) {
	summary := completeWithChildUsage(t, time.Second, 2*time.Second, false)
	if summary.ChildCPUSeconds != nil || summary.ChildWallSeconds != nil {
		t.Fatalf("unmeasured usage was recorded: cpu=%v wall=%v", summary.ChildCPUSeconds, summary.ChildWallSeconds)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"childCpuSeconds", "childWallSeconds"} {
		if strings.Contains(string(encoded), field) {
			t.Fatalf("unmeasured summary carries %s: %s", field, encoded)
		}
	}
}

func TestRunAccumulatorKeepsMeasuredZeroDistinctFromUnmeasured(t *testing.T) {
	summary := completeWithChildUsage(t, 0, 0, true)
	if summary.ChildCPUSeconds == nil || *summary.ChildCPUSeconds != 0 || summary.ChildWallSeconds == nil {
		t.Fatalf("measured zero usage was lost: cpu=%v wall=%v", summary.ChildCPUSeconds, summary.ChildWallSeconds)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"childCpuSeconds":0`) {
		t.Fatalf("measured zero is not encoded: %s", encoded)
	}
}

func TestRunAccumulatorChildUsageIsNotSharedAcrossCompletions(t *testing.T) {
	accumulator := evidence.NewRunAccumulator("run")
	accumulator.SetChildUsage(time.Second, time.Second, true)
	first := accumulator.Complete(policy.TaskEphemeral, evidence.OutcomePassed, 0)
	accumulator.SetChildUsage(3*time.Second, 4*time.Second, true)
	second := accumulator.Complete(policy.TaskEphemeral, evidence.OutcomePassed, 0)
	accumulator.SetChildUsage(0, 0, false)
	third := accumulator.Complete(policy.TaskEphemeral, evidence.OutcomePassed, 0)
	if *first.ChildCPUSeconds != 1 || *second.ChildCPUSeconds != 3 || third.ChildCPUSeconds != nil {
		t.Fatalf("completions share usage: first=%v second=%v third=%v", *first.ChildCPUSeconds, *second.ChildCPUSeconds, third.ChildCPUSeconds)
	}
}
