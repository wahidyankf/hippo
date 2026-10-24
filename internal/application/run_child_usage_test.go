package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/policy"
)

// TestRunContractRecordsChildUsageOnCompletion delivers the child's completion
// on the first supervision sample, after the fixture clock has advanced three
// seconds. The sink must receive that usage, and its wall time must be measured
// from the moment the workload started.
func TestRunContractRecordsChildUsageOnCompletion(t *testing.T) {
	for _, measured := range []bool{true, false} {
		fixture := newRunContractFixture()
		tickRunContract(fixture)
		delivered := false
		fixture.collectHook = func(call int, _ context.Context, _ *policy.Reading) {
			if call != 2 || delivered {
				return
			}
			delivered = true
			fixture.now = fixture.now.Add(3 * time.Second)
			fixture.done <- application.ChildCompletion{ExitCode: 0, CPU: 1500 * time.Millisecond, CPUMeasured: measured}
		}
		code, err := fixture.services().Run(context.Background(), fixture.settings())
		if err != nil || code != 0 {
			t.Fatalf("measured=%v code=%d error=%v trace=%v", measured, code, err, fixture.trace)
		}
		want := childUsageRecord{cpu: 1500 * time.Millisecond, wall: 3 * time.Second, measured: measured}
		if len(fixture.childUsage) != 1 || fixture.childUsage[0] != want {
			t.Fatalf("child usage = %+v, want one record %+v", fixture.childUsage, want)
		}
	}
}

// TestRunContractRecordsChildUsageWhenShed covers a pressure shed: the usage the
// stop returned must reach the sink, not be discarded with the stop's result.
func TestRunContractRecordsChildUsageWhenShed(t *testing.T) {
	for _, measured := range []bool{true, false} {
		fixture := newRunContractFixture()
		tickRunContract(fixture)
		postlaunchContractReading(fixture, criticalRunContractReading)
		fixture.stopCPU, fixture.stopCPUMeasured = 400*time.Millisecond, measured
		_, _ = fixture.services().Run(t.Context(), fixture.settings())
		if fixture.stops != 1 {
			t.Fatalf("measured=%v stops=%d, want the shed to stop the child once; trace=%v", measured, fixture.stops, fixture.trace)
		}
		if len(fixture.childUsage) != 1 || fixture.childUsage[0].measured != measured ||
			fixture.childUsage[0].cpu != 400*time.Millisecond || fixture.childUsage[0].wall < 0 {
			t.Fatalf("shed child usage = %+v, want one record with measured=%v", fixture.childUsage, measured)
		}
	}
}

// TestRunContractNeverStartedRunRecordsUnmeasuredUsage covers a run whose child
// never started: the sink is finalized with no usage, so no field is written.
func TestRunContractNeverStartedRunRecordsUnmeasuredUsage(t *testing.T) {
	fixture := newRunContractFixture()
	fixture.startError = errors.New("workload refused to start")
	code, err := fixture.services().Run(context.Background(), fixture.settings())
	if err == nil || code == 0 {
		t.Fatalf("code=%d error=%v, want the refused start to fail", code, err)
	}
	if fixture.finalizations != 1 || len(fixture.childUsage) != 1 || fixture.childUsage[0].measured {
		t.Fatalf("never-started child usage = %+v finalizations=%d, want one unmeasured record", fixture.childUsage, fixture.finalizations)
	}
}
