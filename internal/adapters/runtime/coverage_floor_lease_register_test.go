package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"os"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestFloorRegisterSessionReportsLeaseOwnerWriteFailure(t *testing.T) {
	root := t.TempDir()
	marker, err := os.CreateTemp(root, "lock-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	if _, err := registerSession(
		system.Production, root, marker.Name(), policy.TaskEphemeral, identity.DisplayLabel{}, nil,
	); err == nil {
		t.Fatal("registerSession with a file lease path = nil, want error")
	}
}
