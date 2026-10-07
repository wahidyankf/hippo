package runtime_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/support/runtimewiring"

	guard "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestExclusiveStatusCountsLiveSessionsAndDeduplicatesHeavyOwner(t *testing.T) {
	root := t.TempDir()
	heavy, err := runtimewiring.AcquireSession(context.Background(), root, "", policy.TaskEphemeral, time.Second)
	if err != nil || heavy == nil {
		t.Fatalf("acquire heavy session: session=%v error=%v", heavy != nil, err)
	}
	defer func() { _ = guard.ReleaseSession(root, heavy) }()
	service, err := runtimewiring.AcquireSession(context.Background(), root, "", policy.TaskService, time.Second)
	if err != nil || service == nil {
		t.Fatalf("acquire service session: session=%v error=%v", service != nil, err)
	}
	defer func() { _ = guard.ReleaseSession(root, service) }()

	paths := []string{
		filepath.Join(root, "coordination-mode.json"),
		filepath.Join(root, "heavy.lock", "owner.json"),
		heavy.RecordPath,
		service.RecordPath,
	}
	before := make(map[string][]byte, len(paths))
	for _, path := range paths {
		before[path], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}

	totals, err := guard.ExclusiveStatus(context.Background(), root)
	if err != nil || totals.SchemaVersion != 5 || totals.Mode != "exclusive" ||
		totals.ActiveOwners != 2 || totals.WaitingOwners != 0 || totals.Ephemeral != 1 || totals.Service != 1 ||
		totals.Transactional != 0 || totals.LegacyEntries != 2 || len(totals.Owners) != 2 {
		t.Fatalf("exclusive totals=%+v error=%v", totals, err)
	}
	for _, owner := range totals.Owners {
		if owner.State != "active" || !owner.Legacy || owner.RunID == "" {
			t.Fatalf("invalid exclusive owner row: %+v", owner)
		}
	}
	if totals.Owners[0].RunID > totals.Owners[1].RunID {
		t.Fatalf("exclusive owners are not deterministic: %+v", totals.Owners)
	}
	for path, original := range before {
		after, readError := os.ReadFile(path)
		if readError != nil || !bytes.Equal(original, after) {
			t.Fatalf("status changed %s: error=%v", filepath.Base(path), readError)
		}
	}
}

func TestExclusiveStatusClassifiesSessionStateWithoutMutation(t *testing.T) {
	for _, testCase := range []struct {
		name, contents string
		protocol       bool
	}{
		{name: "malformed", contents: `{"schemaVersion":1`, protocol: false},
		{name: "future", contents: `{"schemaVersion":2}`, protocol: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			session, err := runtimewiring.AcquireSession(context.Background(), root, "", policy.TaskService, time.Second)
			if err != nil || session == nil {
				t.Fatalf("acquire service session: session=%v error=%v", session != nil, err)
			}
			defer func() { _ = guard.ReleaseSession(root, session) }()
			contents := []byte(testCase.contents + "\n")
			if err = os.WriteFile(session.RecordPath, contents, 0o600); err != nil {
				t.Fatal(err)
			}

			_, statusError := guard.ExclusiveStatus(context.Background(), root)
			if statusError == nil || guard.IsCoordinationProtocolMismatch(statusError) != testCase.protocol {
				t.Fatalf("status error=%v protocol=%t", statusError, testCase.protocol)
			}
			after, readError := os.ReadFile(session.RecordPath)
			if readError != nil || !bytes.Equal(contents, after) {
				t.Fatalf("status changed invalid state: %q error=%v", after, readError)
			}
		})
	}
}

func TestExclusiveStatusWithoutHeavyOwnerCountsSessionsOnly(t *testing.T) {
	root := t.TempDir()
	session, err := runtimewiring.AcquireSession(context.Background(), root, "", policy.TaskService, time.Second)
	if err != nil || session == nil {
		t.Fatalf("acquire service session: session=%v error=%v", session != nil, err)
	}
	defer func() { _ = guard.ReleaseSession(root, session) }()
	if _, statError := os.Stat(filepath.Join(root, "heavy.lock")); !os.IsNotExist(statError) {
		t.Fatalf("a service session must not hold the heavy lock: %v", statError)
	}

	totals, err := guard.ExclusiveStatus(context.Background(), root)
	if err != nil || totals.ActiveOwners != 1 || totals.Service != 1 || len(totals.Owners) != 1 {
		t.Fatalf("exclusive totals=%+v error=%v", totals, err)
	}
}

func TestExclusiveStatusRefusesNullHeavyOwnerDocument(t *testing.T) {
	root := t.TempDir()
	session, err := runtimewiring.AcquireSession(context.Background(), root, "", policy.TaskService, time.Second)
	if err != nil || session == nil {
		t.Fatalf("acquire service session: session=%v error=%v", session != nil, err)
	}
	defer func() { _ = guard.ReleaseSession(root, session) }()
	heavyOwner := filepath.Join(root, "heavy.lock", "owner.json")
	if err = os.MkdirAll(filepath.Dir(heavyOwner), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(heavyOwner, []byte("null\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, statusError := guard.ExclusiveStatus(context.Background(), root)
	if statusError == nil || guard.IsCoordinationProtocolMismatch(statusError) {
		t.Fatalf("a null heavy owner document must be refused as unreadable state: %v", statusError)
	}
	after, readError := os.ReadFile(heavyOwner)
	if readError != nil || string(after) != "null\n" {
		t.Fatalf("status changed the heavy owner document: %q error=%v", after, readError)
	}
}
