package support //nolint:testpackage // The readiness seam is private to the behaviour driver it synchronises.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
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

func TestPublicRootFromSymlinkTemporaryParent(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "private-temp")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "temporary-alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", alias)
	driver := &Driver{mode: contract.E2E}
	t.Cleanup(driver.Close)
	root, directory, err := driver.PreparePublicRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{directory, root} {
		if err := publicOwnedDirectory(path); err != nil {
			t.Fatalf("constructed public root fails its actual ownership boundary: %v", err)
		}
	}
	if filepath.Dir(directory) != target || root != filepath.Join(directory, "public-state") || len(driver.temporaryPaths) != 1 || driver.temporaryPaths[0] != directory {
		t.Fatal("public root lost canonical registered ownership")
	}
	driver.Close()
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned public directory survived cleanup: %v", err)
	}
	if len(driver.temporaryPaths) != 0 {
		t.Fatal("cleanup retained registered temporary ownership")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("cleanup changed the temporary parent: %v", err)
	}
}
