package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/identity"
)

const displayContractToken = "0123456789abcdef0123456789abcdef"

func displayContractLabel(t *testing.T, value string) identity.DisplayLabel {
	t.Helper()
	label, err := identity.ParseDisplayLabel(value)
	if err != nil {
		t.Fatal(err)
	}
	return label
}

func displayContractRead(t *testing.T, root, token string) application.DisplayAnnotation {
	t.Helper()
	annotations, err := (evidence.DisplayRepository{}).ReadDisplay(t.Context(), root, []string{token})
	if err != nil {
		t.Fatal(err)
	}
	return annotations[token]
}

func TestDisplayRepositoryStrictBoundedRecords(t *testing.T) {
	valid := fmt.Sprintf(`{"schemaVersion":1,"runId":%q,"label":"check-docs"}`, displayContractToken)
	cases := []struct {
		name, data string
		want       application.DisplayAvailability
	}{
		{"valid", valid, application.DisplayAvailable},
		{"exact-512-byte-bound", valid + strings.Repeat(" ", 512-len(valid)), application.DisplayAvailable},
		{"oversize", valid + strings.Repeat(" ", 513-len(valid)), application.DisplayInvalid},
		{"corrupt", "{", application.DisplayInvalid},
		{"non-object", "[]", application.DisplayInvalid},
		{"duplicate", strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1), application.DisplayInvalid},
		{"unknown-field", strings.Replace(valid, `"label":`, `"private":true,"label":`, 1), application.DisplayInvalid},
		{"trailing-value", valid + " {}", application.DisplayInvalid},
		{"mismatched-run-id", strings.Replace(valid, displayContractToken, strings.Repeat("f", 32), 1), application.DisplayInvalid},
		{"unset-schema", strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":0`, 1), application.DisplayInvalid},
		{"unsupported-schema", strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":2`, 1), application.DisplayUnsupported},
		{"missing-field", fmt.Sprintf(`{"schemaVersion":1,"runId":%q}`, displayContractToken), application.DisplayInvalid},
		{"wrong-field-type", strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":"1"`, 1), application.DisplayInvalid},
		{"unsafe-label", strings.Replace(valid, "check-docs", "../private", 1), application.DisplayInvalid},
		{"escaped-control", strings.Replace(valid, "check-docs", `bad\u001b[31m`, 1), application.DisplayInvalid},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "run-display"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "run-display", displayContractToken+".json"), []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			annotation := displayContractRead(t, root, displayContractToken)
			if annotation.Availability != test.want || test.want == application.DisplayAvailable && annotation.Label.String() != "check-docs" || test.want != application.DisplayAvailable && annotation.Label.String() != "" {
				t.Fatalf("unsafe/incorrect annotation escaped: %+v want=%s", annotation, test.want)
			}
		})
	}
}

func TestDisplayRepositoryMissingAndNonTokenRowsRemainAdvisory(t *testing.T) {
	root := t.TempDir()
	if annotation := displayContractRead(t, root, displayContractToken); annotation.Availability != application.DisplayMissing {
		t.Fatalf("old row invented a label: %+v", annotation)
	}
	for _, token := range []string{"legacy-heavy", "../private", strings.Repeat("a", 31), strings.Repeat("A", 32)} {
		if annotation := displayContractRead(t, root, token); annotation.Availability != application.DisplayInvalid || annotation.Label.String() != "" {
			t.Fatalf("non-token row built an annotation path: %+v", annotation)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (evidence.DisplayRepository{}).ReadDisplay(ctx, root, []string{displayContractToken}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled annotation batch kept reading: %v", err)
	}
}

func TestDisplayRepositoryNeverFollowsSidecarOrDirectoryLinks(t *testing.T) {
	for _, directoryLink := range []bool{false, true} {
		root, outside := t.TempDir(), t.TempDir()
		label := displayContractLabel(t, "outside-context")
		if err := evidence.WriteDisplay(outside, displayContractToken, label); err != nil {
			t.Fatal(err)
		}
		outsidePath := filepath.Join(outside, "run-display", displayContractToken+".json")
		before, err := os.ReadFile(outsidePath)
		if err != nil {
			t.Fatal(err)
		}
		if directoryLink {
			err = os.Symlink(filepath.Join(outside, "run-display"), filepath.Join(root, "run-display"))
		} else {
			err = os.Mkdir(filepath.Join(root, "run-display"), 0o700)
			if err == nil {
				err = os.Symlink(outsidePath, filepath.Join(root, "run-display", displayContractToken+".json"))
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		annotations, readError := (evidence.DisplayRepository{}).ReadDisplay(t.Context(), root, []string{displayContractToken})
		if annotations[displayContractToken].Availability != application.DisplayUnreadable || annotations[displayContractToken].Label.String() != "" || directoryLink && readError == nil {
			t.Fatalf("link exposed outside label: %+v %v", annotations, readError)
		}
		if directoryLink && evidence.WriteDisplay(root, displayContractToken, label) == nil {
			t.Fatal("writer traversed directory link")
		}
		if evidence.RemoveDisplay(root, displayContractToken) == nil {
			t.Fatal("removal accepted linked sidecar namespace")
		}
		after, err := os.ReadFile(outsidePath)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("annotation operations changed link target: %v", err)
		}
	}
}

func TestDisplayRepositoryRejectsNonRegularWithoutBlocking(t *testing.T) {
	for _, kind := range []string{"directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "run-display"), 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "run-display", displayContractToken+".json")
			var err error
			if kind == "directory" {
				err = os.Mkdir(path, 0o700)
			} else {
				err = syscall.Mkfifo(path, 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if annotation := displayContractRead(t, root, displayContractToken); annotation.Availability != application.DisplayUnreadable || annotation.Label.String() != "" {
				t.Fatalf("non-regular record accepted: %+v", annotation)
			}
		})
	}
}

func TestDisplayRepositoryPrivateAtomicPublicationAndOwnedRemoval(t *testing.T) {
	root := t.TempDir()
	label := displayContractLabel(t, "check-docs")
	if err := evidence.WriteDisplay(root, displayContractToken, label); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Stat(filepath.Join(root, "run-display"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Stat(filepath.Join(root, "run-display", displayContractToken+".json"))
	if err != nil || directory.Mode().Perm() != 0o700 || file.Mode().Perm() != 0o600 || file.Size() > 512 {
		t.Fatalf("annotation modes/bound are not private: directory=%v file=%v error=%v", directory, file, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "run-display", displayContractToken+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil || len(fields) != 3 || fields["schemaVersion"] == nil || fields["runId"] == nil || fields["label"] == nil {
		t.Fatalf("sidecar expanded the declared wire shape: %s %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "run-display"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("atomic writer left temporary files: %v %v", entries, err)
	}
	if err = evidence.WriteDisplay(root, "../private", label); err == nil {
		t.Fatal("writer accepted path-like token")
	}
	if err = evidence.WriteDisplay(root, displayContractToken, identity.DisplayLabel{}); err == nil {
		t.Fatal("writer accepted unavailable label")
	}
	if err = evidence.RemoveDisplay(root, displayContractToken); err != nil {
		t.Fatal(err)
	}
	if annotation := displayContractRead(t, root, displayContractToken); annotation.Availability != application.DisplayMissing {
		t.Fatalf("owned annotation remained after removal: %+v", annotation)
	}
	if err = evidence.RemoveDisplay(root, displayContractToken); err != nil {
		t.Fatalf("repeated owned removal failed: %v", err)
	}
}

func TestDisplayRepositoryExpiredTemporariesRequireProtocolNameAndAge(t *testing.T) {
	root := t.TempDir()
	label := displayContractLabel(t, "check-docs")
	if err := evidence.WriteDisplay(root, displayContractToken, label); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	directory := filepath.Join(root, "run-display")
	oldName := ".run-display-" + displayContractToken + "-0123456789abcdef.tmp"
	youngName := ".run-display-" + displayContractToken + "-fedcba9876543210.tmp"
	unknownName := ".run-display-unknown.tmp"
	for _, name := range []string{oldName, youngName, unknownName} {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
		modified := now.Add(-2 * time.Hour)
		if name == youngName {
			modified = now.Add(-30 * time.Minute)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	if err := evidence.PruneDisplay(root, nil, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, oldName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired abandoned temporary survived: %v", err)
	}
	for _, name := range []string{youngName, unknownName, displayContractToken + ".json"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("unproven temporary/live annotation was pruned: %s %v", name, err)
		}
	}
}

func TestDisplayRepositoryConcurrentReadersSeeOnlyCompleteAtomicRecords(t *testing.T) {
	root := t.TempDir()
	first := displayContractLabel(t, "first-context")
	second := displayContractLabel(t, strings.Repeat("z", 64))
	if err := evidence.WriteDisplay(root, displayContractToken, first); err != nil {
		t.Fatal(err)
	}
	// The owned reader is joined by cleanup before its context is cancelled;
	// testing.T cancels its own context before running cleanup callbacks.
	ctx, cancel := context.WithCancel(context.Background())
	stop, ready := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		read := func() error {
			annotations, err := (evidence.DisplayRepository{}).ReadDisplay(ctx, root, []string{displayContractToken})
			annotation := annotations[displayContractToken]
			if err != nil || annotation.Availability != application.DisplayAvailable || annotation.Label != first && annotation.Label != second {
				return fmt.Errorf("atomic publication exposed incomplete annotation: %+v %w", annotation, err)
			}
			return nil
		}
		firstError := read()
		close(ready)
		if firstError != nil {
			finished <- firstError
			return
		}
		for {
			select {
			case <-stop:
				finished <- nil
				return
			default:
				if err := read(); err != nil {
					finished <- err
					return
				}
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		if err := <-finished; err != nil {
			t.Error(err)
		}
		cancel()
	})
	<-ready
	for range 25 {
		if err := evidence.WriteDisplay(root, displayContractToken, second); err != nil {
			t.Fatal(err)
		}
		if err := evidence.WriteDisplay(root, displayContractToken, first); err != nil {
			t.Fatal(err)
		}
	}
}
