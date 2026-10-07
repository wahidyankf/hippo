package runtime_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
)

func TestNormalizePreservesAdmissionControls(t *testing.T) {
	ctx := t.Context()
	pauseError := errors.New("caller paused admission")
	wait := coordination.ReservationWaitStatus{RunID: "queued-run", Position: 2, Remaining: 3 * time.Second}
	pauseCalls, heartbeatCalls := 0, 0
	config := application.RunConfig{
		AdmissionPause: func(gotContext context.Context, duration time.Duration) error {
			pauseCalls++
			if gotContext != ctx || duration != time.Second {
				t.Errorf("pause received context=%v duration=%s", gotContext, duration)
			}
			return pauseError
		},
		AdmissionHeartbeat: func(got coordination.ReservationWaitStatus) {
			heartbeatCalls++
			if got != wait {
				t.Errorf("heartbeat = %+v, want %+v", got, wait)
			}
		},
		AdmissionCleanupWait: 7 * time.Second,
	}
	normalized := runtimeadapter.NewEngine().Normalize(config)
	if normalized.AdmissionCleanupWait != config.AdmissionCleanupWait {
		t.Errorf("cleanup wait = %s, want %s", normalized.AdmissionCleanupWait, config.AdmissionCleanupWait)
	}
	if normalized.AdmissionPause == nil {
		t.Error("normalization discarded the caller's admission pause")
	} else if err := normalized.AdmissionPause(ctx, time.Second); !errors.Is(err, pauseError) {
		t.Errorf("pause error = %v, want %v", err, pauseError)
	}
	if normalized.AdmissionHeartbeat == nil {
		t.Error("normalization discarded the caller's admission heartbeat")
	} else {
		normalized.AdmissionHeartbeat(wait)
	}
	if pauseCalls != 1 || heartbeatCalls != 1 {
		t.Errorf("callback calls: pause=%d heartbeat=%d, want one each", pauseCalls, heartbeatCalls)
	}
}

func TestNormalizeRetainsRuntimeDefaultsAndOutput(t *testing.T) {
	defaults := runtimeadapter.NewEngine().Normalize(application.RunConfig{})
	if defaults.ChildStdin == nil || defaults.ChildStdout == nil || defaults.ChildStderr == nil || defaults.Stderr == nil {
		t.Fatal("normalization omitted default process streams")
	}
	if defaults.Environment == nil || defaults.PortLeaseRoot == "" {
		t.Fatal("normalization omitted the runtime environment or port lease root")
	}
	output := &bytes.Buffer{}
	input := bytes.NewBufferString("input")
	config := application.RunConfig{
		Environment: []string{}, PortLeaseRoot: "caller-port-root", ChildStdin: input,
		ChildStdout: output, ChildStderr: output, Stderr: output,
	}
	normalized := runtimeadapter.NewEngine().Normalize(config)
	if len(normalized.Environment) != 0 || normalized.Environment == nil || normalized.PortLeaseRoot != config.PortLeaseRoot || normalized.ChildStdin != input {
		t.Fatal("normalization replaced caller-provided runtime settings")
	}
	for _, writer := range []struct {
		write func([]byte) (int, error)
		text  string
	}{
		{normalized.ChildStdout.Write, "child-out"},
		{normalized.ChildStderr.Write, "child-err"},
		{normalized.Stderr.Write, "hippo-err"},
	} {
		if _, err := writer.write([]byte(writer.text)); err != nil {
			t.Fatal(err)
		}
	}
	if got := output.String(); got != "child-outchild-errhippo-err" {
		t.Errorf("output = %q", got)
	}
}
