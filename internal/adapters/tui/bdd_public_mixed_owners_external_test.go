package tui_test

import (
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
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/wahidyankf/hippo/tests/support"
)

const publicBDDMixedV084Variant = "mixed-v084"

func (fixture *publicBDDObservation) isMixedObservation() bool {
	return fixture.variant == "mixed" || fixture.variant == publicBDDMixedV084Variant
}

func publicBDDMixedOwnerCPU(variant string) int {
	if variant == publicBDDMixedV084Variant {
		return 1
	}
	return 2
}

// These are actual guarded command lifetimes. Their readiness token comes from
// the real HIPPO_SESSION published into the ordinary shell child's environment.
type publicBDDMixedJob struct {
	command                                                                           *exec.Cmd
	probe                                                                             *support.PublicTUIHostProbe
	cancel                                                                            context.CancelFunc
	done                                                                              chan struct{}
	release, token                                                                    string
	payloadPID                                                                        int
	output, diagnostics                                                               publicBDDMixedCapture
	waitError                                                                         error
	joined                                                                            bool
	role, stage                                                                       string
	probeBefore, probeAfter, startBefore, startAfter, readinessDeadline, waitFinished time.Time
}

func publicBDDVerifiedOldExecutable() (string, error) {
	path := os.Getenv("HIPPO_PUBLIC_OLD_BIN")
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errors.New("mixed versions require an explicitly pinned old executable")
	}
	owned, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	verified, verifyError := verifyPublicBDDOldExecutableAtRoot(owned, path)
	return verified, errors.Join(verifyError, owned.Close())
}

func verifyPublicBDDOldExecutableAtRoot(owned *os.Root, path string) (string, error) {
	info, err := owned.Lstat(filepath.Base(path))
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", errors.New("old executable must be regular and executable")
	}
	digest, err := hex.DecodeString(os.Getenv("HIPPO_PUBLIC_OLD_BIN_SHA256"))
	if err != nil || len(digest) != sha256.Size {
		return "", errors.New("old executable digest is not pinned")
	}
	file, err := owned.Open(filepath.Base(path))
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, readError := io.Copy(hash, file)
	if err := errors.Join(readError, file.Close()); err != nil {
		return "", err
	}
	if hex.EncodeToString(hash.Sum(nil)) != hex.EncodeToString(digest) {
		return "", errors.New("old executable digest changed")
	}
	// This is a pre-existing actual version receipt, not a constructor query or
	// a stamp synthesized from the desired version. The supervisor independently
	// binds its raw bytes and verified canonical archive provenance before Go.
	receipt := os.Getenv("HIPPO_PUBLIC_OLD_VERSION_RECEIPT")
	if !filepath.IsAbs(receipt) || filepath.Clean(receipt) != receipt {
		return "", errors.New("old executable has no actual identity receipt")
	}
	data, err := readPublicBDDPinnedReceipt(receipt)
	if err != nil {
		return "", err
	}
	if len(data) > 4096 {
		return "", errors.New("old identity receipt exceeds bound")
	}
	var identity struct {
		SchemaVersion int    `json:"schemaVersion"`
		Version       string `json:"version"`
		Commit        string `json:"commit"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil {
		return "", err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return "", errors.New("old identity receipt contains extra data")
	}
	if identity.SchemaVersion != 1 || identity.Version != "v0.8.4" || identity.Commit != "abde2ecbeff7513549cfd1c0805fd20b5d232c01" {
		return "", errors.New("old owner identity is not the verified retained release")
	}
	return path, nil
}

func (fixture *publicBDDObservation) prepareMixedOwners(ctx context.Context) error {
	old, err := publicBDDVerifiedOldExecutable()
	if err != nil {
		return err
	}
	for index, executable := range []string{old, os.Getenv("HIPPO_BIN")} {
		if executable == "" || (index == 1 && executable == old) {
			return errors.New("mixed versions require distinct real executable identities")
		}
		job, err := fixture.startMixedOwner(ctx, executable, index == 1)
		if err != nil {
			return err
		}
		if err := fixture.awaitMixedOwner(ctx, job); err != nil {
			return err
		}
		schemas, err := fixture.ledger.ProtocolSchemas()
		if err != nil {
			return err
		}
		if index == 0 {
			fixture.mixedSchemas = schemas
		} else if !reflect.DeepEqual(fixture.mixedSchemas, schemas) {
			return errors.New("current writer changed old coordination schemas")
		}
	}
	ids, pids := []string{}, []int{}
	for _, job := range fixture.mixed {
		ids = append(ids, job.token)
		pids = append(pids, job.command.Process.Pid)
	}
	if fixture.variant == publicBDDMixedV084Variant {
		if err := fixture.ledger.AdoptV084MixedOwners(ctx, ids, pids); err != nil {
			return err
		}
		for _, job := range fixture.mixed {
			godog.Logf(ctx, "actual v084 compatibility ready=%s", fixture.mixedStartupReceipt(job))
		}
		return nil
	}
	return fixture.ledger.AdoptMixedOwners(ctx, ids, pids)
}

func (fixture *publicBDDObservation) startMixedOwner(parent context.Context, executable string, current bool) (*publicBDDMixedJob, error) {
	ctx, cancel := context.WithCancel(parent)
	job := &publicBDDMixedJob{cancel: cancel, done: make(chan struct{}), role: "old", stage: "owned-directory"}
	if current {
		job.role = "current"
	}
	// Register ownership before the next fallible resource acquisition.
	fixture.mixed = append(fixture.mixed, job)
	closeUnstarted := func(err error) (*publicBDDMixedJob, error) {
		cancel()
		close(job.done)
		job.joined = true
		return nil, err
	}
	directory, err := os.MkdirTemp(fixture.directory, "mixed-owner-")
	if err != nil {
		return closeUnstarted(err)
	}
	ready := filepath.Join(directory, "ready")
	job.release = filepath.Join(directory, "release")
	job.stage = "probe-constructor"
	job.probeBefore = time.Now()
	job.probe, err = fixture.driver.NewPublicTUIHostProbeWithHelper(ctx, fixture.directory, support.PublicTUIHostReply{CPUPercent: 20}, fixture.helper)
	job.probeAfter = time.Now()
	if err != nil {
		return closeUnstarted(err)
	}
	job.stage = "command-construction"
	arguments := []string{"run", "--config", fixture.ledger.Configuration(), "--cwd", directory, "--disk-path", directory, "--profile", "balanced", "--class", "ephemeral", "--resource-tier", "light", "--source", "owner", "--tag", "role=owner", "--reserve-cpu", strconv.Itoa(publicBDDMixedOwnerCPU(fixture.variant)), "--reserve-memory-mib", "1024"}
	if current {
		arguments = append(arguments, "--label", "current-owner")
	}
	script := `trap 'exit 130' TERM; printf '%s' "$$" > "$PUBLIC_OWNER_READY.pid"; printf '%s' "$HIPPO_SESSION" > "$PUBLIC_OWNER_READY.tmp"; /bin/mv "$PUBLIC_OWNER_READY.tmp" "$PUBLIC_OWNER_READY"; while [ ! -e "$PUBLIC_OWNER_RELEASE" ]; do /bin/sleep 0.01; done; exit 0`
	arguments = append(arguments, "--", "/bin/sh", "-c", script)
	if current {
		job.command, err = fixture.driver.NativeAnnotationCommand(ctx, arguments...)
		if err != nil {
			return closeUnstarted(err)
		}
	} else {
		// Old binary has its own explicit route. It is never relabeled as the
		// TestMain selected current candidate or passed through its fallback.
		job.command = exec.CommandContext(ctx, executable, arguments...) // #nosec G702 -- Old executable is authenticated as absolute, regular and executable with pinned SHA and strict release receipt; argv and context remain separate.
	}
	command := job.command
	command.Dir = directory
	command.Env = append(fixture.environment(job.probe), "PUBLIC_OWNER_READY="+ready, "PUBLIC_OWNER_RELEASE="+job.release)
	command.Stdin = strings.NewReader("")
	command.Stdout, command.Stderr = &job.output, &job.diagnostics
	command.WaitDelay = 3 * time.Second
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		return command.Process.Signal(os.Interrupt)
	}
	job.stage = "exec-start"
	job.startBefore = time.Now()
	if err := command.Start(); err != nil {
		return closeUnstarted(err)
	}
	job.startAfter = time.Now()
	job.stage = "await-ready"
	go func() {
		defer close(job.done)
		job.waitError = command.Wait()
		job.waitFinished = time.Now()
	}()
	return job, nil
}

func (fixture *publicBDDObservation) awaitMixedOwner(ctx context.Context, job *publicBDDMixedJob) (result error) {
	defer func() {
		if result != nil {
			result = fmt.Errorf("%w; actual mixed startup=%s", result, fixture.mixedStartupReceipt(job))
		}
	}()
	// Guarded-owner preparation includes the public admission sampling window.
	// Reuse the remaining shared journey; TUI post-Start readiness stays three seconds.
	bound := ctx
	job.readinessDeadline, _ = bound.Deadline()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	ready := filepath.Join(job.command.Dir, "ready")
	for {
		data, err := os.ReadFile(ready)
		if err == nil {
			return acceptPublicMixedReadyToken(job, data, ready)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-job.done:
			job.joined = true
			return fmt.Errorf("mixed owner exited before readiness: %w stdout=%q stderr=%q", publicBDDDiagnosticCause(job.waitError), job.output.bytes(), job.diagnostics.bytes())
		case <-bound.Done():
			return bound.Err()
		case <-ticker.C:
		}
	}
}

func acceptPublicMixedReadyToken(job *publicBDDMixedJob, data []byte, ready string) error {
	runID := string(data)
	if len(runID) != 32 {
		return errors.New("real mixed owner published an invalid session ID")
	}
	if _, err := hex.DecodeString(runID); err != nil {
		return err
	}
	pidData, err := os.ReadFile(ready + ".pid")
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(string(pidData))
	if err != nil || pid <= 0 {
		return errors.New("real mixed payload has no positive PID")
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return fmt.Errorf("real mixed payload is not live: %w", err)
	}
	job.token, job.payloadPID = runID, pid
	job.stage = "ready-token-and-live-payload"
	return nil
}

// After actual queued joins, release both ordinary children together, then
// reserve part of the SAME remaining
// cleanup deadline for cancellation and observing their actual Wait completion.
func (fixture *publicBDDObservation) awaitNaturalMixedOwners(ctx context.Context) error {
	var result error
	deadline, bounded := ctx.Deadline()
	if bounded && time.Until(deadline) > 0 {
		natural, stop := context.WithDeadline(ctx, time.Now().Add(time.Until(deadline)/2))
		for _, job := range fixture.mixed {
			if job.joined {
				continue
			}
			select {
			case <-job.done:
				job.joined = true
			case <-natural.Done():
			}
			if natural.Err() != nil {
				break
			}
		}
		stop()
	} else if !bounded {
		result = errors.Join(result, errors.New("mixed cleanup requires its existing finite deadline"))
	}
	return result
}

func (fixture *publicBDDObservation) closeMixedJobs(ctx context.Context, queuedJoined bool) error {
	var result error
	if !queuedJoined && len(fixture.mixed) != 0 {
		result = errors.Join(result, errors.New("mixed owner natural release refused: queued actors remain unjoined"))
	}
	for _, job := range fixture.mixed {
		if queuedJoined && !job.joined && job.command != nil && job.command.Process != nil {
			if err := releasePublicMixedOwner(job); err != nil {
				result = errors.Join(result, fmt.Errorf("release mixed owner: %w", err))
				job.cancel()
			}
		}
	}
	if queuedJoined && len(fixture.mixed) != 0 {
		result = errors.Join(result, fixture.awaitNaturalMixedOwners(ctx))
	}
	// Cancel all pending commands before waiting on either one individually.
	// The parent context remains the original deadline; no fallback is created.
	for _, job := range fixture.mixed {
		if !job.joined {
			select {
			case <-job.done:
				job.joined = true
			default:
				result = errors.Join(result, errors.New("mixed owner did not complete during natural-release budget"))
				job.cancel()
			}
		}
	}
	for _, job := range fixture.mixed {
		result = errors.Join(result, fixture.closeMixedJob(ctx, job))
	}
	return result
}

func (fixture *publicBDDObservation) closeMixedJob(ctx context.Context, job *publicBDDMixedJob) error {
	var result error
	if !job.joined {
		select {
		case <-job.done:
			job.joined = true
		case <-ctx.Done():
			job.cancel()
			result = errors.Join(result, ctx.Err())
		}
		// If cancellation and actual completion raced, retain the observed done
		// fact. Never read Wait's result until that goroutine has really joined.
		if !job.joined {
			select {
			case <-job.done:
				job.joined = true
			default:
			}
		}
	}
	if !job.joined {
		result = errors.Join(result, errors.New("mixed owned command Wait remains unjoined"))
	} else if job.command != nil && job.command.Process != nil {
		if job.waitError != nil || job.command.ProcessState == nil || job.command.ProcessState.ExitCode() != 0 {
			result = errors.Join(result, fmt.Errorf("mixed owner did not naturally complete: %w", publicBDDDiagnosticCause(job.waitError)))
		}
		result = errors.Join(result, fixture.ledger.RequireOwnerRetired(ctx, job.token))
		if job.payloadPID > 0 && !errors.Is(syscall.Kill(job.payloadPID, 0), syscall.ESRCH) {
			result = errors.Join(result, errors.New("mixed owned payload survived joined command"))
		}
		result = errors.Join(result, requirePublicMixedArtifactsRetired(fixture.root, job.token))
	}
	// Probe cleanup is independent even when the original deadline expired or
	// the command remained unjoined. Both errors remain in the returned result.
	if job.probe != nil {
		result = errors.Join(result, job.probe.Close(ctx))
	}
	job.cancel()
	if fixture.variant == publicBDDMixedV084Variant {
		godog.Logf(ctx, "actual v084 compatibility retired=%s", fixture.mixedStartupReceipt(job))
	}
	return result
}

// Mixed commands may still write at the original readiness deadline. These
// bounded copies serialize only that fixture's capture, never a guessed Wait.
type publicBDDMixedCapture struct {
	mu      sync.Mutex
	capture publicBDDObservationCapture
}

func (capture *publicBDDMixedCapture) Write(data []byte) (int, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.capture.Write(data)
}

func (capture *publicBDDMixedCapture) bytes() []byte {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return append([]byte(nil), capture.capture.content.Bytes()...)
}

func (fixture *publicBDDObservation) mixedStartupReceipt(job *publicBDDMixedJob) string {
	receipt := map[string]any{"role": job.role, "stage": job.stage, "probeBefore": job.probeBefore, "probeAfter": job.probeAfter, "startBefore": job.startBefore, "startAfter": job.startAfter, "readinessDeadline": job.readinessDeadline, "observedAt": time.Now(), "token": job.token, "payloadPID": job.payloadPID, "stdout": string(job.output.bytes()), "stderr": string(job.diagnostics.bytes()), "root": fixture.root, "limits": "probe ACK is output only, not child exit; unavailable PID or Wait facts remain zero/unobserved"}
	recordPublicMixedCommandReceipt(receipt, job)
	select {
	case <-job.done:
		receipt["waitReturned"] = true
		receipt["waitFinished"] = job.waitFinished
		if job.waitError != nil {
			receipt["waitError"] = job.waitError.Error()
		}
		if job.command != nil && job.command.ProcessState != nil {
			receipt["exitCode"] = job.command.ProcessState.ExitCode()
		}
	default:
		receipt["waitReturned"] = false
	}
	if job.probe != nil {
		receipt["queryTrace"] = job.probe.Trace()
	}
	if fixture.ledger != nil {
		receipt["expectedOwnedInputs"] = fixture.ledger.Expected()
		config, err := readPublicMixedConfiguration(fixture.directory, fixture.ledger.Configuration())
		switch {
		case err != nil:
			receipt["configurationReadError"] = err.Error()
		case len(config) > 16384:
			receipt["configurationReadError"] = "owned config exceeds16KiB"
		default:
			receipt["configuration"] = string(config)
		}
	}
	recordPublicMixedLedgerReceipt(receipt, fixture.root)
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Sprintf("receipt serialization failed: %v", err)
	}
	return string(encoded)
}

func readPublicMixedConfiguration(directory, configuration string) ([]byte, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || configuration != filepath.Join(directory, "observation.json") {
		return nil, errors.New("mixed diagnostic configuration is not the owned fixture file")
	}
	owned, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	config, readError := owned.ReadFile("observation.json")
	return config, errors.Join(readError, owned.Close())
}

func recordPublicMixedCommandReceipt(receipt map[string]any, job *publicBDDMixedJob) {
	if job.command != nil {
		receipt["argv"] = append([]string(nil), job.command.Args...)
		if job.command.Process != nil {
			receipt["candidatePID"] = job.command.Process.Pid
		}
		safe := []string{}
		for _, entry := range job.command.Env {
			key, _, ok := strings.Cut(entry, "=")
			if ok && (key == "PATH" || key == "HIPPO_ROOT" || key == "TERM" || key == "NO_COLOR" || key == "GOMAXPROCS" || key == "PUBLIC_OWNER_READY" || key == "PUBLIC_OWNER_RELEASE") {
				safe = append(safe, entry)
			}
		}
		receipt["safeEnvironment"] = safe
		receipt["environmentScope"] = "only explicitly constructed nonsecret subject inputs; collector argv/race options observed in query trace; other entries omitted"
	}
}

func recordPublicMixedLedgerReceipt(receipt map[string]any, root string) {
	owned, err := os.OpenRoot(root)
	if err != nil {
		receipt["rootOpenError"] = err.Error()
		return
	}
	file, openError := owned.Open("reservations.json")
	if openError != nil {
		receipt["ledgerReadError"] = openError.Error()
	} else {
		recordPublicMixedLedgerFile(receipt, file)
		if closeError := file.Close(); closeError != nil {
			receipt["ledgerCloseError"] = closeError.Error()
		}
	}
	if closeError := owned.Close(); closeError != nil {
		receipt["rootCloseError"] = closeError.Error()
	}
}

func recordPublicMixedLedgerFile(receipt map[string]any, file *os.File) {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		receipt["ledgerReadError"] = "owned ledger not regular or stat failed"
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	switch {
	case err != nil:
		receipt["ledgerReadError"] = err.Error()
	case len(data) > 1024*1024:
		receipt["ledgerReadError"] = "owned ledger exceeds1MiB"
	default:
		receipt["ledger"] = string(data)
	}
}

func readPublicBDDPinnedReceipt(path string) ([]byte, error) {
	owned, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	data, readError := owned.ReadFile(filepath.Base(path))
	return data, errors.Join(readError, owned.Close())
}

func releasePublicMixedOwner(job *publicBDDMixedJob) error {
	if job.release != filepath.Join(job.command.Dir, "release") {
		return errors.New("mixed release does not belong to the actual owned command directory")
	}
	owned, err := os.OpenRoot(job.command.Dir)
	if err != nil {
		return err
	}
	return errors.Join(owned.WriteFile("release", nil, 0o600), owned.Close())
}

func TestPublicMixedReleaseRejectsForeignMarker(t *testing.T) {
	owned := t.TempDir()
	foreign := filepath.Join(t.TempDir(), "release")
	job := &publicBDDMixedJob{command: &exec.Cmd{Dir: owned}, release: foreign}
	if err := releasePublicMixedOwner(job); err == nil {
		t.Fatal("foreign release marker was accepted")
	}
	if _, err := os.Stat(foreign); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected foreign marker was changed: %v", err)
	}
	job.release = filepath.Join(owned, "release")
	if err := releasePublicMixedOwner(job); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(job.release)
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("actual owned release marker changed: info=%v error=%v", info, err)
	}
}

func requirePublicMixedArtifactsRetired(root, token string) error {
	owned, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	var result error
	for _, relative := range []string{"owner-metadata", "run-display"} {
		_, err := owned.Lstat(filepath.Join(relative, token+".json"))
		if !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, fmt.Errorf("mixed owned %s not retired: %w", relative, publicBDDDiagnosticCause(err)))
		}
	}
	return errors.Join(result, owned.Close())
}
