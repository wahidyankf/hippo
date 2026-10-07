package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/status"
)

func TestAdmissionContractDeadlineAndCancellation(t *testing.T) {
	for _, reservation := range []bool{false, true} {
		for _, receiptFails := range []bool{false, true} {
			for _, cancelled := range []bool{false, true} {
				t.Run(admissionCaseName(reservation, receiptFails, cancelled), func(t *testing.T) {
					checkAdmissionContractDeadlineAndCancellation(t, reservation, receiptFails, cancelled)
				})
			}
		}
	}
}

func admissionCaseName(reservation, receiptFails, cancelled bool) string {
	parts := []string{"compatibility"}
	if reservation {
		parts[0] = "reservation"
	}
	if receiptFails {
		parts = append(parts, "receipt-fails")
	}
	if cancelled {
		parts = append(parts, "cancelled")
	}
	return strings.Join(parts, "/")
}

func TestAdmissionContractAttemptAndBeginErrors(t *testing.T) {
	for _, stage := range []string{"begin", "attempt", "admitted"} {
		t.Run(stage, func(t *testing.T) {
			fixture := newRunContractFixture()
			fixture.begin = func(context.Context, application.RunConfig) (application.Admission, error) {
				if stage == "begin" {
					return application.Admission{}, errRunContract
				}
				admission := application.Admission{Close: func() { fixture.closed++ }, Attempt: func(context.Context, time.Duration, time.Time) (application.AdmissionResult, error) {
					return application.AdmissionResult{}, errRunContract
				}}
				if stage == "admitted" {
					admission.Admitted = &fixture.lease
				}
				return admission, nil
			}
			result, err := fixture.services().Acquire(context.Background(), fixture.settings())
			if stage == "admitted" {
				if err != nil || result.Lease != &fixture.lease {
					t.Fatalf("admitted=%+v err=%v", result, err)
				}
			} else if !errors.Is(err, errRunContract) {
				t.Fatalf("error=%v", err)
			}
			closed := 1
			if stage == "begin" {
				closed = 0
			}
			if fixture.closed != closed || fixture.waits != 0 {
				t.Fatalf("closed=%d waits=%d", fixture.closed, fixture.waits)
			}
		})
	}
}

func TestAdmissionContractHeartbeatAndPause(t *testing.T) {
	for _, target := range []string{"callback", "stderr", "silent"} {
		t.Run(target, func(t *testing.T) { checkAdmissionContractHeartbeatAndPause(t, target) })
	}
}

func checkAdmissionContractDeadlineAndCancellation(t *testing.T, reservation, receiptFails, cancelled bool) {
	t.Helper()
	fixture := newRunContractFixture()
	config := fixture.settings()
	config.ReservationPolicy.Enabled = reservation
	config.Policy.LeaseWait = 0
	if cancelled {
		config.Policy.LeaseWait = time.Second
		fixture.waitError = context.Canceled
	}
	fixture.refused = receiptFails
	receipts, attempts := 0, 0
	fixture.begin = func(context.Context, application.RunConfig) (application.Admission, error) {
		return application.Admission{Attempt: func(_ context.Context, remaining time.Duration, _ time.Time) (application.AdmissionResult, error) {
			attempts++
			if remaining < 0 {
				t.Fatal("negative bounded wait")
			}
			return application.AdmissionResult{Waiting: true}, nil
		}, Receipt: func(reason string, _ time.Time) error {
			receipts++
			expected := "admission-deadline"
			if cancelled {
				expected = evidence.OutcomeAdmissionCancelled.String()
			}
			if reason != expected {
				t.Fatalf("receipt reason=%q want=%q", reason, expected)
			}
			if receiptFails {
				return errRunContract
			}
			return nil
		}, Close: func() { fixture.closed++ }}, nil
	}
	result, err := fixture.services().Acquire(context.Background(), config)
	if attempts != 1 || fixture.closed != 1 || result.Lease != nil {
		t.Fatalf("result=%+v err=%v attempts=%d close=%d", result, err, attempts, fixture.closed)
	}
	switch {
	case !reservation && !cancelled:
		if err != nil || !result.Deferred {
			t.Fatalf("compatibility deferral=%+v %v", result, err)
		}
	case cancelled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
	case receiptFails:
		if failure, ok := errors.AsType[status.Failure](err); !ok || failure.Code != status.CodeEvidenceUnwritable {
			t.Fatalf("receipt refusal=%v", err)
		}
	default:
		if !errors.Is(err, coordination.ErrReservationDeferred) {
			t.Fatalf("reservation deferral=%v", err)
		}
	}
	wantReceipts := 0
	if reservation {
		wantReceipts = 1
	}
	if receipts != wantReceipts {
		t.Fatalf("receipts=%d want=%d", receipts, wantReceipts)
	}
}

func checkAdmissionContractHeartbeatAndPause(t *testing.T, target string) {
	t.Helper()
	fixture := newRunContractFixture()
	config := fixture.settings()
	config.ReservationPolicy.Enabled = true
	config.Policy.LeaseWait = time.Minute
	if target == "silent" {
		config.Stderr = nil
	}
	beats, pauses, attempts := 0, 0, 0
	if target == "callback" {
		config.AdmissionHeartbeat = func(wait coordination.ReservationWaitStatus) {
			beats++
			if wait.RunID != "waiting" || wait.Position != 2 {
				t.Fatalf("heartbeat=%+v", wait)
			}
		}
	}
	config.AdmissionPause = func(_ context.Context, duration time.Duration) error {
		pauses++
		if duration != 10*time.Millisecond {
			t.Fatalf("pause=%s", duration)
		}
		fixture.now = fixture.now.Add(20 * time.Second)
		return nil
	}
	fixture.begin = func(context.Context, application.RunConfig) (application.Admission, error) {
		return application.Admission{Attempt: func(context.Context, time.Duration, time.Time) (application.AdmissionResult, error) {
			attempts++
			if attempts == 4 {
				return application.AdmissionResult{Lease: &fixture.lease}, nil
			}
			return application.AdmissionResult{Waiting: true, Status: coordination.ReservationWaitStatus{RunID: "waiting", Position: 2, Remaining: time.Minute}}, nil
		}, Close: func() { fixture.closed++ }}, nil
	}
	result, err := fixture.services().Acquire(context.Background(), config)
	if err != nil || result.Lease != &fixture.lease || attempts != 4 || pauses != 3 || fixture.waits != 0 || fixture.closed != 1 {
		t.Fatalf("result=%+v err=%v attempts=%d pauses=%d waits=%d close=%d", result, err, attempts, pauses, fixture.waits, fixture.closed)
	}
	if target == "callback" && beats != 2 {
		t.Fatalf("beats=%d", beats)
	}
	if target == "stderr" && strings.Count(fixture.stderr.String(), "HIPPO queued") != 2 {
		t.Fatalf("heartbeat diagnostic=%q", fixture.stderr.String())
	}
}
