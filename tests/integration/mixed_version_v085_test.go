//go:build mixedversion

package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	evidenceadapter "github.com/wahidyankf/hippo/internal/adapters/evidence"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// The full gate registers this genuine two-binary leaf explicitly with the
// mixedversion tag. Dedicated inputs leave its v0.8.4 corpus fixture untouched.
func TestMixedVersionV085DisplayInteroperability(t *testing.T) {
	if !t.Run("metadata oracle rejects altered declarations", mixedV085MetadataOracleControls) {
		return
	}
	if !t.Run("owned stream rejects foreign identity", func(t *testing.T) {
		started, ready := time.UnixMilli(1000), time.UnixMilli(2000)
		if !mixedV085OwnedStream("development-ephemeral-1500-42.jsonl", 42, started, ready) {
			t.Fatal("declared owner stream rejected")
		}
		for _, path := range []string{"development-ephemeral-1500-43.jsonl", "development-ephemeral-999-42.jsonl", "development-ephemeral-2001-42.jsonl", "nested/development-ephemeral-1500-42.jsonl", "other-1500-42.jsonl", "development-ephemeral-+1500-42.jsonl", "development-ephemeral-1500-42.summary.json"} {
			if mixedV085OwnedStream(path, 42, started, ready) {
				t.Fatalf("foreign sample stream accepted: %s", path)
			}
		}
	}) {
		return
	}
	old := os.Getenv("HIPPO_PUBLIC_V085_BIN")
	current := os.Getenv("HIPPO_BIN")
	oldDigest := mixedV085ExecutableDigest(t, old)
	if oldDigest != os.Getenv("HIPPO_PUBLIC_V085_BIN_SHA256") {
		t.Fatal("v0.8.5 executable does not match its supervisor-pinned digest")
	}
	currentDigest := mixedV085ExecutableDigest(t, current)
	if current == old || currentDigest == oldDigest {
		t.Fatal("mixed owners require distinct current and released executable bytes")
	}
	var release struct {
		SchemaVersion int    `json:"schemaVersion"`
		Version       string `json:"version"`
		Commit        string `json:"commit"`
	}
	// The supervisor must independently establish official archive provenance and
	// obtain this bounded raw receipt from the very binary whose hash is above.
	mixedV085Decode(t, mixedV085Read(t, os.Getenv("HIPPO_PUBLIC_V085_VERSION_RECEIPT"), 4096), &release)
	if release.SchemaVersion != 1 || release.Version != "v0.8.5" || release.Commit != "456d24bc2a20ee23a7b81746c1789133cd5c3a97" {
		t.Fatal("mixed leaf requires an actual verified v0.8.5 version receipt")
	}

	driver := support.NewDriver(contract.Adapter{Name: contract.E2E})
	driver.Reset()
	// t.Context is canceled before Cleanup; this finite owner context instead
	// remains alive until cleanup has observed natural command retirement.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	fixture := &mixedV085Fixture{driver: driver, cancel: cancel}
	t.Cleanup(func() { fixture.close(t) })
	var err error
	fixture.root, fixture.directory, err = driver.PreparePublicTUIRoot()
	if err != nil {
		t.Fatal(err)
	}
	// Explicit identity keeps upward repository discovery out of both binaries.
	fixture.identityPath = filepath.Join(fixture.directory, "hippo.identity.json")
	if err := os.WriteFile(fixture.identityPath, []byte(`{"schemaVersion":1,"source":"mixed-v085-fixture","tags":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.ledger, err = driver.NewPublicTUILedger(ctx, fixture.root, fixture.directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.ledger.PrepareObservation(ctx, "mixed", ""); err != nil {
		t.Fatal(err)
	}
	oldOwner := fixture.start(t, ctx, old, false)
	schemas, err := fixture.ledger.ProtocolSchemas()
	if err != nil {
		t.Fatal(err)
	}
	currentOwner := fixture.start(t, ctx, current, true)
	fixture.requireSchemas(t, schemas)
	fixture.requireLabels(t, ctx, oldOwner.token, currentOwner.token, "current-owner")
	fixture.requireDeclaredOwnerMetadata(t, ctx)
	queuedStarted := time.Now()
	if err := fixture.ledger.AdoptMixedOwners(ctx, []string{oldOwner.token, currentOwner.token}, []int{oldOwner.command.Process.Pid, currentOwner.command.Process.Pid}); err != nil {
		t.Fatal(err)
	}
	fixture.prepareExpectedMetadata(t, queuedStarted, time.Now())
	// The actual current run publishes its label before this test touches it.
	fixture.requireLabels(t, ctx, oldOwner.token, currentOwner.token, "current-owner")
	fixture.requireTransitions(t, ctx, old, current, oldOwner.token, currentOwner.token)
	// Restore the owned annotation so natural current-owner retirement itself
	// must remove it; removal above must not be mistaken for cleanup evidence.
	label, err := identity.ParseDisplayLabel("retirement-owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := evidenceadapter.WriteDisplay(fixture.root, currentOwner.token, label); err != nil {
		t.Fatal(err)
	}
	fixture.requireLabels(t, ctx, oldOwner.token, currentOwner.token, "retirement-owner")
	if mixedV085ExecutableDigest(t, old) != oldDigest || mixedV085ExecutableDigest(t, current) != currentDigest {
		t.Fatal("selected executable bytes changed during genuine mixed interoperability")
	}
}

type mixedV085Fixture struct {
	driver          *support.Driver
	cancel          context.CancelFunc
	ledger          *support.PublicTUILedger
	root, directory string
	identityPath    string
	expected        coordination.ReservationTotals
	jobs            []*mixedV085Owner
	probes          []*support.PublicTUIHostProbe
}

type mixedV085Owner struct {
	command             *exec.Cmd
	cancel              context.CancelFunc
	done                chan struct{}
	release, token      string
	payloadPID          int
	startedAt, readyAt  time.Time
	output, diagnostics mixedV085Capture
	waitError           error
}

func (fixture *mixedV085Fixture) probe(ctx context.Context, t *testing.T) *support.PublicTUIHostProbe {
	t.Helper()
	helper := support.PublicTUIHostHelper{Executable: os.Getenv("HIPPO_PUBLIC_HOST_HELPER"), SHA256: os.Getenv("HIPPO_PUBLIC_HOST_HELPER_SHA256")}
	probe, err := fixture.driver.NewPublicTUIHostProbeWithHelper(ctx, fixture.directory, support.PublicTUIHostReply{CPUPercent: 20}, helper)
	if err != nil {
		t.Fatal(err)
	}
	fixture.probes = append(fixture.probes, probe)
	return probe
}

func (fixture *mixedV085Fixture) environment(probe *support.PublicTUIHostProbe) []string {
	return probe.Environment([]string{"HIPPO_ROOT=" + fixture.root, "HIPPO_IDENTITY=" + fixture.identityPath, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "NO_COLOR=1", "GOMAXPROCS=2"})
}

func (fixture *mixedV085Fixture) start(t *testing.T, parent context.Context, executable string, current bool) *mixedV085Owner {
	t.Helper()
	directory, err := os.MkdirTemp(fixture.directory, "mixed-v085-owner-")
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(directory, "ready")
	ctx, cancel := context.WithCancel(parent)
	job := &mixedV085Owner{cancel: cancel, done: make(chan struct{}), release: filepath.Join(directory, "release")}
	fixture.jobs = append(fixture.jobs, job)
	probe := fixture.probe(ctx, t)
	arguments := []string{"run", "--config", fixture.ledger.Configuration(), "--cwd", directory, "--disk-path", directory, "--profile", "balanced", "--class", "ephemeral", "--resource-tier", "light", "--source", "owner", "--tag", "role=owner", "--reserve-cpu", "2", "--reserve-memory-mib", "1024"}
	if current {
		arguments = append(arguments, "--label", "current-owner")
	}
	script := `trap 'exit 130' TERM; printf '%s' "$$" > "$PUBLIC_OWNER_READY.pid"; printf '%s' "$HIPPO_SESSION" > "$PUBLIC_OWNER_READY.tmp"; /bin/mv "$PUBLIC_OWNER_READY.tmp" "$PUBLIC_OWNER_READY"; while [ ! -e "$PUBLIC_OWNER_RELEASE" ]; do /bin/sleep 0.01; done; exit 0`
	arguments = append(arguments, "--", "/bin/sh", "-c", script)
	// Both owners use their explicitly selected real binaries. No TestMain build,
	// integration model, old-version relabeling, or E2E fallback can select them.
	job.command = exec.CommandContext(ctx, executable, arguments...)
	job.command.Dir = directory
	job.command.Env = append(fixture.environment(probe), "PUBLIC_OWNER_READY="+ready, "PUBLIC_OWNER_RELEASE="+job.release)
	job.command.Stdin = strings.NewReader("")
	job.command.Stdout, job.command.Stderr = &job.output, &job.diagnostics
	job.command.WaitDelay = 3 * time.Second
	job.command.Cancel = func() error { return job.command.Process.Signal(os.Interrupt) }
	job.startedAt = time.Now()
	if err := job.command.Start(); err != nil {
		close(job.done)
		t.Fatal(err)
	}
	go func() {
		defer close(job.done)
		job.waitError = job.command.Wait()
	}()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(ready)
		if err == nil {
			job.token = string(data)
			if len(job.token) != 32 {
				t.Fatal("real owner did not publish a complete session identity")
			}
			if _, err := hex.DecodeString(job.token); err != nil {
				t.Fatal(err)
			}
			job.payloadPID, err = strconv.Atoi(string(mixedV085Read(t, ready+".pid", 32)))
			if err != nil || job.payloadPID <= 0 || syscall.Kill(job.payloadPID, 0) != nil {
				t.Fatal("real owner's payload PID is not live")
			}
			job.readyAt = time.Now()
			return job
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-job.done:
			t.Fatalf("owner ended before readiness: Wait=%v stdout=%q stderr=%q", job.waitError, job.output.Bytes(), job.diagnostics.Bytes())
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

func (fixture *mixedV085Fixture) requireDeclaredOwnerMetadata(t *testing.T, ctx context.Context) {
	t.Helper()
	actual, err := runtimeadapter.NewEngine().Snapshot(ctx, fixture.root, "reservation")
	if err != nil || len(actual.Owners) != len(fixture.jobs) {
		t.Fatalf("declared mixed owners are not the actual live rows: %+v error=%v", actual, err)
	}
	// Independent launch inputs, not values copied from the snapshot adopted
	// below: explicit overrides fix both light-tier bounds at two CPUs/one GiB.
	vector := coordination.ReservationVector{CPU: 2, MemoryBytes: 1024 * 1024 * 1024}
	for _, job := range fixture.jobs {
		found := false
		for _, row := range actual.Owners {
			if row.RunID != job.token {
				continue
			}
			found = true
			if row.Source != "owner" || !reflect.DeepEqual(row.Tags, map[string]string{"role": "owner"}) || row.Tier != "light" || row.Minimum != vector || row.Maximum != vector || row.Legacy {
				t.Fatalf("real owner lost declared source/tags/light-tier metadata: %+v", row)
			}
			registered, registrationError := time.Parse(time.RFC3339Nano, row.RegisteredAt)
			deadline, deadlineError := time.Parse(time.RFC3339Nano, row.Deadline)
			if registrationError != nil || deadlineError != nil || registered.Before(job.startedAt) || registered.After(job.readyAt) || deadline.Sub(registered) != 30*time.Minute {
				t.Fatalf("real owner registration/deadline differs from its actual launch and declared light queue window: %+v launch=%s ready=%s", row, job.startedAt, job.readyAt)
			}
		}
		if !found {
			t.Fatalf("real owner token %s is absent before adopting the preservation baseline", job.token)
		}
	}
}

// This is the persisted metadata contract shared by v0.8.5 and current. The
// disk document is decoded separately from either selected status reader.
type mixedV085Metadata struct {
	SchemaVersion int                            `json:"schemaVersion"`
	RunID         string                         `json:"runId"`
	Source        string                         `json:"source"`
	Tags          map[string]string              `json:"tags,omitempty"`
	Tier          string                         `json:"tier,omitempty"`
	Minimum       coordination.ReservationVector `json:"minimum"`
	Maximum       coordination.ReservationVector `json:"maximum"`
	RegisteredAt  string                         `json:"registeredAt"`
	Deadline      string                         `json:"deadline"`
}

func mixedV085ValidateMetadata(actual mixedV085Metadata, declared coordination.ReservationEntry, started, ready time.Time, window time.Duration) error {
	if actual.SchemaVersion != 1 || actual.RunID != declared.RunID || actual.Source != declared.Source || !reflect.DeepEqual(actual.Tags, declared.Tags) || actual.Tier != declared.Tier || actual.Minimum != declared.Minimum || actual.Maximum != declared.Maximum {
		return errors.New("mixed disk metadata differs from declared identity/tier/bounds")
	}
	registered, registrationError := time.Parse(time.RFC3339Nano, actual.RegisteredAt)
	deadline, deadlineError := time.Parse(time.RFC3339Nano, actual.Deadline)
	if registrationError != nil || deadlineError != nil || registered.Before(started) || registered.After(ready) || deadline.Sub(registered) != window {
		return errors.New("mixed disk registration/deadline differs from declared launch window")
	}
	return nil
}

func (fixture *mixedV085Fixture) prepareExpectedMetadata(t *testing.T, queuedStarted, queuedReady time.Time) {
	t.Helper()
	fixture.expected = fixture.ledger.Expected()
	if len(fixture.expected.AbandonedProcessGroups) != 0 {
		t.Fatal("live mixed fixture contains abandoned payload groups")
	}
	// Schema-five omits an empty abandonedProcessGroups list on the wire.
	fixture.expected.AbandonedProcessGroups = nil
	vector := coordination.ReservationVector{CPU: 2, MemoryBytes: 1024 * 1024 * 1024}
	for group, rows := range [][]coordination.ReservationEntry{fixture.expected.Owners, fixture.expected.Waiters} {
		for index := range rows {
			row := &rows[index]
			source, tier, window := "other", "", 30*time.Second
			started, ready := queuedStarted, queuedReady
			if group == 0 {
				source, tier, window = "owner", "light", 30*time.Minute
				found := false
				for _, job := range fixture.jobs {
					if job.token == row.RunID {
						started, ready, found = job.startedAt, job.readyAt, true
					}
				}
				if !found {
					t.Fatal("disk metadata owner has no actual launch/readiness identity")
				}
			}
			declared := coordination.ReservationEntry{RunID: row.RunID, Source: source, Tags: map[string]string{"role": source}, Tier: tier, Minimum: vector, Maximum: vector}
			var metadata mixedV085Metadata
			mixedV085Decode(t, mixedV085Read(t, filepath.Join(fixture.root, "owner-metadata", row.RunID+".json"), 4096), &metadata)
			if err := mixedV085ValidateMetadata(metadata, declared, started, ready, window); err != nil {
				t.Fatalf("neutral disk metadata row %s: %v", row.RunID, err)
			}
			// Fill only the fields missing from the shared helper's queued oracle.
			// Bounds/tier are declared inputs; clock values come from validated disk,
			// never from the current snapshot or either binary's status response.
			row.Tier, row.Minimum, row.Maximum = tier, vector, vector
			row.RegisteredAt, row.Deadline = metadata.RegisteredAt, metadata.Deadline
		}
	}
	fixture.requireWaiterWindow(t)
}

func (fixture *mixedV085Fixture) requireWaiterWindow(t *testing.T) {
	t.Helper()
	for _, row := range fixture.expected.Waiters {
		deadline, err := time.Parse(time.RFC3339Nano, row.Deadline)
		if err != nil || !time.Now().Before(deadline) {
			t.Fatalf("mixed reader transition outlived the declared 30-second FIFO waiter window: %s", row.RunID)
		}
	}
}

func mixedV085MetadataOracleControls(t *testing.T) {
	t.Helper()
	started := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	vector := coordination.ReservationVector{CPU: 2, MemoryBytes: 1024 * 1024 * 1024}
	for _, source := range []string{"owner", "other"} {
		t.Run(source, func(t *testing.T) {
			tier, window := "", 30*time.Second
			if source == "owner" {
				tier, window = "light", 30*time.Minute
			}
			declared := coordination.ReservationEntry{RunID: strings.Repeat("a", 32), Source: source, Tags: map[string]string{"role": source}, Tier: tier, Minimum: vector, Maximum: vector}
			valid := mixedV085Metadata{SchemaVersion: 1, RunID: declared.RunID, Source: source, Tags: declared.Tags, Tier: tier, Minimum: vector, Maximum: vector, RegisteredAt: started.Format(time.RFC3339Nano), Deadline: started.Add(window).Format(time.RFC3339Nano)}
			if err := mixedV085ValidateMetadata(valid, declared, started, started.Add(time.Second), window); err != nil {
				t.Fatal(err)
			}
			for _, fault := range []string{"schema", "identity", "source", "tags", "tier", "minimum", "maximum", "registration", "late registration", "invalid registration", "deadline", "invalid deadline"} {
				t.Run(fault, func(t *testing.T) {
					bad := valid
					switch fault {
					case "schema":
						bad.SchemaVersion = 2
					case "identity":
						bad.RunID = strings.Repeat("b", 32)
					case "source":
						bad.Source = "wrong"
					case "tags":
						bad.Tags = map[string]string{"role": "wrong"}
					case "tier":
						bad.Tier = "heavy"
					case "minimum":
						bad.Minimum.CPU++
					case "maximum":
						bad.Maximum.MemoryBytes++
					case "registration":
						bad.RegisteredAt = started.Add(-time.Second).Format(time.RFC3339Nano)
					case "late registration":
						bad.RegisteredAt = started.Add(2 * time.Second).Format(time.RFC3339Nano)
						bad.Deadline = started.Add(2*time.Second + window).Format(time.RFC3339Nano)
					case "invalid registration":
						bad.RegisteredAt = "not-a-time"
					case "deadline":
						bad.Deadline = started.Add(window + time.Second).Format(time.RFC3339Nano)
					case "invalid deadline":
						bad.Deadline = "not-a-time"
					}
					if err := mixedV085ValidateMetadata(bad, declared, started, started.Add(time.Second), window); err == nil {
						t.Fatal("altered neutral metadata accepted as a complete reader oracle")
					}
				})
			}
		})
	}
}

func (fixture *mixedV085Fixture) requireStatus(t *testing.T, parent context.Context, executable string) {
	t.Helper()
	fixture.requireWaiterWindow(t)
	probe := fixture.probe(parent, t)
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "status", "--json", "--config", fixture.ledger.Configuration(), "--disk-path", fixture.directory)
	command.Dir, command.Env = fixture.directory, fixture.environment(probe)
	command.Stdin = strings.NewReader("")
	output, diagnostics := &mixedV085Capture{}, &mixedV085Capture{}
	command.Stdout, command.Stderr = output, diagnostics
	command.WaitDelay = 3 * time.Second
	if err := command.Run(); err != nil {
		t.Fatalf("real status reader failed: %v stdout=%q stderr=%q", err, output.Bytes(), diagnostics.Bytes())
	}
	if command.ProcessState == nil || command.ProcessState.ExitCode() != 0 || diagnostics.Len() != 0 {
		t.Fatal("real status reader lacks a natural zero-exit Wait with clean diagnostics")
	}
	var actual application.StatusView
	mixedV085Decode(t, output.Bytes(), &actual)
	if actual.SchemaVersion != 5 || actual.MeasuredAt == "" || actual.AvailableParallelism < 1 {
		t.Fatal("real status reader lost schema-five or host-reading evidence")
	}
	if err := fixture.ledger.RequirePublicTotals(actual.Coordination); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual.Coordination, fixture.expected) {
		difference := fmt.Sprintf("actual=%+v expected=%+v", actual.Coordination, fixture.expected)
		if len(difference) > 16384 {
			difference = difference[:16384] + " [bounded]"
		}
		t.Fatalf("real strict reader changed complete row identity, tier bounds or registration metadata binary=%s %s", executable, difference)
	}
	fixture.requireWaiterWindow(t)
	trace := probe.Trace()
	if trace.Readings != 2 || trace.Active != 0 || trace.MaximumActive != 1 {
		t.Fatalf("real status reader did not complete two serialized collector readings: %+v", trace)
	}
	t.Logf("mixed real status binary=%s pid=%d exit=%d coordination=%+v collector=%+v", executable, command.Process.Pid, command.ProcessState.ExitCode(), actual.Coordination, trace)
}

func (fixture *mixedV085Fixture) requireTransitions(t *testing.T, ctx context.Context, old, current, oldToken, currentToken string) {
	t.Helper()
	schemas, err := fixture.ledger.ProtocolSchemas()
	if err != nil {
		t.Fatal(err)
	}
	before, err := fixture.ledger.CoordinationTree()
	if err != nil {
		t.Fatal(err)
	}
	// Each independently launched live owner appends samples in this same root.
	// Qualify exactly its PID/time-bound stream; unknown paths remain immutable.
	streams := map[string]os.FileInfo{}
	for _, job := range fixture.jobs {
		found := 0
		for path := range before {
			if !mixedV085OwnedStream(path, job.command.Process.Pid, job.startedAt, job.readyAt) {
				continue
			}
			info, err := os.Lstat(filepath.Join(fixture.root, path))
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("owned live sample stream is not a regular file: %s %v", path, err)
			}
			streams[path] = info
			found++
		}
		if found != 1 {
			t.Fatalf("live owner pid=%d has %d qualified sample streams, want one", job.command.Process.Pid, found)
		}
	}
	if len(streams) != 2 {
		t.Fatal("mixed live evidence projection requires exactly two distinct owned streams")
	}
	// Labels change only their sidecar; every persisted coordination and metadata
	// byte, FIFO position and vector must remain intact through both real readers.
	for _, label := range []string{"current-owner", "changed-owner", ""} {
		if label == "changed-owner" {
			parsed, err := identity.ParseDisplayLabel(label)
			if err != nil {
				t.Fatal(err)
			}
			if err := evidenceadapter.WriteDisplay(fixture.root, currentToken, parsed); err != nil {
				t.Fatal(err)
			}
		} else if label == "" {
			if err := evidenceadapter.RemoveDisplay(fixture.root, currentToken); err != nil {
				t.Fatal(err)
			}
		}
		fixture.requireLabels(t, ctx, oldToken, currentToken, label)
		for _, executable := range []string{old, current} {
			fixture.requireStatus(t, ctx, executable)
		}
		fixture.requireSchemas(t, schemas)
		after, err := fixture.ledger.CoordinationTree()
		if err != nil {
			t.Fatal(err)
		}
		for path, content := range before {
			if original, owned := streams[path]; owned {
				info, err := os.Lstat(filepath.Join(fixture.root, path))
				current, present := after[path]
				if err != nil || !present || !info.Mode().IsRegular() || info.Mode() != original.Mode() || !os.SameFile(original, info) || !strings.HasPrefix(current, content) {
					t.Fatalf("live owner sample stream lost ownership or append-only bytes: %s", path)
				}
				before[path] = current
				continue
			}
			if !strings.HasPrefix(path, "run-display/") && after[path] != content {
				t.Fatalf("display transition changed persisted coordination/metadata %s", path)
			}
		}
		for path := range after {
			if !strings.HasPrefix(path, "run-display/") {
				if _, existed := before[path]; !existed {
					t.Fatalf("display transition added persisted coordination/metadata %s", path)
				}
			}
		}
	}
}

// The sample path is declared by the actual process identity and launch window,
// rather than by a filename extension or the path that happened to change.
func mixedV085OwnedStream(path string, pid int, started, ready time.Time) bool {
	if filepath.Base(path) != path {
		return false
	}
	epoch, prefix := strings.CutPrefix(path, "development-ephemeral-")
	epoch, suffix := strings.CutSuffix(epoch, "-"+strconv.Itoa(pid)+".jsonl")
	milliseconds, err := strconv.ParseInt(epoch, 10, 64)
	return prefix && suffix && err == nil && strconv.FormatInt(milliseconds, 10) == epoch && milliseconds >= started.UnixMilli() && milliseconds <= ready.UnixMilli()
}

func (fixture *mixedV085Fixture) requireSchemas(t *testing.T, expected map[string]int) {
	t.Helper()
	actual, err := fixture.ledger.ProtocolSchemas()
	if err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("mixed writer changed actual persisted schemas: before=%v after=%v error=%v", expected, actual, err)
	}
}

func (fixture *mixedV085Fixture) requireLabels(t *testing.T, ctx context.Context, old, current, label string) {
	t.Helper()
	annotations, err := (evidenceadapter.DisplayRepository{}).ReadDisplay(ctx, fixture.root, []string{old, current})
	if err != nil || annotations[old].Availability != application.DisplayMissing {
		t.Fatalf("old owner acquired display context: %+v %v", annotations, err)
	}
	want := application.DisplayAvailable
	if label == "" {
		want = application.DisplayMissing
	}
	if annotations[current].Availability != want || annotations[current].Label.String() != label {
		t.Fatalf("real display reader missed sidecar transition: %+v want=%q", annotations[current], label)
	}
}

func (fixture *mixedV085Fixture) close(t *testing.T) {
	t.Helper()
	defer fixture.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	var result error
	queuedJoined := true
	if fixture.ledger != nil {
		result = fixture.ledger.Close(ctx)
		queuedJoined = fixture.ledger.QueuedActorsJoined()
	}
	// Cancel/join actual waiters first. Releasing both ordinary children then
	// proves natural retirement without accidentally admitting a queued actor.
	for _, job := range fixture.jobs {
		if job.command == nil || job.command.Process == nil {
			job.cancel()
			continue
		}
		if queuedJoined {
			if err := os.WriteFile(job.release, nil, 0o600); err != nil {
				result = errors.Join(result, err)
				job.cancel()
			}
		} else {
			job.cancel()
		}
	}
	natural, stop := context.WithTimeout(ctx, 3*time.Second)
	for _, job := range fixture.jobs {
		if job.command == nil || job.command.Process == nil {
			continue
		}
		select {
		case <-job.done:
		case <-natural.Done():
			result = errors.Join(result, errors.New("mixed owner missed natural-release budget"))
		}
	}
	stop()
	for _, job := range fixture.jobs {
		job.cancel()
	}
	allJoined := true
	for _, job := range fixture.jobs {
		if job.command == nil || job.command.Process == nil {
			continue
		}
		select {
		case <-job.done:
			if job.waitError != nil || job.command.ProcessState == nil || job.command.ProcessState.ExitCode() != 0 {
				result = errors.Join(result, fmt.Errorf("mixed owner did not naturally exit zero: %v", job.waitError))
			}
			if !errors.Is(syscall.Kill(job.payloadPID, 0), syscall.ESRCH) {
				result = errors.Join(result, errors.New("mixed payload survived actual joined Wait"))
			}
			for _, relative := range []string{"owner-metadata", "run-display"} {
				if _, err := os.Lstat(filepath.Join(fixture.root, relative, job.token+".json")); !errors.Is(err, os.ErrNotExist) {
					result = errors.Join(result, fmt.Errorf("mixed owned %s survived retirement: %v", relative, err))
				}
			}
			t.Logf("mixed owner retired pid=%d payloadPID=%d token=%s Wait=%v stdout=%q stderr=%q", job.command.Process.Pid, job.payloadPID, job.token, job.waitError, job.output.Bytes(), job.diagnostics.Bytes())
		case <-ctx.Done():
			allJoined = false
			result = errors.Join(result, errors.New("mixed owner actual Wait remains unjoined"))
		}
	}
	// Finish BOTH owners' raw absence checks above before the first snapshot:
	// reconciliation for either token can prune the other owner's leftovers.
	if allJoined {
		for _, job := range fixture.jobs {
			if job.command != nil && job.command.Process != nil {
				result = errors.Join(result, fixture.ledger.RequireOwnerRetired(ctx, job.token))
			}
		}
	}
	for _, probe := range fixture.probes {
		result = errors.Join(result, probe.Close(ctx))
		trace := probe.Trace()
		if !trace.Closed || trace.Active != 0 {
			allJoined = false
			result = errors.Join(result, errors.New("mixed collector fixture remains active or unjoined"))
		}
	}
	// A failed ownership join retains paths for the outer supervisor; never
	// delete an owned directory while a command or collector still uses it.
	if allJoined && queuedJoined {
		if fixture.ledger != nil {
			actual, err := runtimeadapter.NewEngine().Snapshot(ctx, fixture.root, "reservation")
			if err != nil || actual.ActiveOwners != 0 || actual.WaitingOwners != 0 || len(actual.Owners) != 0 || len(actual.Waiters) != 0 || actual.Allocated != (coordination.ReservationVector{}) || actual.Waiting != (coordination.ReservationVector{}) {
				result = errors.Join(result, fmt.Errorf("mixed cleanup left live ownership or reservation demand: %+v error=%v", actual, err))
			}
		}
		fixture.driver.Close()
		if fixture.directory != "" {
			if _, err := os.Lstat(fixture.directory); !errors.Is(err, os.ErrNotExist) {
				result = errors.Join(result, errors.New("mixed owned directory survived joined cleanup"))
			}
		}
	}
	if result != nil {
		t.Error(result)
	}
}

func mixedV085ExecutableDigest(t *testing.T, path string) string {
	t.Helper()
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		t.Fatal("mixed executable requires a clean absolute path")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("mixed executable must be regular and executable: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	_, readError := io.Copy(hash, file)
	if err := errors.Join(readError, file.Close()); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func mixedV085Read(t *testing.T, path string, maximum int64) []byte {
	t.Helper()
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		t.Fatal("mixed receipt requires a clean absolute path")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("mixed receipt must be a regular file: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	data, readError := io.ReadAll(io.LimitReader(file, maximum+1))
	if err := errors.Join(readError, file.Close()); err != nil {
		t.Fatal(err)
	}
	if int64(len(data)) > maximum {
		t.Fatal("mixed receipt exceeds its byte bound")
	}
	return data
}

func mixedV085Decode(t *testing.T, data []byte, destination any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatal("mixed reader requires exactly one JSON value")
	}
}

type mixedV085Capture struct{ buffer bytes.Buffer }

func (capture *mixedV085Capture) Bytes() []byte { return capture.buffer.Bytes() }
func (capture *mixedV085Capture) Len() int      { return capture.buffer.Len() }

func (capture *mixedV085Capture) Write(data []byte) (int, error) {
	if len(data) > 8*1024*1024-capture.Len() {
		return 0, errors.New("mixed command stream exceeds eight MiB")
	}
	return capture.buffer.Write(data)
}
