package application_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
)

type unavailableConformance struct{ failure error }

func (environment unavailableConformance) Open(context.Context, string) (application.ConformanceSession, error) {
	return application.ConformanceSession{}, environment.failure
}

func TestConformancePropagatesEnvironmentOpenFailure(t *testing.T) {
	failure := errors.New("manifest unavailable")
	service := application.ConformanceService{Environment: unavailableConformance{failure: failure}}
	if err := service.Run(context.Background(), "manifest.json", nil); !errors.Is(err, failure) {
		t.Fatalf("opening failure lost: got %v, want %v", err, failure)
	}
}

type fakeConformanceEnvironment struct{ session *fakeConformanceSession }

func (environment fakeConformanceEnvironment) Open(context.Context, string) (application.ConformanceSession, error) {
	return environment.session.capabilities(), nil
}

type fakeConformanceSession struct {
	binaryError        error
	binaryVerify       func() error
	manifest           application.ConformanceManifest
	mutex              sync.Mutex
	calls              []string
	verify             func(string) error
	snapshot           func(context.Context, string) (application.ConformanceCheckoutState, error)
	execute            func(context.Context, string, application.ConformanceCommand, string, io.Writer) error
	receiptError       error
	receiptVerified    bool
	receiptVerifyError error
	fixture            *fakeConformanceFixture
	fixtureError       error
	closeError         error
}

func (session *fakeConformanceSession) record(call string) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	session.calls = append(session.calls, call)
}

func (session *fakeConformanceSession) Manifest() application.ConformanceManifest {
	return session.manifest
}

func (session *fakeConformanceSession) Verify(name string) error {
	session.record("verify:" + name)
	if session.verify != nil {
		return session.verify(name)
	}
	return nil
}

func (session *fakeConformanceSession) VerifyBinary() error {
	session.record("verify-binary")
	if session.binaryVerify != nil {
		return session.binaryVerify()
	}
	return session.binaryError
}

func (session *fakeConformanceSession) Snapshot(ctx context.Context, name string) (application.ConformanceCheckoutState, error) {
	session.record("snapshot:" + name)
	if session.snapshot != nil {
		return session.snapshot(ctx, name)
	}
	return application.ConformanceCheckoutState{Head: "head", Dirty: "dirty"}, nil
}

func (session *fakeConformanceSession) Execute(ctx context.Context, name string, command application.ConformanceCommand, root string, output io.Writer) error {
	session.record("execute:" + name + ":" + command.Arguments[0])
	if session.execute != nil {
		return session.execute(ctx, name, command, root, output)
	}
	return nil
}

func (session *fakeConformanceSession) Receipts() (map[string]struct{}, error) {
	session.record("receipts")
	return map[string]struct{}{"old": {}}, session.receiptError
}

func (session *fakeConformanceSession) NeverStartedReceipt(map[string]struct{}) (bool, error) {
	session.record("receipt-proof")
	return session.receiptVerified, session.receiptVerifyError
}

func (session *fakeConformanceSession) DeferralFixture() (application.ConformanceDeferralFixture, error) {
	session.record("fixture")
	if session.fixtureError != nil {
		return application.ConformanceDeferralFixture{}, session.fixtureError
	}
	return application.ConformanceDeferralFixture{Root: session.fixture.Root, Release: session.fixture.Release, Close: session.fixture.Close}, nil
}

func (session *fakeConformanceSession) Close() error {
	session.record("close")
	return session.closeError
}

type fakeConformanceFixture struct {
	released                 atomic.Bool
	closed                   atomic.Bool
	releaseError, closeError error
}

func (fixture *fakeConformanceFixture) Root() string { return "private-root" }
func (fixture *fakeConformanceFixture) Release() error {
	fixture.released.Store(true)
	return fixture.releaseError
}

func (fixture *fakeConformanceFixture) Close() error {
	fixture.closed.Store(true)
	return fixture.closeError
}

func conformanceConsumer(name string) application.ConformanceConsumer {
	return application.ConformanceConsumer{Name: name, Bootstrap: []application.ConformanceCommand{{Arguments: []string{"bootstrap"}}}, Gates: []application.ConformanceCommand{{Arguments: []string{"gate"}}}}
}

func conformanceService(session *fakeConformanceSession) application.ConformanceService {
	instant := time.Unix(1, 0)
	var ticks atomic.Int32
	return application.ConformanceService{Environment: fakeConformanceEnvironment{session: session}, Now: func() time.Time { return instant.Add(time.Duration(ticks.Add(1)-1) * 2 * time.Second) }, Wait: func(context.Context, time.Duration) {}}
}

func TestConformanceApplicationOwnsPhaseOrderAndConcurrentConsumers(t *testing.T) {
	session := &fakeConformanceSession{manifest: application.ConformanceManifest{Consumers: []application.ConformanceConsumer{conformanceConsumer("a"), conformanceConsumer("b")}, CoordinationChecks: []application.ConformanceCheck{{Consumer: "a", Command: application.ConformanceCommand{Arguments: []string{"coordination"}}}}}}
	arrivals := make(chan struct{}, 2)
	released := make(chan struct{})
	session.execute = func(_ context.Context, _ string, command application.ConformanceCommand, _ string, _ io.Writer) error {
		if command.Arguments[0] == "bootstrap" {
			arrivals <- struct{}{}
			<-released
		}
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- conformanceService(session).Run(context.Background(), "manifest", nil) }()
	for range 2 {
		select {
		case <-arrivals:
		case <-time.After(time.Second):
			close(released)
			<-done
			t.Fatal("bootstrap consumers did not execute concurrently")
		}
	}
	close(released)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var phases []string
	for _, call := range session.calls {
		if strings.HasPrefix(call, "execute:") {
			parts := strings.Split(call, ":")
			phases = append(phases, parts[2])
		}
	}
	want := []string{"bootstrap", "bootstrap", "coordination", "gate", "gate"}
	if !slices.Equal(phases, want) {
		t.Fatalf("phase sequence %v, want %v", phases, want)
	}
	if session.calls[len(session.calls)-1] != "close" {
		t.Fatalf("session not closed: %v", session.calls)
	}
	t.Logf("phase trace: %v", session.calls)
}

func TestConformanceApplicationReconcilesAfterCancellationWithBoundedFreshContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &fakeConformanceSession{manifest: application.ConformanceManifest{Consumers: []application.ConformanceConsumer{conformanceConsumer("a")}}}
	snapshots := 0
	session.snapshot = func(snapshotContext context.Context, _ string) (application.ConformanceCheckoutState, error) {
		snapshots++
		if snapshots == 2 {
			if snapshotContext.Err() != nil {
				t.Errorf("reconciliation inherited cancellation: %v", snapshotContext.Err())
			}
			deadline, ok := snapshotContext.Deadline()
			if !ok || time.Until(deadline) > 5*time.Second || time.Until(deadline) <= 0 {
				t.Errorf("cleanup lacks five-second bound: %v %v", deadline, ok)
			}
			return application.ConformanceCheckoutState{Head: "changed"}, nil
		}
		return application.ConformanceCheckoutState{Head: "initial"}, nil
	}
	session.execute = func(context.Context, string, application.ConformanceCommand, string, io.Writer) error {
		cancel()
		return context.Canceled
	}
	err := conformanceService(session).Run(ctx, "manifest", nil)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "checkout changed") {
		t.Fatalf("cancel/reconcile result: %v", err)
	}
	if snapshots != 2 {
		t.Fatalf("snapshot count %d, want initial plus reconciliation", snapshots)
	}
	t.Logf("cancel/reconciliation trace: %v", session.calls)
}

func TestConformanceApplicationOwnsDeferralRetryAndFixtureLifetime(t *testing.T) {
	consumer := conformanceConsumer("a")
	consumer.DeferralRetryProbe = application.ConformanceCommand{Arguments: []string{"consumer-retry"}}
	fixture := &fakeConformanceFixture{}
	session := &fakeConformanceSession{manifest: application.ConformanceManifest{Consumers: []application.ConformanceConsumer{consumer}}, fixture: fixture}
	invocations := 0
	session.execute = func(_ context.Context, _ string, command application.ConformanceCommand, root string, _ io.Writer) error {
		if command.Arguments[0] == "consumer-retry" {
			if root != "private-root" {
				t.Errorf("probe root %q", root)
			}
			// The consumer owns its retry payload; application owns the fixture hold.
			invocations++
		}
		return nil
	}
	var output bytes.Buffer
	if err := conformanceService(session).Run(context.Background(), "manifest", &output); err != nil {
		t.Fatal(err)
	}
	if invocations != 1 || !fixture.released.Load() || !fixture.closed.Load() {
		t.Fatalf("probe invocations=%d released=%v closed=%v", invocations, fixture.released.Load(), fixture.closed.Load())
	}
	if !strings.Contains(output.String(), "retried a capacity deferral") {
		t.Fatalf("missing probe verdict: %s", output.String())
	}
	t.Logf("deferral fixture trace: %v", session.calls)
}

type conformanceExitError struct {
	category string
	exit     int
	cause    bool
}

func (failure conformanceExitError) Error() string { return "command failed" }
func (failure conformanceExitError) ConformanceFailure() (string, int, bool) {
	return failure.category, failure.exit, failure.cause
}

func TestConformanceApplicationRequiresReceiptForCapacitySkip(t *testing.T) {
	for _, row := range []struct {
		name        string
		verified    bool
		exit        int
		diagnostic  string
		wantFailure bool
	}{
		{"proved never-started", true, 124, "hippo: [hippo.limit.capacity-deferred]", false},
		{"missing proof", false, 124, "hippo: [hippo.limit.capacity-deferred]", true},
		{"pressure shed", true, 124, "hippo: [hippo.limit.pressure-shed]", true},
		{"protocol mismatch", true, 125, "hippo: [hippo.limit.capacity-deferred]", true},
	} {
		t.Run(row.name, func(t *testing.T) {
			session := &fakeConformanceSession{manifest: application.ConformanceManifest{Consumers: []application.ConformanceConsumer{conformanceConsumer("a")}, CoordinationChecks: []application.ConformanceCheck{{Consumer: "a", Command: application.ConformanceCommand{Arguments: []string{"capacity"}}, AllowCapacitySkip: true}}}, receiptVerified: row.verified}
			session.execute = func(_ context.Context, _ string, command application.ConformanceCommand, _ string, output io.Writer) error {
				if command.Arguments[0] == "capacity" {
					if _, err := io.WriteString(output, row.diagnostic); err != nil {
						return err
					}
					return conformanceExitError{category: "exited", exit: row.exit}
				}
				return nil
			}
			var output bytes.Buffer
			err := conformanceService(session).Run(context.Background(), "manifest", &output)
			if (err != nil) != row.wantFailure {
				t.Fatalf("capacity result %v, want failure=%v", err, row.wantFailure)
			}
			t.Logf("capacity proof trace: %v", session.calls)
		})
	}
}

func TestConformanceApplicationFailurePathsPreserveReconciliation(t *testing.T) {
	failure := errors.New("injected failure")
	rows := []struct {
		name      string
		configure func(*fakeConformanceSession, *application.ConformanceService)
		want      string
	}{
		{"initial snapshot", func(session *fakeConformanceSession, _ *application.ConformanceService) {
			session.snapshot = func(context.Context, string) (application.ConformanceCheckoutState, error) {
				return application.ConformanceCheckoutState{}, failure
			}
		}, "injected failure"},
		{"reconciliation snapshot", func(session *fakeConformanceSession, _ *application.ConformanceService) {
			snapshots := 0
			session.snapshot = func(context.Context, string) (application.ConformanceCheckoutState, error) {
				snapshots++
				if snapshots == 2 {
					return application.ConformanceCheckoutState{}, failure
				}
				return application.ConformanceCheckoutState{}, nil
			}
		}, "injected failure"},
		{"bootstrap identity", func(session *fakeConformanceSession, _ *application.ConformanceService) {
			session.verify = func(string) error { return failure }
		}, "bootstrap failed"},
		{"deferral fixture", func(session *fakeConformanceSession, _ *application.ConformanceService) {
			session.manifest.Consumers[0].DeferralRetryProbe = application.ConformanceCommand{Arguments: []string{"probe"}}
			session.fixtureError = failure
		}, "injected failure"},
		{"deferral command", func(session *fakeConformanceSession, _ *application.ConformanceService) {
			session.manifest.Consumers[0].DeferralRetryProbe = application.ConformanceCommand{Arguments: []string{"probe"}}
			session.execute = func(_ context.Context, _ string, command application.ConformanceCommand, _ string, _ io.Writer) error {
				if command.Arguments[0] == "probe" {
					return failure
				}
				return nil
			}
		}, "would read exit 124 as an admission"},
		{"deferral release", func(session *fakeConformanceSession, _ *application.ConformanceService) {
			session.manifest.Consumers[0].DeferralRetryProbe = application.ConformanceCommand{Arguments: []string{"probe"}}
			session.fixture.releaseError = failure
		}, "injected failure"},
		{"deferral early success", func(session *fakeConformanceSession, service *application.ConformanceService) {
			session.manifest.Consumers[0].DeferralRetryProbe = application.ConformanceCommand{Arguments: []string{"probe"}}
			service.Now = func() time.Time { return time.Unix(1, 0) }
		}, "before capacity could free"},
		{"deferral disturbs identity", func(session *fakeConformanceSession, _ *application.ConformanceService) {
			verifications := 0
			session.verify = func(string) error {
				verifications++
				if verifications == 3 {
					return failure
				}
				return nil
			}
		}, "disturbed state"},
		{"receipt inventory", func(session *fakeConformanceSession, _ *application.ConformanceService) {
			session.receiptError = failure
		}, "injected failure"},
		{"receipt proof", func(session *fakeConformanceSession, _ *application.ConformanceService) {
			session.receiptVerifyError = failure
			session.execute = func(_ context.Context, _ string, command application.ConformanceCommand, _ string, _ io.Writer) error {
				if command.Arguments[0] == "capacity" {
					return conformanceExitError{category: "exited", exit: 124}
				}
				return nil
			}
		}, "injected failure"},
		{"coordination identity", func(session *fakeConformanceSession, _ *application.ConformanceService) {
			verifications := 0
			session.verify = func(string) error {
				verifications++
				if verifications == 4 {
					return failure
				}
				return nil
			}
		}, "coordination check"},
		{"close", func(session *fakeConformanceSession, _ *application.ConformanceService) { session.closeError = failure }, "injected failure"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			session := &fakeConformanceSession{manifest: application.ConformanceManifest{Consumers: []application.ConformanceConsumer{conformanceConsumer("a")}, CoordinationChecks: []application.ConformanceCheck{{Consumer: "a", Command: application.ConformanceCommand{Arguments: []string{"capacity"}}, AllowCapacitySkip: true}}}, fixture: &fakeConformanceFixture{}}
			service := conformanceService(session)
			row.configure(session, &service)
			err := service.Run(context.Background(), "manifest", nil)
			if err == nil || !strings.Contains(err.Error(), row.want) {
				t.Fatalf("failure %v, want %q; trace %v", err, row.want, session.calls)
			}
			if session.calls[len(session.calls)-1] != "close" {
				t.Fatalf("failure did not close session: %v", session.calls)
			}
		})
	}
}

func TestConformanceCapacitySkipRejectsAmbiguousErrorShapes(t *testing.T) {
	failure := conformanceExitError{category: "exited", exit: 124}
	diagnostic := []byte("hippo: [hippo.limit.capacity-deferred]")
	for _, err := range []error{nil, errors.New("unknown"), fmt.Errorf("wrapped: %w", failure), errors.Join(failure, errors.New("integrity")), conformanceExitError{category: "exited", exit: 124, cause: true}} {
		if application.CleanConformanceCapacitySkip(err, diagnostic, true) {
			t.Fatalf("ambiguous shape accepted: %v", err)
		}
	}
}

type failingConformanceWriter struct{}

func (failingConformanceWriter) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable")
}

func TestConformanceApplicationReportsOutputFailures(t *testing.T) {
	session := &fakeConformanceSession{manifest: application.ConformanceManifest{Consumers: []application.ConformanceConsumer{conformanceConsumer("a")}}}
	if err := conformanceService(session).Run(context.Background(), "manifest", failingConformanceWriter{}); err == nil || !strings.Contains(err.Error(), "output unavailable") {
		t.Fatalf("lost output failure: %v", err)
	}
}

func TestConformanceApplicationDefaultClockAndWaitCancel(t *testing.T) {
	consumer := conformanceConsumer("a")
	consumer.DeferralRetryProbe = application.ConformanceCommand{Arguments: []string{"probe"}}
	session := &fakeConformanceSession{manifest: application.ConformanceManifest{Consumers: []application.ConformanceConsumer{consumer}}, fixture: &fakeConformanceFixture{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session.execute = func(_ context.Context, _ string, command application.ConformanceCommand, _ string, _ io.Writer) error {
		if command.Arguments[0] == "probe" {
			return ctx.Err()
		}
		return nil
	}
	service := application.ConformanceService{Environment: fakeConformanceEnvironment{session: session}}
	if err := service.Run(ctx, "manifest", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled probe error: %v", err)
	}
	if !session.fixture.released.Load() || !session.fixture.closed.Load() {
		t.Fatal("cancelled probe fixture leaked")
	}
}

func TestConformanceApplicationPreservesBareBinaryFailureCategory(t *testing.T) {
	session := &fakeConformanceSession{manifest: application.ConformanceManifest{Consumers: []application.ConformanceConsumer{conformanceConsumer("a")}}, binaryError: errors.New("HIPPO binary identity changed during conformance")}
	err := conformanceService(session).Run(context.Background(), "manifest", nil)
	if err == nil || strings.Contains(err.Error(), "consumer") || !strings.Contains(err.Error(), "HIPPO binary identity changed") {
		t.Fatalf("binary category changed: %v", err)
	}
	for _, call := range session.calls {
		if strings.HasPrefix(call, "execute:") {
			t.Fatalf("changed binary executed: %v", session.calls)
		}
	}
}

func TestConformanceApplicationPreservesCoordinationBinaryFailureCategory(t *testing.T) {
	session := &fakeConformanceSession{manifest: application.ConformanceManifest{Consumers: []application.ConformanceConsumer{conformanceConsumer("a")}, CoordinationChecks: []application.ConformanceCheck{{Consumer: "a", Command: application.ConformanceCommand{Arguments: []string{"coordination"}}}}}}
	verifications := 0
	session.binaryVerify = func() error {
		verifications++
		if verifications == 2 {
			return errors.New("HIPPO binary identity changed during conformance")
		}
		return nil
	}
	err := conformanceService(session).Run(context.Background(), "manifest", nil)
	if err == nil || strings.Contains(err.Error(), "consumer") || !strings.Contains(err.Error(), "HIPPO binary identity changed") {
		t.Fatalf("coordination binary category changed: %v", err)
	}
	for _, call := range session.calls {
		if strings.Contains(call, "coordination") || strings.Contains(call, ":gate") {
			t.Fatalf("later phase executed: %v", session.calls)
		}
	}
}

func (session *fakeConformanceSession) capabilities() application.ConformanceSession {
	return application.ConformanceSession{
		Manifest: session.Manifest, Verify: session.Verify, VerifyBinary: session.VerifyBinary,
		Snapshot: session.Snapshot, Execute: session.Execute, Receipts: session.Receipts,
		NeverStartedReceipt: session.NeverStartedReceipt, DeferralFixture: session.DeferralFixture, Close: session.Close,
	}
}
