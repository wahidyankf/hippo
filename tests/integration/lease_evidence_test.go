package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

func marshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestHeavyLeaseLifecycleAndInheritance(t *testing.T) {
	root := t.TempDir()
	session, err := guard.AcquireSession(context.Background(), root, "", "ephemeral", time.Second)
	if err != nil || session == nil || session.Inherited {
		t.Fatalf("acquire failed: session=%+v error=%v", session, err)
	}

	if !guard.InheritedSession(root, session.Token) || guard.InheritedSession(root, "") || guard.InheritedSession(root, "wrong") {
		t.Fatal("inheritance validation failed")
	}
	inherited, err := guard.AcquireSession(context.Background(), root, session.Token, "ephemeral", 0)
	if err != nil || !inherited.Inherited {
		t.Fatalf("inherit failed: %+v %v", inherited, err)
	}

	deferred, err := guard.AcquireSession(context.Background(), root, "wrong", "ephemeral", 0)
	if err != nil || deferred != nil {
		t.Fatalf("second owner was not deferred: %+v %v", deferred, err)
	}

	if err := guard.ReleaseSession(root, inherited); err != nil {
		t.Fatal(err)
	}
	if err := guard.ReleaseSession(root, session); err != nil {
		t.Fatal(err)
	}
	if err := guard.ReleaseSession(root, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCoordinationModeLifecycleAcrossConcurrentServices(t *testing.T) {
	root := t.TempDir()
	const owners = 12

	sessions := make(chan *guard.Session, owners)
	errorsFound := make(chan error, owners)
	var wait sync.WaitGroup
	for range owners {
		wait.Go(func() {
			session, err := guard.AcquireSession(context.Background(), root, "", policy.TaskService, time.Second)
			if err != nil {
				errorsFound <- err

				return
			}
			if session == nil {
				errorsFound <- errors.New("service admission was deferred")

				return
			}

			sessions <- session
		})
	}
	wait.Wait()
	close(sessions)
	close(errorsFound)

	for err := range errorsFound {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	markerPath := filepath.Join(root, "coordination-mode.json")
	marker := map[string]any{}
	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &marker); err != nil {
		t.Fatal(err)
	}
	if marker["schemaVersion"] != float64(1) || marker["mode"] != "exclusive" {
		t.Fatalf("unexpected marker: %s", data)
	}

	owned := make([]*guard.Session, 0, owners)
	for session := range sessions {
		owned = append(owned, session)
	}
	if len(owned) != owners {
		t.Fatalf("acquired %d service sessions, want %d", len(owned), owners)
	}

	for _, session := range owned[:len(owned)-1] {
		if err = guard.ReleaseSession(root, session); err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(markerPath); err != nil {
			t.Fatalf("active coordination marker disappeared: %v", err)
		}
	}
	if err = guard.ReleaseSession(root, owned[len(owned)-1]); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("idle coordination marker remained: %v", err)
	}
}

func TestCoordinationMarkerRejectsIncompatibleOrMalformedState(t *testing.T) {
	testCases := []struct {
		name   string
		marker string
	}{
		{name: "future schema", marker: `{"schemaVersion":2,"mode":"exclusive"}`},
		{name: "unknown mode", marker: `{"schemaVersion":1,"mode":"unknown"}`},
		{name: "unknown field", marker: `{"schemaVersion":1,"mode":"exclusive","extra":true}`},
		{name: "duplicate mode", marker: `{"schemaVersion":1,"mode":"reservation","mode":"exclusive"}`},
		{name: "multiple values", marker: `{"schemaVersion":1,"mode":"exclusive"} {}`},
		{name: "invalid JSON", marker: `{`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			markerPath := filepath.Join(root, "coordination-mode.json")
			if err := os.WriteFile(markerPath, []byte(testCase.marker), 0o600); err != nil {
				t.Fatal(err)
			}

			session, err := guard.AcquireSession(context.Background(), root, "", policy.TaskEphemeral, 0)
			if err == nil || session != nil {
				t.Fatalf("incompatible marker was accepted: session=%+v error=%v", session, err)
			}

			data, readError := os.ReadFile(markerPath)
			if readError != nil {
				t.Fatal(readError)
			}
			if string(data) != testCase.marker {
				t.Fatalf("marker changed from %q to %q", testCase.marker, data)
			}
		})
	}
}

func TestReservationCoordinationDefersEveryCompatibilityClass(t *testing.T) {
	root := t.TempDir()
	marker := []byte("{\"schemaVersion\":1,\"mode\":\"reservation\"}\n")
	markerPath := filepath.Join(root, "coordination-mode.json")
	if err := os.WriteFile(markerPath, marker, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, class := range []policy.TaskClass{policy.TaskEphemeral, policy.TaskTransactional, policy.TaskService} {
		t.Run(string(class), func(t *testing.T) {
			session, err := guard.AcquireSession(context.Background(), root, "", class, 0)
			if err == nil || session != nil {
				t.Fatalf("reservation coordination admitted %s: session=%+v error=%v", class, session, err)
			}
			if !strings.Contains(err.Error(), "reservation mode is active") {
				t.Fatalf("%s deferral was not actionable: %v", class, err)
			}
		})
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(marker) {
		t.Fatalf("reservation marker changed from %q to %q", marker, data)
	}
}

func TestMalformedHeavyLeaseRemainsFailClosed(t *testing.T) {
	testCases := []struct {
		name  string
		owner string
	}{
		{name: "invalid JSON", owner: `{`},
		{name: "unsupported schema", owner: `{"schemaVersion":2,"pid":2147483647}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			heavyPath := filepath.Join(root, "heavy.lock")
			if err := os.Mkdir(heavyPath, 0o700); err != nil {
				t.Fatal(err)
			}
			ownerPath := filepath.Join(heavyPath, "owner.json")
			if err := os.WriteFile(ownerPath, []byte(testCase.owner), 0o600); err != nil {
				t.Fatal(err)
			}

			session, err := guard.AcquireSession(context.Background(), root, "", policy.TaskEphemeral, 0)
			if err != nil || session != nil {
				t.Fatalf("unverifiable heavy lease did not defer safely: session=%+v error=%v", session, err)
			}
			if description := guard.DescribeHeavyLease(root); !strings.Contains(description, "cannot be verified") {
				t.Fatalf("unverifiable owner diagnostic was not actionable: %q", description)
			}

			data, readError := os.ReadFile(ownerPath)
			if readError != nil {
				t.Fatal(readError)
			}
			if string(data) != testCase.owner {
				t.Fatalf("unverifiable heavy owner changed from %q to %q", testCase.owner, data)
			}
			mode, readError := os.ReadFile(filepath.Join(root, "coordination-mode.json"))
			if readError != nil {
				t.Fatal(readError)
			}
			if !strings.Contains(string(mode), `"mode":"exclusive"`) {
				t.Fatalf("fail-closed coordination marker missing: %q", mode)
			}
		})
	}
}

func TestHeavyLeaseRejectsInvalidReleaseAndReclaimsStaleOwner(t *testing.T) {
	root := t.TempDir()
	lock := filepath.Join(root, "heavy.lock")
	if err := os.Mkdir(lock, 0o700); err != nil {
		t.Fatal(err)
	}

	marker := map[string]any{"schemaVersion": 1, "pid": 2_147_483_647, "token": "stale"}
	data := marshalJSON(t, marker)
	if err := os.WriteFile(filepath.Join(lock, "owner.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	session, err := guard.AcquireSession(context.Background(), root, "", "ephemeral", time.Second)
	if err != nil || session == nil {
		t.Fatalf("stale reclaim failed: %+v %v", session, err)
	}

	invalid := *session
	invalid.Path = filepath.Join(root, "other")
	if guard.ReleaseSession(root, &invalid) == nil {
		t.Fatal("invalid path release accepted")
	}

	ownerPath := filepath.Join(lock, "owner.json")
	ownerData, _ := os.ReadFile(ownerPath)
	var owner map[string]any
	_ = json.Unmarshal(ownerData, &owner)
	owner["token"] = "other"
	ownerData = marshalJSON(t, owner)
	_ = os.WriteFile(ownerPath, ownerData, 0o600)
	if guard.ReleaseSession(root, session) == nil {
		t.Fatal("foreign owner release accepted")
	}

	owner["token"] = session.Token
	ownerData = marshalJSON(t, owner)
	_ = os.WriteFile(ownerPath, ownerData, 0o600)
	if err := guard.ReleaseSession(root, session); err != nil {
		t.Fatal(err)
	}
}

func TestPortLeaseLifecycleValidationAndStaleRecovery(t *testing.T) {
	root := t.TempDir()
	if _, err := guard.AcquirePortLease(root, 10, "owner", 20, 30); err == nil {
		t.Fatal("out-of-range port accepted")
	}

	if _, err := guard.AcquirePortLease(root, 25, "INVALID", 20, 30); err == nil {
		t.Fatal("invalid owner accepted")
	}

	lease, err := guard.AcquirePortLease(root, 25, "first-owner", 20, 30)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := guard.AcquirePortLease(root, 25, "second-owner", 20, 30); err == nil {
		t.Fatal("live collision accepted")
	}
	invalid := *lease
	invalid.Path = filepath.Join(root, "invalid")
	if guard.ReleasePortLease(root, &invalid) == nil {
		t.Fatal("invalid release accepted")
	}

	ownerPath := filepath.Join(lease.Path, "owner.json")
	data, _ := os.ReadFile(ownerPath)
	var owner map[string]any
	_ = json.Unmarshal(data, &owner)
	owner["owner"] = "other"
	data = marshalJSON(t, owner)
	_ = os.WriteFile(ownerPath, data, 0o600)
	if guard.ReleasePortLease(root, lease) == nil {
		t.Fatal("foreign port release accepted")
	}

	owner["owner"] = lease.Owner
	data = marshalJSON(t, owner)
	_ = os.WriteFile(ownerPath, data, 0o600)
	if err := guard.ReleasePortLease(root, lease); err != nil {
		t.Fatal(err)
	}
	if err := guard.ReleasePortLease(root, nil); err != nil {
		t.Fatal(err)
	}

	stalePath := filepath.Join(root, "26.lock")
	_ = os.Mkdir(stalePath, 0o700)
	stale := marshalJSON(t, map[string]any{"schemaVersion": 1, "pid": 2_147_483_647, "port": 26, "owner": "stale"})
	_ = os.WriteFile(filepath.Join(stalePath, "owner.json"), stale, 0o600)

	replacement, err := guard.AcquirePortLease(root, 26, "replacement", 20, 30)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.ReleasePortLease(root, replacement); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceLifecycleSummaryAndCleanup(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(2_000_000, 0)
	if err := evidence.Cleanup(root, now); err != nil {
		t.Fatal(err)
	}

	old := filepath.Join(root, "old.jsonl")
	summaryOld := filepath.Join(root, "old.summary.json")
	preserve := filepath.Join(root, "preserve.jsonl")
	for _, path := range []string{old, summaryOld, preserve} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		stale := now.Add(-31 * 24 * time.Hour)
		_ = os.Chtimes(path, stale, stale)
	}

	if err := evidence.Cleanup(root, now, preserve); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("expired evidence retained")
	}
	if _, err := os.Stat(summaryOld); !os.IsNotExist(err) {
		t.Fatal("expired summary retained")
	}
	if _, err := os.Stat(preserve); err != nil {
		t.Fatal("preserved evidence removed")
	}

	writer, err := guard.NewEvidenceWriter(root, guard.EvidenceIdentifier("integration/test", now, os.Getpid()), evidence.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	writer.SetContext(policy.Resolution{
		RequestedProfile: "balanced",
		ResolvedProfile:  "minimal",
		FallbackChain:    []string{"balanced", "minimal"},
		Concurrency:      1,
	}, "config-hash")

	one := int64(1)
	two := int64(2)
	levelOne := 1
	levelTwo := 2
	yes := true
	compressorUnavailable := false
	cpuOne, cpuTwo := 10.0, 20.0

	first := policy.Sample{
		AvailableParallelism:                8,
		AvailableNonCompressedEstimateBytes: &two,
		MemoryPressureLevel:                 &levelOne,
		CompressorAvailable:                 &yes,
		CompressorPayloadBytes:              &one,
		CPUUtilizationPercent:               &cpuOne,
		DiskFreeBytes:                       &two,
		SwapIns:                             &one,
		SwapOuts:                            &one,
		SwapFreeBytes:                       &two,
	}
	second := policy.Sample{
		AvailableParallelism:                8,
		AvailableNonCompressedEstimateBytes: &one,
		MemoryPressureLevel:                 &levelTwo,
		CompressorAvailable:                 &compressorUnavailable,
		CompressorPayloadBytes:              &two,
		CPUUtilizationPercent:               &cpuTwo,
		DiskFreeBytes:                       &one,
		SwapIns:                             &two,
		SwapOuts:                            &two,
		SwapFreeBytes:                       &one,
	}

	if err := writer.Append(first); err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(second); err != nil {
		t.Fatal(err)
	}

	summary, err := writer.Finalize("ephemeral", "passed", 1)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != 3 ||
		summary.SampleCount != 2 ||
		summary.CompressorAvailableAll ||
		summary.SwapInsDelta != 1 ||
		summary.SwapOutsDelta != 1 ||
		summary.HealthFailures != 1 ||
		summary.ResolvedProfile != "minimal" ||
		summary.ConfigHash != "config-hash" {
		t.Fatalf("unexpected summary %+v", summary)
	}

	if _, err := writer.Finalize("ephemeral", "passed", 0); err == nil {
		t.Fatal("second finalize accepted")
	}
}

func TestEvidenceCleanupPreservesCoordinationProtocolFiles(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(2_000_000, 0)
	coordinationLock := filepath.Join(root, "coordination.lock")
	coordinationMarker := filepath.Join(root, "coordination-mode.json")
	expiredEvidence := filepath.Join(root, "expired.jsonl")
	for path, content := range map[string]string{
		coordinationLock:   "",
		coordinationMarker: `{"schemaVersion":1,"mode":"exclusive"}`,
		expiredEvidence:    "{}\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		stale := now.Add(-31 * 24 * time.Hour)
		if err := os.Chtimes(path, stale, stale); err != nil {
			t.Fatal(err)
		}
	}

	if err := evidence.Cleanup(root, now); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{coordinationLock, coordinationMarker} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("coordination protocol file %q was removed: %v", filepath.Base(path), err)
		}
	}
	if _, err := os.Stat(expiredEvidence); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired evidence remained: %v", err)
	}
}

func TestServiceSessionsDoNotHoldTheHeavyLease(t *testing.T) {
	root := t.TempDir()
	service, err := guard.AcquireSession(context.Background(), root, "", "service", time.Second)
	if err != nil || service == nil {
		t.Fatalf("service acquire failed: session=%+v error=%v", service, err)
	}

	if _, statError := os.Stat(filepath.Join(root, "heavy.lock")); !os.IsNotExist(statError) {
		t.Fatal("a service session took the heavy-work lease")
	}
	heavy, err := guard.AcquireSession(context.Background(), root, "", "ephemeral", time.Second)
	if err != nil || heavy == nil {
		t.Fatalf("heavy work was deferred by a live service: session=%+v error=%v", heavy, err)
	}

	if !guard.InheritedSession(root, service.Token) {
		t.Fatal("a service child could not inherit its own session")
	}
	if !guard.InheritedSession(root, heavy.Token) {
		t.Fatal("a heavy child could not inherit its own session")
	}

	second, err := guard.AcquireSession(context.Background(), root, "", "service", time.Second)
	if err != nil || second == nil {
		t.Fatalf("a concurrent service was deferred: session=%+v error=%v", second, err)
	}
	if second.Token == service.Token {
		t.Fatal("concurrent services shared one session token")
	}

	for _, owned := range []*guard.Session{service, second, heavy} {
		if releaseError := guard.ReleaseSession(root, owned); releaseError != nil {
			t.Fatal(releaseError)
		}
	}

	if guard.InheritedSession(root, service.Token) {
		t.Fatal("a released service session stayed inheritable")
	}
}

func TestHeavyLeaseDeferralDescribesItsHolder(t *testing.T) {
	root := t.TempDir()
	holder, err := guard.AcquireSession(context.Background(), root, "", "ephemeral", time.Second)
	if err != nil || holder == nil {
		t.Fatalf("acquire failed: session=%+v error=%v", holder, err)
	}

	deferred, err := guard.AcquireSession(context.Background(), root, "", "ephemeral", 200*time.Millisecond)
	if err != nil || deferred != nil {
		t.Fatalf("second heavy owner was not deferred: %+v %v", deferred, err)
	}

	description := guard.DescribeHeavyLease(root)
	if !strings.Contains(description, "heavy-work lease") ||
		!strings.Contains(description, strconv.Itoa(os.Getpid())) ||
		!strings.Contains(description, "ephemeral") {
		t.Fatalf("deferral description did not name the holder: %q", description)
	}
	if err := guard.ReleaseSession(root, holder); err != nil {
		t.Fatal(err)
	}

	if empty := guard.DescribeHeavyLease(root); !strings.Contains(empty, "no live") {
		t.Fatalf("released lease was still described as held: %q", empty)
	}
}
