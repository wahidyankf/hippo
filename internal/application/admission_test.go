package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
)

type admissionFixture struct {
	application.RunCoordination

	admission application.Admission
}

func (fixture admissionFixture) BeginAdmission(context.Context, application.RunConfig) (application.Admission, error) {
	return fixture.admission, nil
}

type admissionClock struct {
	application.RunClock

	now   time.Time
	waits int
}

func (clock *admissionClock) Now() time.Time { return clock.now }
func (clock *admissionClock) Wait(_ context.Context, duration time.Duration, _ func(time.Duration)) error {
	clock.waits++
	clock.now = clock.now.Add(duration)
	return nil
}

func TestAdmissionApplicationOwnsRetries(t *testing.T) {
	clock := &admissionClock{now: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)}
	attempts, closed := 0, false
	fixture := admissionFixture{admission: application.Admission{
		Attempt: func(context.Context, time.Duration, time.Time) (application.AdmissionResult, error) {
			attempts++
			if attempts == 1 {
				return application.AdmissionResult{Waiting: true}, nil
			}
			return application.AdmissionResult{Lease: &application.Lease{Token: "owned"}}, nil
		},
		Receipt: func(string, time.Time) error { return nil },
		Close:   func() { closed = true },
	}}
	services := application.RunServices{Coordination: fixture, Clock: clock}
	config := application.RunConfig{ReservationPolicy: coordination.ReservationPolicy{}, Now: clock.Now}
	config.Policy.LeaseWait = time.Second
	result, err := services.Acquire(context.Background(), config)
	lease := result.Lease
	if err != nil || lease == nil || lease.Token != "owned" || attempts != 2 || clock.waits != 1 || !closed {
		t.Fatalf("lease=%v err=%v attempts=%d waits=%d closed=%t", lease, err, attempts, clock.waits, closed)
	}
}

func TestAdmissionDefaultsOptionalClock(t *testing.T) {
	started := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	clock := &admissionClock{now: started}
	attempts, closed := 0, false
	fixture := admissionFixture{admission: application.Admission{
		Attempt: func(_ context.Context, remaining time.Duration, current time.Time) (application.AdmissionResult, error) {
			attempts++
			elapsed := time.Duration(attempts-1) * 10 * time.Millisecond
			if current != started.Add(elapsed) || remaining != time.Second-elapsed {
				t.Fatalf("attempt %d timestamp=%s remaining=%s, want clock-based bounded wait", attempts, current, remaining)
			}
			if attempts == 1 {
				return application.AdmissionResult{Waiting: true}, nil
			}
			return application.AdmissionResult{Lease: &application.Lease{Token: "clock-default"}}, nil
		},
		Receipt: func(string, time.Time) error { return nil },
		Close:   func() { closed = true },
	}}
	services := application.RunServices{Coordination: fixture, Clock: clock}
	config := application.RunConfig{ReservationPolicy: coordination.ReservationPolicy{Enabled: true}}
	config.Policy.LeaseWait = time.Second
	result, err := services.Acquire(context.Background(), config)
	if err != nil || result.Lease == nil || result.Lease.Token != "clock-default" || attempts != 2 || clock.waits != 1 || !closed {
		t.Fatalf("acquisition=%+v err=%v attempts=%d waits=%d closed=%t", result, err, attempts, clock.waits, closed)
	}
}
