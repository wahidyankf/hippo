//go:build mixedversion

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/support"
)

// The released v0.10.0 binary is the one tests/e2e/v0100-release.sh verifies
// against tests/e2e/hippo-v0100.lock. This leaf pins the compatibility hazards of
// the optional very-heavy tier and of coordination.countSwap: a v0.10.0 reader
// refuses a file that declares either, and a very-heavy owner written by this
// build is still listed by v0.10.0.
const (
	mixedV0100Timeout          = 120 * time.Second
	mixedV0100PollInterval     = 10 * time.Millisecond
	mixedV0100RefusedCode      = 125
	mixedV0100Unreadable       = "hippo.config.unreadable"
	mixedV0100UnknownTier      = "schema 3 contains an unknown resource tier"
	mixedV0100UnknownCountSwap = `json: unknown field "countSwap"`
	mixedV0100OwnerTierText    = "tier=very-heavy"
)

// mixedV0100SharedDocument is a schema-3 file that both releases read: three tiers, one pool.
const mixedV0100SharedDocument = `{"schemaVersion":3,"coordination":{"maxCpu":2,"maxMemoryMiB":1024,"baseActiveOwners":2,"maxActiveOwners":3,` +
	`"promotion":{"completedRuns":1,"minimumSources":1,"minimumAvailableMemoryMiB":256,"maximumCpuP95Percent":98},` +
	`"emergencyAvailableMemoryMiB":256,"tiers":{` +
	`"light":{"minimumCpu":1,"maximumCpu":1,"minimumMemoryMiB":256,"maximumMemoryMiB":256,"queueDeadline":"30m"},` +
	`"standard":{"minimumCpu":1,"maximumCpu":1,"minimumMemoryMiB":256,"maximumMemoryMiB":256,"queueDeadline":"90m"},` +
	`"heavy":{"minimumCpu":1,"maximumCpu":2,"minimumMemoryMiB":256,"maximumMemoryMiB":512,"queueDeadline":"4h"}}}}`

// mixedV0100DeclaredDocument is the same file with the optional very-heavy tier declared.
const mixedV0100DeclaredDocument = `{"schemaVersion":3,"coordination":{"maxCpu":2,"maxMemoryMiB":1024,"baseActiveOwners":2,"maxActiveOwners":3,` +
	`"promotion":{"completedRuns":1,"minimumSources":1,"minimumAvailableMemoryMiB":256,"maximumCpuP95Percent":98},` +
	`"emergencyAvailableMemoryMiB":256,"tiers":{` +
	`"light":{"minimumCpu":1,"maximumCpu":1,"minimumMemoryMiB":256,"maximumMemoryMiB":256,"queueDeadline":"30m"},` +
	`"standard":{"minimumCpu":1,"maximumCpu":1,"minimumMemoryMiB":256,"maximumMemoryMiB":256,"queueDeadline":"90m"},` +
	`"heavy":{"minimumCpu":1,"maximumCpu":2,"minimumMemoryMiB":256,"maximumMemoryMiB":512,"queueDeadline":"4h"},` +
	`"very-heavy":{"minimumCpu":1,"maximumCpu":2,"minimumMemoryMiB":256,"maximumMemoryMiB":512,"queueDeadline":"4h"}}}}`

// mixedV0100CountSwapDocument is the shared file with coordination.countSwap declared. The v0.10.0 reader
// refuses the whole file for that unknown key, so the setting is the only difference from the shared file.
var mixedV0100CountSwapDocument = strings.Replace(mixedV0100SharedDocument, `"coordination":{`, `"coordination":{"countSwap":true,`, 1)

// TestMixedVersionV0100VeryHeavyCompatibility runs the released binary beside the
// stamped candidate over one shared evidence root.
func TestMixedVersionV0100VeryHeavyCompatibility(t *testing.T) {
	released := os.Getenv("HIPPO_PUBLIC_V0100_BIN")
	if released == "" || os.Getenv("HIPPO_PUBLIC_V0100_BIN_SHA256") != mixedV085ExecutableDigest(t, released) {
		t.Fatal("v0.10.0 release executable is not the verified one prepared for this leaf")
	}
	candidate := os.Getenv("HIPPO_BIN")
	if candidate == "" {
		t.Fatal("HIPPO_BIN must name the stamped current candidate")
	}
	fixture := newMixedV0100Fixture(t, released, candidate)
	if !t.Run("released binary refuses a file that declares very-heavy", fixture.refusesDeclaration) {
		return
	}
	t.Run("released binary lists a very-heavy owner written by this build", fixture.listsVeryHeavyOwner)
	t.Run("released binary refuses a file that declares countSwap", fixture.refusesCountSwap)
	t.Run("current build accepts a file that declares countSwap", fixture.acceptsCountSwap)
}

type mixedV0100Fixture struct {
	released, candidate string
	root, work          string
	sharedConfig        string
	declaredConfig      string
	countSwapConfig     string
	environment         []string
	runContext          context.Context
}

type mixedV0100Result struct {
	stdout, stderr string
	code           int
}

func newMixedV0100Fixture(t *testing.T, released, candidate string) *mixedV0100Fixture {
	t.Helper()
	directory := t.TempDir()
	fixture := &mixedV0100Fixture{
		released: released, candidate: candidate,
		root: filepath.Join(directory, "root"), work: filepath.Join(directory, "work"),
		sharedConfig:    filepath.Join(directory, "shared.json"),
		declaredConfig:  filepath.Join(directory, "declared.json"),
		countSwapConfig: filepath.Join(directory, "countswap.json"),
	}
	for _, path := range []string{fixture.root, fixture.work} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, document := range map[string]string{
		fixture.sharedConfig:    mixedV0100SharedDocument,
		fixture.declaredConfig:  mixedV0100DeclaredDocument,
		fixture.countSwapConfig: mixedV0100CountSwapDocument,
	} {
		if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Every invocation samples the fixed quiet host, as the compiled-binary runner does: the owner's
	// admission reads the runner's CPU, which parallel test packages load, so a live host defers it.
	environment, releaseHost, err := support.QuietHostEnvironment([]string{"HIPPO_ROOT=" + fixture.root, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "NO_COLOR=1"})
	if err != nil {
		t.Fatalf("publish the fixed host evidence: %v", err)
	}
	t.Cleanup(releaseHost)
	fixture.environment = environment
	var cancel context.CancelFunc
	fixture.runContext, cancel = context.WithTimeout(context.Background(), mixedV0100Timeout)
	t.Cleanup(cancel)

	return fixture
}

// invoke runs one binary over the shared root with no inherited configuration.
func (fixture *mixedV0100Fixture) invoke(t *testing.T, binary string, arguments ...string) mixedV0100Result {
	t.Helper()
	command := exec.CommandContext(fixture.runContext, binary, arguments...)
	command.Dir = fixture.work
	command.Env = fixture.environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	result := mixedV0100Result{}
	err := command.Run()
	var exitError *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitError):
		result.code = exitError.ExitCode()
	default:
		t.Fatal(err)
	}
	result.stdout, result.stderr = stdout.String(), stderr.String()

	return result
}

// refusesDeclaration pins the hazard: the released reader refuses the declaring file with exit 125.
func (fixture *mixedV0100Fixture) refusesDeclaration(t *testing.T) {
	result := fixture.invoke(t, fixture.released, "status", "--json", "--config", fixture.declaredConfig, "--disk-path", fixture.work)
	if result.code != mixedV0100RefusedCode || !strings.Contains(result.stderr, mixedV0100Unreadable) ||
		!strings.Contains(result.stderr, mixedV0100UnknownTier) {
		t.Fatalf("released status exit=%d stderr=%q, want %d naming %s and %q",
			result.code, result.stderr, mixedV0100RefusedCode, mixedV0100Unreadable, mixedV0100UnknownTier)
	}
}

// refusesCountSwap pins the countSwap hazard: the released reader refuses the file, because it names a key it does
// not know, with exit 125 and the unknown-field text.
func (fixture *mixedV0100Fixture) refusesCountSwap(t *testing.T) {
	result := fixture.invoke(t, fixture.released, "status", "--json", "--config", fixture.countSwapConfig, "--disk-path", fixture.work)
	if result.code != mixedV0100RefusedCode || !strings.Contains(result.stderr, mixedV0100Unreadable) ||
		!strings.Contains(result.stderr, mixedV0100UnknownCountSwap) {
		t.Fatalf("released status exit=%d stderr=%q, want %d naming %s and %q",
			result.code, result.stderr, mixedV0100RefusedCode, mixedV0100Unreadable, mixedV0100UnknownCountSwap)
	}
}

// acceptsCountSwap requires the current build to read the same file and report a schema-five status.
func (fixture *mixedV0100Fixture) acceptsCountSwap(t *testing.T) {
	result := fixture.invoke(t, fixture.candidate, "status", "--json", "--config", fixture.countSwapConfig, "--disk-path", fixture.work)
	var view struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if result.code != 0 || strings.Contains(result.stderr, mixedV0100Unreadable) || json.Unmarshal([]byte(result.stdout), &view) != nil || view.SchemaVersion != 5 {
		t.Fatalf("current status exit=%d stderr=%q stdout=%q, want exit 0 and a schema-five status", result.code, result.stderr, result.stdout)
	}
}

// listsVeryHeavyOwner starts a very-heavy owner with this build, waits until its
// payload is running, then requires the released status to list it. The payload
// exits when the release file appears, and the deferred release covers every path.
func (fixture *mixedV0100Fixture) listsVeryHeavyOwner(t *testing.T) {
	ready := filepath.Join(fixture.work, "owner-ready")
	release := filepath.Join(fixture.work, "owner-release")
	script := fmt.Sprintf(": > '%s'; while [ ! -e '%s' ]; do /bin/sleep 0.05; done", ready, release)
	owner := exec.CommandContext(fixture.runContext, fixture.candidate,
		"run", "--config", fixture.declaredConfig, "--source", "mixed-v0100", "--class", "ephemeral",
		"--resource-tier", "very-heavy", "--disk-path", fixture.work, "--", "/bin/sh", "-c", script)
	owner.Dir = fixture.work
	owner.Env = fixture.environment
	var ownerStderr bytes.Buffer
	owner.Stderr = &ownerStderr
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	var ownerError error
	finished := make(chan struct{})
	go func() {
		ownerError = owner.Wait()
		close(finished)
	}()
	defer func() {
		_ = os.WriteFile(release, nil, 0o600)
		<-finished
	}()
	fixture.awaitReady(t, ready, finished, &ownerError, &ownerStderr)

	listed := fixture.invoke(t, fixture.released, "status", "--config", fixture.sharedConfig, "--disk-path", fixture.work)
	if listed.code != 0 || !strings.Contains(listed.stdout, mixedV0100OwnerTierText) {
		t.Fatalf("released status exit=%d stdout=%q, want exit 0 listing %s", listed.code, listed.stdout, mixedV0100OwnerTierText)
	}
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	<-finished
	if ownerError != nil {
		t.Fatalf("very-heavy owner did not finish cleanly: %v: %s", ownerError, ownerStderr.String())
	}
}

// awaitReady polls for the payload's ready marker until the bound, and fails if the owner stops first.
func (fixture *mixedV0100Fixture) awaitReady(t *testing.T, ready string, finished <-chan struct{}, ownerError *error, stderr *bytes.Buffer) {
	t.Helper()
	ticker := time.NewTicker(mixedV0100PollInterval)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			return
		}
		select {
		case <-finished:
			t.Fatalf("very-heavy owner stopped before its payload ran: %v: %s", *ownerError, stderr.String())
		case <-fixture.runContext.Done():
			t.Fatalf("very-heavy owner was not ready before the bound: %v", fixture.runContext.Err())
		case <-ticker.C:
		}
	}
}
