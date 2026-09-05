package support

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/config"
	"github.com/wahidyankf/hippo/internal/conformance"
	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
	"golang.org/x/sys/unix" //nolint:depguard // Cross-process flock fixtures must exercise the production kernel primitive.
)

func requireV04UnknownIdentityError(root string) error {
	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		return err
	}
	ledgerPath := filepath.Join(root, "reservations.json")
	before, err := os.ReadFile(ledgerPath)
	if err != nil {
		return err
	}
	identityPath := filepath.Join(root, "reservation-identities", session.Token+".lock")
	if err = os.Remove(identityPath); err != nil {
		return err
	}
	if err = os.Symlink(filepath.Base(identityPath), identityPath); err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(identityPath)
		_ = guard.ReleaseReservation(root, session)
	}()

	_, statusError := guard.ReservationStatus(context.Background(), root)
	candidate, admissionError := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if candidate != nil {
		_ = guard.ReleaseReservation(root, candidate)
	}
	after, readError := os.ReadFile(ledgerPath)
	if statusError == nil || admissionError == nil || candidate != nil || readError != nil || !bytes.Equal(before, after) {
		//nolint:errorlint // The diagnostic intentionally reports several independent fixture outcomes.
		return fmt.Errorf("unknown identity state did not fail closed: status=%v admission=%v candidate=%v read=%v", statusError, admissionError, candidate != nil, readError)
	}
	combined := statusError.Error() + admissionError.Error()
	if strings.Contains(combined, session.Token) || strings.Contains(combined, identityPath) {
		return errors.New("unknown identity error exposed a token or path")
	}

	return nil
}

func requireV04MissingLiveLedger(root string) error {
	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		return err
	}
	ledgerPath := filepath.Join(root, "reservations.json")
	ledger, err := os.ReadFile(ledgerPath)
	if err != nil {
		return err
	}
	if err = os.Remove(ledgerPath); err != nil {
		return err
	}
	defer func() {
		_ = os.WriteFile(ledgerPath, ledger, 0o600) //nolint:gosec // The fixture restores only its test-owned temporary ledger.
		_ = guard.ReleaseReservation(root, session)
	}()

	_, statusError := guard.ReservationStatus(context.Background(), root)
	candidate, admissionError := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if candidate != nil {
		_ = guard.ReleaseReservation(root, candidate)
	}
	_, statError := os.Stat(ledgerPath)
	if statusError == nil || admissionError == nil || candidate != nil || !errors.Is(statError, os.ErrNotExist) {
		//nolint:errorlint // The diagnostic intentionally reports several independent fixture outcomes.
		return fmt.Errorf("missing live ledger did not fail closed: status=%v admission=%v candidate=%v stat=%v", statusError, admissionError, candidate != nil, statError)
	}
	if strings.Contains(statusError.Error()+admissionError.Error(), session.Token) {
		return errors.New("lost-accounting error exposed a token")
	}

	return nil
}

func requireV04UnreadableSessionInventory(root string) error {
	inventoryPath := filepath.Join(root, "sessions")
	before := []byte("private legacy inventory\n")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(inventoryPath, before, 0o600); err != nil {
		return err
	}
	session, admissionError := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	_, markerError := os.Stat(filepath.Join(root, "coordination-mode.json"))
	markerPresent := markerError == nil
	if session != nil {
		_ = guard.ReleaseReservation(root, session)
	}
	after, readError := os.ReadFile(inventoryPath)
	if session != nil || !guard.IsCoordinationDeferred(admissionError) || markerPresent || readError != nil || !bytes.Equal(before, after) {
		//nolint:errorlint // The diagnostic intentionally reports several independent fixture outcomes.
		return fmt.Errorf("unreadable compatibility inventory did not fail closed: session=%v marker=%v admission=%v read=%v", session != nil, markerPresent, admissionError, readError)
	}
	if strings.Contains(admissionError.Error(), root) || strings.Contains(admissionError.Error(), inventoryPath) || !strings.Contains(admissionError.Error(), "inspect") {
		return errors.New("compatibility inventory error lacks private recovery guidance")
	}

	return nil
}

func requireV04FailedStaleHeavyCleanup(root string) error {
	heavyPath := filepath.Join(root, "heavy.lock")
	if err := os.MkdirAll(heavyPath, 0o700); err != nil {
		return err
	}
	ownerPath := filepath.Join(heavyPath, "owner.json")
	before := []byte(`{"schemaVersion":1,"pid":2147483647,"token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","class":"ephemeral"}` + "\n")
	if err := os.WriteFile(ownerPath, before, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(heavyPath, 0o500); err != nil {
		return err
	}
	defer func() {
		_ = os.Chmod(heavyPath, 0o700)
		_ = os.RemoveAll(heavyPath)
	}()
	session, admissionError := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	_, markerError := os.Stat(filepath.Join(root, "coordination-mode.json"))
	markerPresent := markerError == nil
	if session != nil {
		_ = guard.ReleaseReservation(root, session)
	}
	after, readError := os.ReadFile(ownerPath)
	if session != nil || !guard.IsCoordinationDeferred(admissionError) || markerPresent || readError != nil || !bytes.Equal(before, after) {
		//nolint:errorlint // The diagnostic intentionally reports several independent fixture outcomes.
		return fmt.Errorf("failed stale-heavy cleanup did not fail closed: session=%v marker=%v admission=%v read=%v", session != nil, markerPresent, admissionError, readError)
	}
	if strings.Contains(admissionError.Error(), root) || strings.Contains(admissionError.Error(), heavyPath) {
		return errors.New("stale-heavy cleanup error exposed a private path")
	}

	return nil
}

func setLedgerSequence(path string, sequence uint64) ([]byte, error) {
	before, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ledger map[string]any
	if err = json.Unmarshal(before, &ledger); err != nil {
		return nil, err
	}
	ledger[nextSequenceField] = sequence
	data, err := json.Marshal(ledger)
	if err != nil {
		return nil, err
	}

	return before, os.WriteFile(path, append(data, '\n'), 0o600)
}

func requireV04SequenceExhaustion(root string) error {
	liveRoot := filepath.Join(root, "live")
	owner, err := guard.AcquireReservation(
		context.Background(), liveRoot, "", policy.TaskService, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		return err
	}
	ledgerPath := filepath.Join(liveRoot, "reservations.json")
	original, err := setLedgerSequence(ledgerPath, math.MaxUint64)
	if err != nil {
		return err
	}
	maximumBytes, err := os.ReadFile(ledgerPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.WriteFile(ledgerPath, original, 0o600)
		_ = guard.ReleaseReservation(liveRoot, owner)
	}()
	candidate, admissionError := guard.AcquireReservation(
		context.Background(), liveRoot, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if candidate != nil {
		_ = guard.ReleaseReservation(liveRoot, candidate)
	}
	after, readError := os.ReadFile(ledgerPath)
	if candidate != nil || admissionError == nil || readError != nil || !bytes.Equal(maximumBytes, after) {
		//nolint:errorlint // The diagnostic intentionally reports several independent fixture outcomes.
		return fmt.Errorf("live maximum sequence mutated: candidate=%v admission=%v read=%v", candidate != nil, admissionError, readError)
	}

	staleRoot := filepath.Join(root, "stale")
	if err = os.MkdirAll(staleRoot, 0o700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(staleRoot, "coordination-mode.json"), []byte(reviewReservationMarker+"\n"), 0o600); err != nil {
		return err
	}
	floor := guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
	stale := map[string]any{
		schemaVersionField: 2,
		capacityField:      guard.ReservationVector{CPU: 4, MemoryBytes: policy.GiB},
		nextSequenceField:  uint64(math.MaxUint64),
		ownersField:        []any{validReviewOwner(strings.Repeat("a", 32), string(policy.TaskService), math.MaxUint64, floor, floor)},
		waitersField:       []any{},
	}
	data, err := json.Marshal(stale)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(staleRoot, "reservations.json"), append(data, '\n'), 0o600); err != nil {
		return err
	}
	fresh, err := guard.AcquireReservation(
		context.Background(), staleRoot, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil || fresh == nil {
		return fmt.Errorf("stale exhausted epoch did not restart: session=%v error=%w", fresh != nil, err)
	}
	defer func() { _ = guard.ReleaseReservation(staleRoot, fresh) }()
	if _, err = guard.ReservationStatus(context.Background(), staleRoot); err != nil {
		return fmt.Errorf("restarted stale epoch is invalid: %w", err)
	}

	return nil
}

func requireV04WaiterAggregateOverflow(root string) error {
	maximum := guard.ReservationVector{CPU: math.MaxInt, MemoryBytes: math.MaxInt64}
	floor := guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
	owner, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, profileBalanced, "",
		guard.ReservationPlan{Capacity: maximum, Requested: floor, Allocated: floor}, 20, 0,
	)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, owner) }()
	plan := guard.ReservationPlan{Capacity: maximum, Requested: maximum, Allocated: maximum}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan error, 2)
	for range 2 {
		go func() {
			session, acquireError := guard.AcquireReservation(ctx, root, "", policy.TaskEphemeral, profileBalanced, "", plan, 20, 10*time.Second)
			if session != nil {
				_ = guard.ReleaseReservation(root, session) //nolint:contextcheck // Test cleanup must outlive the canceled contender context.
			}
			results <- acquireError
		}()
	}
	deadline := time.Now().Add(time.Second)
	for {
		data, readError := os.ReadFile(filepath.Join(root, "reservations.json"))
		if readError == nil && bytes.Count(data, []byte(`"profile":"balanced"`)) >= 3 {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("overflow waiters did not both enqueue")
		}
		time.Sleep(time.Millisecond)
	}
	_, statusError := guard.ReservationStatus(context.Background(), root)
	cancel()
	<-results
	<-results
	if statusError == nil {
		return errors.New("schema four status accepted overflowing waiter totals")
	}
	if strings.Contains(statusError.Error(), owner.Token) || strings.Contains(statusError.Error(), root) {
		return errors.New("waiter aggregation error exposed private identity")
	}

	return nil
}

func (driver *Driver) requireCheckedMiBConversionV04(root string) error {
	maximum := math.MaxInt64 / policy.MiB
	for _, testCase := range []struct {
		name  string
		value int64
		valid bool
	}{
		{name: "maximum", value: maximum, valid: true},
		{name: "overflow", value: maximum + 1, valid: false},
	} {
		path := filepath.Join(root, testCase.name+".json")
		document := fmt.Sprintf(`{"schemaVersion":2,"coordination":{"maxMemoryMiB":%d}}`, testCase.value)
		if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
			return err
		}
		loaded, loadError := config.Load(path, true)
		if testCase.valid {
			if loadError != nil || loaded.Coordination.MaxMemoryBytes != testCase.value*policy.MiB {
				return fmt.Errorf("maximum MiB conversion was not exact: bytes=%d error=%w", loaded.Coordination.MaxMemoryBytes, loadError)
			}
		} else if loadError == nil || !strings.Contains(loadError.Error(), "representable") {
			return fmt.Errorf("overflowing config MiB was not rejected before multiplication: %w", loadError)
		}
	}

	overflowConfig := filepath.Join(root, "cli.json")
	if err := os.WriteFile(overflowConfig, []byte(`{"schemaVersion":2}`), 0o600); err != nil {
		return err
	}
	arguments := []string{"run", configFlag, overflowConfig, "--reserve-memory-mib", strconv.FormatInt(maximum+1, 10), "--", shellPath, "-c", "exit 0"}
	if driver.mode == e2eMode {
		command := exec.Command(driver.binary, arguments...)
		command.Env = append(os.Environ(), "HIPPO_ROOT="+filepath.Join(root, "cli-root"))
		output, runError := command.CombinedOutput()
		if exitCode(runError) != policy.ReplanRequiredExitCode || !strings.Contains(string(output), "representable") {
			return fmt.Errorf("compiled overflowing MiB result: exit=%d output=%q error=%w", exitCode(runError), output, runError)
		}

		return nil
	}
	base := time.Unix(0, 0)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, runError := (cli.Application{
		Stdout: stdout, Stderr: stderr,
		Collector:   &sequenceCollector{samples: []policy.Sample{healthySample(base)}},
		Environment: []string{"HIPPO_ROOT=" + filepath.Join(root, "cli-root")},
	}).Run(context.Background(), arguments)
	if code != policy.ReplanRequiredExitCode || runError == nil || !strings.Contains(runError.Error(), "representable") {
		return fmt.Errorf("in-process overflowing MiB result: exit=%d stderr=%q error=%w", code, stderr.String(), runError)
	}

	return nil
}

func requireV04CustomProfileReservation(root string) error {
	configPath := filepath.Join(root, "custom-profile.json")
	document := `{
  "schemaVersion": 2,
  "defaultProfile": "local-constrained",
  "profiles": {
    "local-constrained": {
      "extends": "constrained",
      "fallback": "minimal",
      "strict": false
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(document), 0o600); err != nil {
		return err
	}
	configured, err := config.Load(configPath, true)
	if err != nil {
		return err
	}
	if configured.Coordination.OwnerShares["local-constrained"] != 2 {
		return fmt.Errorf("custom constrained share=%d want=2", configured.Coordination.OwnerShares["local-constrained"])
	}
	sample := healthySample(time.Now())
	sample.AvailableParallelism = 9
	sample.PhysicalMemoryBytes = 8 * policy.GiB
	sample.EffectiveMemoryLimitBytes = 8 * policy.GiB
	resolution, err := configured.Catalog.Resolve("local-constrained", policy.TaskEphemeral, sample)
	if err != nil {
		return err
	}
	plan, err := guard.PlanReservation(sample, resolution, guard.ReservationPolicy{
		Enabled: true, MaxCPU: configured.Coordination.MaxCPU, MaxMemoryBytes: configured.Coordination.MaxMemoryBytes,
		MaxActiveOwners: configured.Coordination.MaxActiveOwners, OwnerShares: configured.Coordination.OwnerShares,
	}, 0, 0)
	if err != nil || plan.Allocated.CPU != 4 {
		return fmt.Errorf("custom constrained allocation=%+v want cpu=4: %w", plan.Allocated, err)
	}
	concurrencyPath := filepath.Join(root, "concurrency")
	settings := v04FastPolicy()
	settings.AdmissionWindow = time.Second
	exit, runError := guard.Run(context.Background(), guard.RunConfig{
		Command: shellPath, Arguments: []string{"-c", `printf '%s' "$HIPPO_CONCURRENCY" > "$CONCURRENCY_PATH"`},
		TaskClass: policy.TaskEphemeral, Environment: append(os.Environ(), "CONCURRENCY_PATH="+concurrencyPath), EvidenceRoot: filepath.Join(root, "shared"),
		Collector: &sequenceCollector{samples: []policy.Sample{sample, sample, sample}}, Policy: settings, Resolution: resolution,
		ReservationPolicy: guard.ReservationPolicy{
			Enabled: true, MaxCPU: configured.Coordination.MaxCPU, MaxMemoryBytes: configured.Coordination.MaxMemoryBytes,
			MaxActiveOwners: configured.Coordination.MaxActiveOwners, OwnerShares: configured.Coordination.OwnerShares,
		}, ReservationPlan: plan, Sleep: func(time.Duration) {}, ChildStdin: bytes.NewBuffer(nil),
		ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
	})
	if runError != nil || exit != 0 {
		return fmt.Errorf("custom profile guarded run: exit=%d error=%w", exit, runError)
	}
	concurrency, err := os.ReadFile(concurrencyPath)
	if err != nil || string(concurrency) != "4" {
		return fmt.Errorf("fixed allocation concurrency=%q want=4: %w", concurrency, err)
	}
	summary, err := readOnlySummaryV04(filepath.Join(root, "shared"))
	if err != nil || summary.Concurrency != plan.Allocated.CPU {
		return fmt.Errorf("summary concurrency=%d allocation=%d: %w", summary.Concurrency, plan.Allocated.CPU, err)
	}

	return nil
}

func exactCeilingV04(value, divisor int64) int64 {
	quotient, remainder := value/divisor, value%divisor
	if remainder != 0 {
		quotient++
	}

	return quotient
}

func requireV04MaximumAutomaticShares(string) error {
	sample := healthySample(time.Now())
	sample.AvailableParallelism = math.MaxInt
	sample.PhysicalMemoryBytes = math.MaxInt64
	sample.EffectiveMemoryLimitBytes = math.MaxInt64
	for _, testCase := range []struct {
		profile string
		shares  int
	}{
		{profile: profileBalanced, shares: 4},
		{profile: profileConstrained, shares: 2},
	} {
		resolution := policy.Resolution{ResolvedProfile: testCase.profile}
		plan, err := guard.PlanReservation(sample, resolution, guard.ReservationPolicy{
			Enabled: true, OwnerShares: map[string]int{testCase.profile: testCase.shares},
		}, 0, 0)
		want := guard.ReservationVector{
			CPU:         int(exactCeilingV04(int64(math.MaxInt-1), int64(testCase.shares))),
			MemoryBytes: exactCeilingV04(math.MaxInt64, int64(testCase.shares)),
		}
		if err != nil || plan.Allocated != want {
			return fmt.Errorf("%s maximum automatic share=%+v want=%+v: %w", testCase.profile, plan.Allocated, want, err)
		}
	}

	return nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitError.ExitCode()
	}

	return 1
}

func holdCoordinationMutation(root string) (*os.File, error) {
	lock, err := os.OpenFile(filepath.Join(root, "coordination.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		_ = lock.Close()

		return nil, err
	}

	return lock, nil
}

func releaseHeldCoordination(lock *os.File) error {
	return errors.Join(unix.Flock(int(lock.Fd()), unix.LOCK_UN), lock.Close())
}

func requireV04BoundedRemoteObservation(root string) error {
	owner, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, owner) }()
	if err = guard.ActivateReservation(root, owner, 91_001); err != nil {
		return err
	}
	victim, selected, err := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
	if err != nil || !selected {
		return fmt.Errorf("select bounded-observation victim: selected=%v error=%w", selected, err)
	}
	lock, err := holdCoordinationMutation(root)
	if err != nil {
		return err
	}
	start := time.Now()
	observed := make(chan error, 1)
	go func() { observed <- guard.WaitPressureVictimRelease(root, victim, 20*time.Millisecond) }()
	var observeError error
	select {
	case observeError = <-observed:
		if elapsed := time.Since(start); elapsed > 80*time.Millisecond {
			_ = releaseHeldCoordination(lock)

			return fmt.Errorf("remote observation exceeded its deadline: %s", elapsed)
		}
	case <-time.After(80 * time.Millisecond):
		_ = releaseHeldCoordination(lock)
		<-observed

		return errors.New("remote observation blocked on coordination mutation past its deadline")
	}
	if observeError == nil {
		_ = releaseHeldCoordination(lock)

		return errors.New("held coordination lock did not leave a bounded observation error")
	}
	if err = releaseHeldCoordination(lock); err != nil {
		return err
	}
	if next, nextSelected, nextError := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode); nextError != nil || nextSelected {
		return fmt.Errorf("bounded observation lost the shedding barrier: victim=%+v selected=%v error=%w", next, nextSelected, nextError)
	}

	return nil
}

func requireV04BoundedOwnerCancellation(string) error {
	return runInternalGuardRegressionV04("TestOwnerCancellationRetainsIdentityAcrossSameProcessCoordination")
}

//nolint:unused // Retained temporarily as independent cross-boundary diagnostic coverage; canonical behavior uses production locking above.
func requireV04BoundedOwnerCancellationExternal(root string) error { //nolint:cyclop,funlen,gocognit // The cross-process probe preserves child, ledger, identity, and later-release assertions together.
	probeSource := filepath.Join(root, "flock-probe.go")
	probeBinary := filepath.Join(root, "flock-probe")
	if err := os.WriteFile(probeSource, []byte(`package main
import ("errors"; "os"; "syscall")
func main() {
	f, err := os.OpenFile(os.Args[1], os.O_RDWR, 0600); if err != nil { os.Exit(2) }
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) { os.Exit(0) }
	if err == nil { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); os.Exit(1) }
	os.Exit(2)
}
`), 0o600); err != nil {
		return err
	}
	buildProbe := exec.Command("go", "build", "-o", probeBinary, probeSource)
	if output, err := buildProbe.CombinedOutput(); err != nil {
		return fmt.Errorf("build lock probe: %s: %w", output, err)
	}
	marker := filepath.Join(root, "child-pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	settings := v04FastPolicy()
	settings.SampleInterval = 2 * time.Millisecond
	settings.AdmissionWindow = 100 * time.Millisecond
	settings.TerminationGrace = 10 * time.Millisecond
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, runError := guard.Run(ctx, guard.RunConfig{
			Command: shellPath, Arguments: []string{"-c", `printf '%s' "$$" > "$CHILD_PID"; trap '' TERM; while :; do sleep 0.01; done`},
			TaskClass: policy.TaskEphemeral, Environment: append(os.Environ(), "CHILD_PID="+marker), EvidenceRoot: root,
			Collector: &sequenceCollector{samples: []policy.Sample{healthySample(time.Now()), healthySample(time.Now()), healthySample(time.Now())}},
			Policy:    settings, Resolution: policy.Resolution{RequestedProfile: profileBalanced, ResolvedProfile: profileBalanced, Concurrency: 1},
			ReservationPolicy: v04ReservationPolicy(), ReservationPlan: v04Plan(1, 256*policy.MiB),
			ChildStdin: bytes.NewBuffer(nil), ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		})
		done <- result{code: code, err: runError}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, statError := os.Stat(marker); statError == nil {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("bounded-cancellation child did not start")
		}
		time.Sleep(time.Millisecond)
	}
	ledgerPath := filepath.Join(root, "reservations.json")
	var before []byte
	var err error
	var ledger struct {
		Owners []struct {
			Token        string `json:"token"`
			ProcessGroup int    `json:"processGroup"`
		} `json:"owners"`
	}
	deadline = time.Now().Add(time.Second)
	for {
		before, err = os.ReadFile(ledgerPath)
		if err == nil && json.Unmarshal(before, &ledger) == nil && len(ledger.Owners) == 1 && ledger.Owners[0].ProcessGroup > 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bounded-cancellation owner ledger did not activate: %w", err)
		}
		time.Sleep(time.Millisecond)
	}
	lock, err := holdCoordinationMutation(root)
	if err != nil {
		return err
	}
	start := time.Now()
	cancel()
	var run result
	select {
	case run = <-done:
		if elapsed := time.Since(start); elapsed > time.Second {
			_ = releaseHeldCoordination(lock)

			return fmt.Errorf("owner cancellation exceeded bounded release: %s", elapsed)
		}
	case <-time.After(time.Second):
		_ = releaseHeldCoordination(lock)
		<-done

		return errors.New("owner cancellation blocked on release coordination")
	}
	if run.err == nil || run.code != 1 {
		_ = releaseHeldCoordination(lock)

		return fmt.Errorf("bounded release failure was not propagated: exit=%d error=%w", run.code, run.err)
	}
	after, readError := os.ReadFile(ledgerPath)
	identityPath := filepath.Join(root, "reservation-identities", ledger.Owners[0].Token+".lock")
	_, identityError := os.Stat(identityPath)
	probeError := exec.Command(probeBinary, identityPath).Run()
	if readError != nil || !bytes.Equal(before, after) || identityError != nil || probeError != nil {
		_ = releaseHeldCoordination(lock)

		//nolint:errorlint // The diagnostic intentionally reports several independent ownership probes.
		return fmt.Errorf("bounded release failure did not retain exact locked ownership: read=%v bytes=%v identity=%v probe=%v",
			readError, bytes.Equal(before, after), identityError, probeError)
	}
	if err = releaseHeldCoordination(lock); err != nil {
		return err
	}
	deadline = time.Now().Add(time.Second)
	for {
		totals, statusError := guard.ReservationStatus(context.Background(), root)
		if statusError == nil && totals.ActiveOwners == 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bounded release retry did not retire ownership: totals=%+v error=%w", totals, statusError)
		}
		time.Sleep(time.Millisecond)
	}
	pidData, err := os.ReadFile(marker)
	if err != nil {
		return err
	}
	var childPID int
	if _, err = fmt.Sscan(string(pidData), &childPID); err != nil {
		return err
	}
	if signalError := syscall.Kill(-childPID, 0); !errors.Is(signalError, syscall.ESRCH) {
		return fmt.Errorf("cancelled child group remains live: %w", signalError)
	}

	return nil
}

func requireV04CancelledWaiterCleanup(root string) error {
	ownerPlan := guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Allocated: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	owner, err := guard.AcquireReservation(context.Background(), root, "", policy.TaskService, profileMinimal, "", ownerPlan, 20, 0)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, owner) }()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		session, acquireError := guard.AcquireReservation(ctx, root, "", policy.TaskEphemeral, profileMinimal, "", ownerPlan, 20, time.Second)
		if session != nil {
			_ = guard.ReleaseReservation(root, session) //nolint:contextcheck // Test cleanup must outlive the canceled contender context.
		}
		result <- acquireError
	}()
	ledgerPath := filepath.Join(root, "reservations.json")
	deadline := time.Now().Add(time.Second)
	for {
		data, readError := os.ReadFile(ledgerPath)
		if readError == nil && bytes.Contains(data, []byte(`"waiters":[{"token"`)) {
			break
		}
		if time.Now().After(deadline) {
			cancel()

			return errors.New("cancelled-cleanup waiter did not enqueue")
		}
		time.Sleep(time.Millisecond)
	}
	lock, err := holdCoordinationMutation(root)
	if err != nil {
		cancel()

		return err
	}
	cancel()
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = releaseHeldCoordination(lock)
	}()
	if acquireError := <-result; !errors.Is(acquireError, context.Canceled) {
		return fmt.Errorf("cancelled waiter result: %w", acquireError)
	}
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		return err
	}
	var ledger struct {
		Waiters []json.RawMessage `json:"waiters"`
	}
	if err = json.Unmarshal(data, &ledger); err != nil {
		return err
	}
	if len(ledger.Waiters) != 0 {
		return errors.New("cancelled waiter remained in FIFO accounting after fresh cleanup deadline")
	}

	return nil
}

func requireV04FailedCancelledWaiterCleanup(root string) error { //nolint:cyclop,funlen,gocognit,gocyclo // One FIFO regression preserves the canceled head, follower order, identity, and later admission.
	plan := guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Allocated: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	owner, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, profileMinimal, "", plan, 20, 0,
	)
	if err != nil {
		return err
	}
	ownerReleased := false
	defer func() {
		if !ownerReleased {
			_ = guard.ReleaseReservation(root, owner)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		session, acquireError := guard.AcquireReservation(
			ctx, root, "", policy.TaskEphemeral, profileMinimal, "", plan, 20, time.Second,
		)
		if session != nil {
			_ = guard.ReleaseReservation(root, session) //nolint:contextcheck // Test cleanup must outlive the canceled contender context.
		}
		result <- acquireError
	}()
	ledgerPath := filepath.Join(root, "reservations.json")
	type waiterRecord struct {
		Token    string `json:"token"`
		Sequence uint64 `json:"sequence"`
	}
	type ledgerRecord struct {
		Waiters []waiterRecord `json:"waiters"`
	}
	deadline := time.Now().Add(time.Second)
	var before []byte
	var ledger ledgerRecord
	for {
		before, err = os.ReadFile(ledgerPath)
		if err == nil && json.Unmarshal(before, &ledger) == nil && len(ledger.Waiters) == 1 {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("failed-cleanup waiter did not enqueue")
		}
		time.Sleep(time.Millisecond)
	}
	identityPath := filepath.Join(root, "reservation-identities", ledger.Waiters[0].Token+".lock")
	anchorPath := filepath.Join(root, "reservation-identities", ledger.Waiters[0].Token+".anchor")
	lock, err := holdCoordinationMutation(root)
	if err != nil {
		return err
	}
	cancel()
	started := time.Now()
	acquireError := <-result
	if !errors.Is(acquireError, context.Canceled) || time.Since(started) > 500*time.Millisecond {
		_ = releaseHeldCoordination(lock)

		return fmt.Errorf("failed-cleanup cancellation was not bounded: %w", acquireError)
	}
	after, readError := os.ReadFile(ledgerPath)
	_, identityError := os.Stat(identityPath)
	_, anchorError := os.Stat(anchorPath)
	probe, probeError := os.OpenFile(identityPath, os.O_RDWR, 0o600)
	probeLocked := false
	if probeError == nil {
		probeLocked = unix.Flock(int(probe.Fd()), unix.LOCK_EX|unix.LOCK_NB) == nil
		if probeLocked {
			_ = unix.Flock(int(probe.Fd()), unix.LOCK_UN)
		}
		_ = probe.Close()
	}
	if readError != nil || !bytes.Equal(before, after) || identityError != nil || anchorError != nil || probeError != nil || probeLocked {
		_ = releaseHeldCoordination(lock)

		return errors.New("failed waiter cleanup did not retain exact FIFO and locked identity evidence")
	}
	type followingResult struct {
		session *guard.Session
		err     error
	}
	following := make(chan followingResult, 1)
	go func() {
		session, acquireError := guard.AcquireReservation(
			context.Background(), root, "", policy.TaskTransactional, profileMinimal, "", plan, 20, 2*time.Second,
		)
		following <- followingResult{session: session, err: acquireError}
	}()
	if err = releaseHeldCoordination(lock); err != nil {
		return err
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		data, ledgerError := os.ReadFile(ledgerPath)
		var current ledgerRecord
		decodeError := json.Unmarshal(data, &current)
		_, identityError = os.Stat(identityPath)
		_, anchorError = os.Stat(anchorPath)
		if ledgerError == nil && decodeError == nil && len(current.Waiters) == 1 &&
			current.Waiters[0].Token != ledger.Waiters[0].Token && current.Waiters[0].Sequence > ledger.Waiters[0].Sequence &&
			errors.Is(identityError, os.ErrNotExist) && errors.Is(anchorError, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			//nolint:errorlint // The diagnostic intentionally reports both ledger read and decode outcomes.
			return fmt.Errorf("failed waiter cleanup did not preserve following FIFO order: ledger=%v decode=%v", ledgerError, decodeError)
		}
		time.Sleep(time.Millisecond)
	}
	if err = guard.ReleaseReservation(root, owner); err != nil {
		return err
	}
	ownerReleased = true
	select {
	case admitted := <-following:
		if admitted.err != nil || admitted.session == nil {
			return fmt.Errorf("following FIFO waiter did not admit after head cleanup: %w", admitted.err)
		}

		return guard.ReleaseReservation(root, admitted.session)
	case <-time.After(2 * time.Second):
		return errors.New("following FIFO waiter admission timed out")
	}
}

func moduleRootV04() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statError := os.Stat(filepath.Join(directory, "go.mod")); statError == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("go.mod root is unavailable")
		}
		directory = parent
	}
}

func requireV04SupervisorDeathOwnership(root string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	binary := filepath.Join(root, "hippo-guard-helper.test")
	build := exec.Command("go", "test", "-c", "-o", binary, "./internal/guard")
	build.Dir = moduleRoot
	if output, buildError := build.CombinedOutput(); buildError != nil {
		return fmt.Errorf("build compiled guard fixture: %s: %w", output, buildError)
	}
	childPIDPath := filepath.Join(root, "long-child-pid")
	sharedRoot := filepath.Join(root, "shared")
	guardCommand := exec.Command(binary, "-test.run=^TestCompiledGuardHelper$", "-test.count=1")
	guardCommand.Env = append(os.Environ(),
		"HIPPO_COMPILED_GUARD_HELPER=supervisor-death", "HIPPO_COMPILED_GUARD_ROOT="+sharedRoot,
		"LONG_CHILD_PID="+childPIDPath,
	)
	guardStderrPath := filepath.Join(root, "guard-stderr")
	guardStderr, err := os.OpenFile(guardStderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = guardStderr.Close() }()
	guardCommand.Stderr = guardStderr
	readGuardStderr := func() string {
		data, _ := os.ReadFile(guardStderrPath)

		return string(data)
	}
	if err = guardCommand.Start(); err != nil {
		return err
	}
	guardExited := make(chan error, 1)
	go func() { guardExited <- guardCommand.Wait() }()
	defer func() {
		if guardCommand.Process != nil {
			_ = guardCommand.Process.Kill()
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	var childPID int
	for {
		select {
		case waitError := <-guardExited:
			return fmt.Errorf("compiled guard exited before child start: %w: %s", waitError, readGuardStderr())
		default:
		}
		data, readError := os.ReadFile(childPIDPath)
		if readError == nil {
			_, err = fmt.Sscan(string(data), &childPID)
			if err == nil && childPID > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			_ = guardCommand.Process.Kill()
			<-guardExited

			return fmt.Errorf("compiled guard child did not start: %s", readGuardStderr())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = guardCommand.Process.Signal(syscall.SIGKILL); err != nil {
		return err
	}
	<-guardExited
	if signalError := syscall.Kill(-childPID, 0); signalError != nil {
		return fmt.Errorf("guard kill also stopped the isolated child group: %w", signalError)
	}
	totals, statusError := guard.ReservationStatus(context.Background(), sharedRoot)
	if statusError != nil || totals.ActiveOwners != 1 {
		_ = syscall.Kill(-childPID, syscall.SIGKILL)

		return fmt.Errorf("live child reservation was released with its guard: totals=%+v error=%w", totals, statusError)
	}
	if err = syscall.Kill(-childPID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline = time.Now().Add(time.Second)
	for {
		totals, statusError = guard.ReservationStatus(context.Background(), sharedRoot)
		if statusError == nil && totals.ActiveOwners == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("exited child group retained reservation: totals=%+v error=%w", totals, statusError)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func runInternalGuardRegressionV04(name string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	command := exec.Command("go", "test", "./internal/guard", "-run", "^"+name+"$", "-count=1", "-v")
	command.Dir = moduleRoot
	output, runError := command.CombinedOutput()
	if runError != nil {
		return fmt.Errorf("internal guard regression %s: %s: %w", name, bytes.TrimSpace(output), runError)
	}
	if !bytes.Contains(output, []byte("=== RUN   "+name)) {
		return fmt.Errorf("internal guard regression %s is not implemented", name)
	}

	return nil
}

func requireV04PayloadIdentityIsolation(mode string) error {
	return runInternalGuardRegressionV04("TestPayloadCannotInheritLifetimeIdentityDescriptors/" + mode)
}

func requireV04FailedActivationIdentity(string) error {
	return runInternalGuardRegressionV04("TestRunFailedActivationReportRetainsLauncherIdentity")
}

func requireV04ActivationReportIsolation(string) error {
	return runInternalGuardRegressionV04("TestPayloadCannotForgeLifetimeActivationReport")
}

func requireV04ReservationIdentityPathCorruption(record, fault string) error {
	return runInternalGuardRegressionV04("TestReservationIdentityPathCorruptionFailClosed/" + record + "/" + fault)
}

func requireV04SchemaOneSupervisorLifetime(class string) error {
	return runInternalGuardRegressionV04("TestSchemaOneOwnershipSurvivesSupervisorDeath/" + class)
}

func requireV04SchemaOneEmbeddedRetirement(string) error {
	return runInternalGuardRegressionV04("TestSchemaOneEmbeddedOwnershipRetiresAfterHolder")
}

func requireV04LegacySchemaOneOwnership(class string) error {
	return runInternalGuardRegressionV04("TestLegacySchemaOnePIDOnlyOwnershipCompatibility/" + class)
}

func requireV04HeldPortDefersContender(string) error {
	return runInternalGuardRegressionV04("TestRunDefersWhenTheRequestedPortIsHeldByALiveOwner")
}

func requireV04StatusWaitsOutContention(string) error {
	return runInternalGuardRegressionV04("TestReservationStatusWaitsOutABusyCoordinationRoot")
}

func requireV04ContentionDefersInsteadOfFailing(string) error {
	return errors.Join(
		runInternalGuardRegressionV04("TestActivationContentionDefersInsteadOfFailing"),
		runInternalGuardRegressionV04("TestSupervisionContentionDoesNotStopHealthyWork"),
	)
}

func requireV04AggressiveGraceRetirement(string) error {
	return runInternalGuardRegressionV04("TestTerminateAndWaitConfirmsRetirementAfterAnAggressiveGrace")
}

func requireV04ExitedUnreapedGroupStop(string) error {
	return runInternalGuardRegressionV04("TestSignalGroupAcceptsAnExitedButUnreapedProcessGroup")
}

func requireV04BoundedGuardCancellation(string) error {
	return runInternalGuardRegressionV04("TestRunUnconfirmedRetirementPreservesOwnershipAndExit")
}

func requireV04BoundedGuardShedding(string) error {
	return runInternalGuardRegressionV04("TestRunUnconfirmedRetirementPreservesOwnershipAndExit")
}

func requireV04BoundedLifetimeHandshake(string) error {
	return runInternalGuardRegressionV04("TestStalledLifetimeHandshakeRetainsOwnershipUntilLauncherExit")
}

func requireV04LifetimeCapability(root string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	binary := filepath.Join(root, "hippo-capability")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hippo")
	build.Dir = moduleRoot
	if output, buildError := build.CombinedOutput(); buildError != nil {
		return fmt.Errorf("build capability fixture: %s: %w", output, buildError)
	}
	payloadMarker := filepath.Join(root, "unauthorized-payload")
	capability := "0123456789abcdef0123456789abcdef"
	command := exec.Command(
		binary, "--hippo-internal-lifetime-launcher", "", shellPath,
		"-c", `: > "$UNAUTHORIZED_PAYLOAD"`,
	)
	command.Env = append(os.Environ(),
		"HIPPO_INTERNAL_LIFETIME_LAUNCHER=1", "HIPPO_INTERNAL_LIFETIME_CAPABILITY="+capability,
		"UNAUTHORIZED_PAYLOAD="+payloadMarker,
	)
	output, runError := command.CombinedOutput()
	exitError, ok := errors.AsType[*exec.ExitError](runError)
	if !ok || exitError.ExitCode() != 1 {
		return fmt.Errorf("unauthorized launcher exit=%w output=%s", runError, output)
	}
	if _, statError := os.Stat(payloadMarker); !errors.Is(statError, os.ErrNotExist) {
		return errors.New("unauthorized launcher mode executed its payload")
	}
	if bytes.Contains(output, []byte(capability)) || bytes.Contains(output, []byte(root)) {
		return errors.New("unauthorized launcher rejection exposed private data")
	}

	return nil
}

func requireV04AbandonedLocalHandles(string) error {
	return runInternalGuardRegressionV04("TestRunUnconfirmedRetirementPreservesOwnershipAndExit/cancellation")
}

func requireV04PortIdentityCorruption(fault string) error {
	return runInternalGuardRegressionV04("TestTokenizedPortIdentityCorruptionFailClosed/" + fault)
}

func requireV04PortHandleABA(string) error {
	return runInternalGuardRegressionV04("TestStaleSameProcessPortHandleCannotReleaseReplacement")
}

func requireV04LegacyPortCompatibility(string) error {
	return runInternalGuardRegressionV04("TestLegacyTokenlessPortMarkerCompatibility")
}

func requireV04PortSupervisorLifetime(root string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	binary := filepath.Join(root, "hippo-port-supervisor.test")
	build := exec.Command("go", "test", "-c", "-o", binary, "./internal/guard")
	build.Dir = moduleRoot
	if output, buildError := build.CombinedOutput(); buildError != nil {
		return fmt.Errorf("build port supervisor fixture: %s: %w", output, buildError)
	}
	childPIDPath := filepath.Join(root, "port-child-pid")
	sharedRoot := filepath.Join(root, "shared")
	leaseRoot := filepath.Join(root, "port-leases")
	const port = 23_741
	guardCommand := exec.Command(binary, "-test.run=^TestCompiledGuardHelper$", "-test.count=1")
	guardCommand.Env = append(os.Environ(),
		"HIPPO_COMPILED_GUARD_HELPER=port-supervisor-death", "HIPPO_COMPILED_GUARD_ROOT="+sharedRoot,
		"HIPPO_COMPILED_GUARD_PORT="+strconv.Itoa(port), "HIPPO_COMPILED_GUARD_PORT_ROOT="+leaseRoot,
		"LONG_CHILD_PID="+childPIDPath,
	)
	if err = guardCommand.Start(); err != nil {
		return err
	}
	defer func() {
		if guardCommand.Process != nil {
			_ = guardCommand.Process.Kill()
		}
	}()
	childPID, err := waitForPIDFileV04(childPIDPath, 5*time.Second)
	if err != nil {
		_ = guardCommand.Process.Kill()
		_ = guardCommand.Wait()

		return err
	}
	if err = guardCommand.Process.Signal(syscall.SIGKILL); err != nil {
		return err
	}
	_ = guardCommand.Wait()
	competitor, acquireError := guard.AcquirePortLease(leaseRoot, port, "competitor", port, port)
	if acquireError == nil {
		_ = guard.ReleasePortLease(leaseRoot, competitor)
		_ = syscall.Kill(-childPID, syscall.SIGKILL)

		return errors.New("supervisor death released a live child port lease")
	}
	if err = syscall.Kill(-childPID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		competitor, acquireError = guard.AcquirePortLease(leaseRoot, port, "competitor", port, port)
		if acquireError == nil {
			return guard.ReleasePortLease(leaseRoot, competitor)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("retired child group retained port lease: %w", acquireError)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForPIDFileV04(path string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		var pid int
		if err == nil {
			_, err = fmt.Sscan(string(data), &pid)
			if err == nil && pid > 0 {
				return pid, nil
			}
		}
		if time.Now().After(deadline) {
			return 0, errors.New("child PID was not reported")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func requireV04BackgroundDescendantLifetime(root string) error {
	helperSource := filepath.Join(root, "background-helper.go")
	helperBinary := filepath.Join(root, "background-helper")
	source := `package main
import ("fmt"; "os"; "os/exec")
func main() {
	command := exec.Command("/bin/sh", "-c", "sleep 10")
	if err := command.Start(); err != nil { panic(err) }
	data := []byte(fmt.Sprintf("%d %d", os.Getpid(), command.Process.Pid))
	if err := os.WriteFile(os.Getenv("DESCENDANT_PIDS"), data, 0600); err != nil { panic(err) }
}
`
	if err := os.WriteFile(helperSource, []byte(source), 0o600); err != nil {
		return err
	}
	build := exec.Command("go", "build", "-o", helperBinary, helperSource)
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build background descendant fixture: %s: %w", output, err)
	}
	sharedRoot := filepath.Join(root, "shared")
	leaseRoot := filepath.Join(root, "ports")
	pidsPath := filepath.Join(root, "descendant-pids")
	const port = 23_742
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, runError := guard.Run(context.Background(), guard.RunConfig{
			Command: helperBinary, TaskClass: policy.TaskEphemeral, EvidenceRoot: sharedRoot,
			Environment: append(os.Environ(), "DESCENDANT_PIDS="+pidsPath),
			Collector:   &sequenceCollector{samples: []policy.Sample{healthySample(time.Now())}}, Policy: v04FastPolicy(),
			Resolution:        policy.Resolution{RequestedProfile: profileMinimal, ResolvedProfile: profileMinimal, Concurrency: 1},
			ReservationPolicy: v04ReservationPolicy(), ReservationPlan: v04Plan(1, 256*policy.MiB),
			LeasePort: port, LeaseMinimum: port, LeaseMaximum: port, LeaseOwner: fixtureOwner, PortLeaseRoot: leaseRoot,
			ChildStdin: bytes.NewBuffer(nil), ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		})
		done <- result{code: code, err: runError}
	}()
	deadline := time.Now().Add(5 * time.Second)
	var leaderPID, descendantPID int
	for {
		data, readError := os.ReadFile(pidsPath)
		if readError == nil {
			_, _ = fmt.Sscan(string(data), &leaderPID, &descendantPID)
			if leaderPID > 0 && descendantPID > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			return errors.New("background descendant was not reported")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(30 * time.Millisecond)
	totals, statusError := guard.ReservationStatus(context.Background(), sharedRoot)
	competitor, portError := guard.AcquirePortLease(leaseRoot, port, "competitor", port, port)
	if competitor != nil {
		_ = guard.ReleasePortLease(leaseRoot, competitor)
	}
	if statusError != nil || totals.ActiveOwners != 1 || portError == nil {
		_ = syscall.Kill(-leaderPID, syscall.SIGKILL)

		//nolint:errorlint // The diagnostic intentionally reports status and port-lease outcomes together.
		return fmt.Errorf("leader exit released descendant ownership: totals=%+v status=%v port=%v", totals, statusError, portError)
	}
	if err := syscall.Kill(-leaderPID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	select {
	case run := <-done:
		if run.err != nil || run.code != 0 {
			return fmt.Errorf("retired descendant run exit=%d error=%w", run.code, run.err)
		}
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)

		return errors.New("guard did not finish after process-group retirement")
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		totals, statusError = guard.ReservationStatus(context.Background(), sharedRoot)
		competitor, portError = guard.AcquirePortLease(leaseRoot, port, "competitor", port, port)
		if statusError == nil && totals.ActiveOwners == 0 && portError == nil {
			return guard.ReleasePortLease(leaseRoot, competitor)
		}
		if time.Now().After(deadline) {
			//nolint:errorlint // The diagnostic intentionally reports status and port-lease outcomes together.
			return fmt.Errorf("retired group did not become reclaimable: totals=%+v status=%v port=%v", totals, statusError, portError)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func requireV04InheritedDescendantLifetime(root string) error {
	helperSource := filepath.Join(root, "inherited-background-helper.go")
	helperBinary := filepath.Join(root, "inherited-background-helper")
	source := `package main
import ("fmt"; "os"; "os/exec")
func main() {
	command := exec.Command("/bin/sh", "-c", "sleep 10")
	if err := command.Start(); err != nil { panic(err) }
	data := []byte(fmt.Sprintf("%d %d", os.Getpid(), command.Process.Pid))
	if err := os.WriteFile(os.Getenv("DESCENDANT_PIDS"), data, 0600); err != nil { panic(err) }
}
`
	if err := os.WriteFile(helperSource, []byte(source), 0o600); err != nil {
		return err
	}
	build := exec.Command("go", "build", "-o", helperBinary, helperSource)
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build inherited descendant fixture: %s: %w", output, err)
	}
	sharedRoot := filepath.Join(root, "inherited-shared")
	leaseRoot := filepath.Join(root, "inherited-ports")
	plan := v04Plan(1, 256*policy.MiB)
	outer, err := guard.AcquireReservation(
		context.Background(), sharedRoot, "", policy.TaskEphemeral, profileMinimal, fixtureOwner, plan, 4, time.Second,
	)
	if err != nil || outer == nil {
		return fmt.Errorf("acquire inherited owner: %w", err)
	}
	defer func() { _ = guard.ReleaseReservation(sharedRoot, outer) }()
	pidsPath := filepath.Join(root, "inherited-descendant-pids")
	const port = 23_743
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, runError := guard.Run(context.Background(), guard.RunConfig{
			Command: helperBinary, TaskClass: policy.TaskEphemeral, EvidenceRoot: sharedRoot,
			Environment: append(os.Environ(), "HIPPO_SESSION="+outer.Token, "DESCENDANT_PIDS="+pidsPath),
			Collector:   &sequenceCollector{samples: []policy.Sample{healthySample(time.Now())}}, Policy: v04FastPolicy(),
			Resolution:        policy.Resolution{RequestedProfile: profileMinimal, ResolvedProfile: profileMinimal, Concurrency: 1},
			ReservationPolicy: v04ReservationPolicy(), ReservationPlan: plan,
			LeasePort: port, LeaseMinimum: port, LeaseMaximum: port, LeaseOwner: fixtureOwner, PortLeaseRoot: leaseRoot,
			ChildStdin: bytes.NewBuffer(nil), ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		})
		done <- result{code: code, err: runError}
	}()
	deadline := time.Now().Add(5 * time.Second)
	var leaderPID, descendantPID int
	for {
		data, readError := os.ReadFile(pidsPath)
		if readError == nil {
			_, _ = fmt.Sscan(string(data), &leaderPID, &descendantPID)
			if leaderPID > 0 && descendantPID > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			return errors.New("inherited background descendant was not reported")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(30 * time.Millisecond)
	select {
	case run := <-done:
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)

		return fmt.Errorf("inherited leader retired before its descendant: exit=%d error=%w", run.code, run.err)
	default:
	}
	competitor, portError := guard.AcquirePortLease(leaseRoot, port, "competitor", port, port)
	if competitor != nil {
		_ = guard.ReleasePortLease(leaseRoot, competitor)
	}
	if portError == nil {
		_ = syscall.Kill(-leaderPID, syscall.SIGKILL)
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)

		return errors.New("inherited descendant did not retain its port evidence")
	}
	if err = syscall.Kill(-leaderPID, syscall.SIGKILL); err != nil {
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)
	}
	select {
	case run := <-done:
		if run.err != nil || run.code != 0 {
			return fmt.Errorf("inherited retired run exit=%d error=%w", run.code, run.err)
		}
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)

		return errors.New("inherited guard did not finish after group retirement")
	}

	return nil
}

func readOnlySummaryV04(root string) (guard.EvidenceSummary, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return guard.EvidenceSummary{}, err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".summary.json") {
			continue
		}
		data, readError := os.ReadFile(filepath.Join(root, entry.Name()))
		if readError != nil {
			return guard.EvidenceSummary{}, readError
		}
		var summary guard.EvidenceSummary
		if decodeError := json.Unmarshal(data, &summary); decodeError != nil {
			return guard.EvidenceSummary{}, decodeError
		}

		return summary, nil
	}

	return guard.EvidenceSummary{}, errors.New("development summary was not written")
}

func requireV04ShortOverlapPeak(root string) error {
	scenarioRoot := filepath.Join(root, "short-overlap-peak")
	if err := os.MkdirAll(scenarioRoot, 0o700); err != nil {
		return err
	}
	marker := filepath.Join(scenarioRoot, "child-started")
	finish := filepath.Join(scenarioRoot, "child-finish")
	settings := v04FastPolicy()
	settings.SampleInterval = 5 * time.Second
	settings.AdmissionWindow = time.Second
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, runError := guard.Run(context.Background(), guard.RunConfig{
			Command: shellPath, Arguments: []string{"-c", `printf started > "$PEAK_CHILD"; while [ ! -f "$PEAK_FINISH" ]; do sleep 0.005; done`},
			TaskClass: policy.TaskEphemeral, Environment: append(os.Environ(), "PEAK_CHILD="+marker, "PEAK_FINISH="+finish), EvidenceRoot: scenarioRoot,
			Collector: &sequenceCollector{samples: []policy.Sample{healthySample(time.Now()), healthySample(time.Now()), healthySample(time.Now())}},
			Policy:    settings, Resolution: policy.Resolution{RequestedProfile: profileBalanced, ResolvedProfile: profileBalanced, Concurrency: 1},
			ReservationPolicy: v04ReservationPolicy(), ReservationPlan: v04Plan(1, 256*policy.MiB), Sleep: func(time.Duration) {},
			ChildStdin: bytes.NewBuffer(nil), ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		})
		done <- result{code: code, err: runError}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("short-overlap child did not start")
		}
		time.Sleep(time.Millisecond)
	}
	defer func() { _ = os.WriteFile(finish, []byte("finish\n"), 0o600) }()
	second, err := guard.AcquireReservation(
		context.Background(), scenarioRoot, "", policy.TaskService, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, time.Second,
	)
	if err != nil {
		return err
	}
	if err = guard.ReleaseReservation(scenarioRoot, second); err != nil {
		return err
	}
	if err = os.WriteFile(finish, []byte("finish\n"), 0o600); err != nil {
		return err
	}
	run := <-done
	if run.err != nil || run.code != 0 {
		return fmt.Errorf("short-overlap guarded run: exit=%d error=%w", run.code, run.err)
	}
	summary, err := readOnlySummaryV04(scenarioRoot)
	if err != nil || summary.PeakOwnerCount != 2 {
		return fmt.Errorf("short-overlap peak=%d want=2: %w", summary.PeakOwnerCount, err)
	}

	return nil
}

func requireV04AtomicTempRetention(root string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	protected := map[string][]byte{
		".coordination-mode-live.tmp": []byte("coordination write\n"),
		".reservations-live.tmp":      []byte("reservation write\n"),
	}
	for name, data := range protected {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
	}
	old := time.Now().Add(-40 * 24 * time.Hour)
	unrelated := filepath.Join(root, ".unrelated-live.tmp")
	if err := os.WriteFile(unrelated, []byte("unrelated\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chtimes(unrelated, old, old); err != nil {
		return err
	}
	if err := evidence.Cleanup(root, time.Now()); err != nil {
		return err
	}
	for name, before := range protected {
		after, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || !bytes.Equal(before, after) {
			return fmt.Errorf("live atomic-write temp %s changed: %w", name, err)
		}
	}
	if _, err := os.Stat(unrelated); !errors.Is(err, os.ErrNotExist) {
		return errors.New("unrecognized expired temp evidence was retained")
	}

	return nil
}

func requireV04OrphanedAtomicTempRetention(root string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	stable := map[string][]byte{
		"coordination-mode.json": []byte("stable coordination\n"),
		"reservations.json":      []byte("stable reservations\n"),
	}
	for name, data := range stable {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			return err
		}
	}
	orphans := []string{".coordination-mode-orphan.tmp", ".reservations-orphan.tmp"}
	old := time.Now().Add(-40 * 24 * time.Hour)
	for _, name := range orphans {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("orphan\n"), 0o600); err != nil {
			return err
		}
		if err := os.Chtimes(path, old, old); err != nil {
			return err
		}
	}
	if err := evidence.Cleanup(root, time.Now()); err != nil {
		return err
	}
	for _, name := range orphans {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("expired atomic temp %s was retained", name)
		}
	}
	for name, before := range stable {
		after, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || !bytes.Equal(before, after) {
			return fmt.Errorf("stable protocol file %s changed: %w", name, err)
		}
	}

	return nil
}

func requireV04ReservedHIPPOEnvironmentMappings(root string) error {
	for _, name := range []string{"HIPPO_ROOT", "HIPPO_RESERVED_MEMORY_BYTES", "HIPPO_CONFIG", "HIPPO_DEFAULT_CONFIG"} {
		caseRoot := filepath.Join(root, strings.ToLower(strings.TrimPrefix(name, "HIPPO_")))
		marker := filepath.Join(caseRoot, "child-started")
		code, runError := guard.Run(context.Background(), guard.RunConfig{
			Command: shellPath, Arguments: []string{"-c", childStartedScript},
			TaskClass: policy.TaskEphemeral, EvidenceRoot: caseRoot,
			Environment:            append(os.Environ(), name+"=1", "CHILD_MARKER="+marker),
			ConcurrencyEnvironment: []string{name},
			Collector:              &sequenceCollector{samples: []policy.Sample{healthySample(time.Now())}},
			Policy:                 v04FastPolicy(),
			Resolution:             policy.Resolution{RequestedProfile: profileBalanced, ResolvedProfile: profileBalanced, Concurrency: 2},
			ReservationPolicy:      v04ReservationPolicy(), ReservationPlan: v04Plan(2, 512*policy.MiB),
			ChildStdin: bytes.NewBuffer(nil), ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		})
		if code != policy.ReplanRequiredExitCode || runError == nil {
			//nolint:errorlint // The diagnostic intentionally reports the stable exit and runtime error together.
			return fmt.Errorf("reserved mapping %s returned exit=%d error=%v", name, code, runError)
		}
		if _, statError := os.Stat(marker); !errors.Is(statError, os.ErrNotExist) {
			return fmt.Errorf("reserved mapping %s started a child", name)
		}
	}

	caseRoot := filepath.Join(root, "arbitrary")
	output := &bytes.Buffer{}
	code, runError := guard.Run(context.Background(), guard.RunConfig{
		Command: shellPath, Arguments: []string{"-c", `printf '%s' "$ARBITRARY_WORKERS"`},
		TaskClass: policy.TaskEphemeral, EvidenceRoot: caseRoot,
		Environment: os.Environ(), ConcurrencyEnvironment: []string{"ARBITRARY_WORKERS"},
		Collector: &sequenceCollector{samples: []policy.Sample{healthySample(time.Now())}}, Policy: v04FastPolicy(),
		Resolution:        policy.Resolution{RequestedProfile: profileBalanced, ResolvedProfile: profileBalanced, Concurrency: 2},
		ReservationPolicy: v04ReservationPolicy(), ReservationPlan: v04Plan(2, 512*policy.MiB),
		ChildStdin: bytes.NewBuffer(nil), ChildStdout: output, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
	})
	if code != 0 || runError != nil || output.String() != "2" {
		//nolint:errorlint // The diagnostic intentionally reports exit, output, and runtime error together.
		return fmt.Errorf("arbitrary worker mapping exit=%d output=%q error=%v", code, output.String(), runError)
	}

	return nil
}

func runEvidenceCleanupGuardV04(root string, class policy.TaskClass) (int, error) {
	settings := v04FastPolicy()
	settings.SampleInterval = 2 * time.Millisecond
	settings.AdmissionWindow = 100 * time.Millisecond

	return guard.Run(context.Background(), guard.RunConfig{
		Command: shellPath, Arguments: []string{"-c", "sleep 0.02"}, TaskClass: class, EvidenceRoot: root,
		Environment: os.Environ(), Collector: &sequenceCollector{samples: []policy.Sample{healthySample(time.Now()), healthySample(time.Now())}},
		Policy: settings, Resolution: policy.Resolution{RequestedProfile: profileBalanced, ResolvedProfile: profileBalanced, Concurrency: 1},
		ReservationPolicy: v04ReservationPolicy(), ReservationPlan: v04Plan(1, 256*policy.MiB),
		ChildStdin: bytes.NewBuffer(nil), ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
	})
}

func requireV04ConcurrentEvidenceCleanup(root string) error {
	churnRoot := filepath.Join(root, "concurrent-evidence")
	if err := os.MkdirAll(churnRoot, 0o700); err != nil {
		return err
	}
	paths := make([]string, 1200)
	marker := []byte(fmt.Sprintf(`{"schemaVersion":1,"pid":%d}`, os.Getpid()))
	for index := range paths {
		paths[index] = filepath.Join(churnRoot, fmt.Sprintf("churn-%04d.active.json", index))
		if err := os.WriteFile(paths[index], marker, 0o600); err != nil {
			return err
		}
	}
	cleanupResult := make(chan error, 1)
	go func() { cleanupResult <- evidence.Cleanup(churnRoot, time.Now()) }()
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	cleanupError := <-cleanupResult

	type result struct {
		code int
		err  error
	}
	results := make(chan result, 2)
	for _, class := range []policy.TaskClass{policy.TaskEphemeral, policy.TaskService} {
		go func() {
			code, runError := runEvidenceCleanupGuardV04(churnRoot, class)
			results <- result{code: code, err: runError}
		}()
	}
	for range 2 {
		run := <-results
		if run.err != nil || run.code != 0 {
			return fmt.Errorf("parallel guarded evidence run: exit=%d error=%w", run.code, run.err)
		}
	}
	if cleanupError != nil {
		return fmt.Errorf("post-snapshot evidence disappearance aborted cleanup: %w", cleanupError)
	}
	entries, err := os.ReadDir(churnRoot)
	if err != nil {
		return err
	}
	summaries := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".summary.json") {
			summaries++
		}
	}
	if summaries < 2 {
		return fmt.Errorf("parallel guarded evidence retained %d summaries", summaries)
	}

	return nil
}

func copyExecutableV04(source, target string) (string, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(target, data, 0o700); err != nil { //nolint:gosec // Both paths are test-owned executable fixtures selected by the harness.
		return "", err
	}
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:]), nil
}

func conformanceFixtureV04(root string) (conformance.Manifest, string, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return conformance.Manifest{}, "", err
	}
	binary := filepath.Join(root, "hippo-fixture")
	truePath, err := exec.LookPath(trueCommandName)
	if err != nil {
		return conformance.Manifest{}, "", err
	}
	checksum, err := copyExecutableV04(truePath, binary)
	if err != nil {
		return conformance.Manifest{}, "", err
	}
	manifest := conformance.Manifest{
		SchemaVersion: 1, HIPPOBinary: binary, HIPPOSHA256: checksum, SharedRoot: filepath.Join(root, "shared"),
	}
	for index := range 4 {
		path := filepath.Join(root, fmt.Sprintf("consumer-%d", index+1))
		if err = initializeReviewCheckout(path); err != nil {
			return conformance.Manifest{}, "", err
		}
		manifest.Consumers = append(manifest.Consumers, conformance.Consumer{
			Name: fmt.Sprintf("consumer-%d", index+1), Path: path,
			Gates: []conformance.Command{{Arguments: []string{trueCommandName}}},
		})
	}

	return manifest, binary, nil
}

func writeConformanceManifestV04(path string, manifest conformance.Manifest) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}

func requireV04EmptyConformanceInput(root, kind string) error {
	caseRoot := filepath.Join(root, strings.ReplaceAll(kind, " ", "-"))
	manifest, _, err := conformanceFixtureV04(caseRoot)
	if err != nil {
		return err
	}
	started := filepath.Join(caseRoot, "command-started")
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		shellPath, "-c", childStartedArgScript, conformanceLabel, started,
	}}}
	manifestPath := filepath.Join(caseRoot, "manifest.json")
	switch kind {
	case "binary":
		manifest.HIPPOBinary = ""
	case "shared root":
		manifest.SharedRoot = ""
	case "consumer":
		manifestPath = filepath.Join(manifest.Consumers[0].Path, "manifest.json")
		manifest.Consumers[0].Path = ""
	default:
		return errors.New("unknown empty conformance input fixture")
	}
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}
	runError := conformance.Run(context.Background(), manifestPath, &bytes.Buffer{})
	if runError == nil || !strings.Contains(runError.Error(), "required") {
		return fmt.Errorf("empty raw %s input was canonicalized before required-field validation: %w", kind, runError)
	}
	if strings.Contains(runError.Error(), caseRoot) {
		return errors.New("empty conformance input error exposed a private path")
	}
	if _, statError := os.Stat(started); !errors.Is(statError, os.ErrNotExist) {
		return errors.New("empty conformance input started a checkout command")
	}

	return nil
}

func requireV04CanonicalConformanceInputs(root string) error { //nolint:gocognit // One scenario exercises three coupled canonical-input invariants.
	caseRoot := filepath.Join(root, "canonical")
	manifest, binary, err := conformanceFixtureV04(caseRoot)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(caseRoot, "manifest.json")
	alias := filepath.Join(caseRoot, "consumer-alias")
	original := manifest.Consumers[0].Path
	other := manifest.Consumers[1].Path
	if err = os.Symlink(original, alias); err != nil {
		return err
	}
	canonicalRoot, err := filepath.EvalSymlinks(caseRoot)
	if err != nil {
		return err
	}
	canonicalOriginal, err := filepath.EvalSymlinks(original)
	if err != nil {
		return err
	}
	manifest.HIPPOBinary = filepath.Base(binary)
	manifest.SharedRoot = "shared"
	for index := range manifest.Consumers {
		manifest.Consumers[index].Path = filepath.Base(manifest.Consumers[index].Path)
	}
	manifest.Consumers[0].Path = filepath.Base(alias)
	manifest.Consumers[0].Bootstrap = []conformance.Command{{Arguments: []string{
		shellPath, "-c", `rm "$1"; ln -s "$2" "$1"`, conformanceLabel, alias, other,
	}}}
	for index := range manifest.Consumers {
		expected := manifest.Consumers[index].Path
		if index == 0 {
			expected = canonicalOriginal
		} else {
			expected = filepath.Join(canonicalRoot, expected)
		}
		manifest.Consumers[index].Gates = []conformance.Command{{Arguments: []string{
			shellPath, "-c",
			`test "$(pwd -P)" = "$1" && test "$(env | grep -c '^HIPPO_BIN=')" = 1 && test "$(env | grep -c '^HIPPO_ROOT=')" = 1 && test -x "$HIPPO_BIN" && test "$HIPPO_BIN" != "$2" && test "$HIPPO_ROOT" = "$3"`,
			conformanceLabel, expected, filepath.Join(canonicalRoot, filepath.Base(binary)), filepath.Join(canonicalRoot, "shared"),
		}}}
	}
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}
	previousBinary, hadBinary := os.LookupEnv("HIPPO_BIN")
	previousRoot, hadRoot := os.LookupEnv("HIPPO_ROOT")
	_ = os.Setenv("HIPPO_BIN", "inherited-wrong-binary")
	_ = os.Setenv("HIPPO_ROOT", "inherited-wrong-root")
	defer func() {
		if hadBinary {
			_ = os.Setenv("HIPPO_BIN", previousBinary)
		} else {
			_ = os.Unsetenv("HIPPO_BIN")
		}
		if hadRoot {
			_ = os.Setenv("HIPPO_ROOT", previousRoot)
		} else {
			_ = os.Unsetenv("HIPPO_ROOT")
		}
	}()
	if err = conformance.Run(context.Background(), manifestPath, &bytes.Buffer{}); err != nil {
		return fmt.Errorf("canonical manifest inputs were not frozen/replaced: %w", err)
	}

	for _, relation := range []string{"equal", "inside", "contains"} {
		overlap, _, fixtureError := conformanceFixtureV04(filepath.Join(root, "overlap-"+relation))
		if fixtureError != nil {
			return fixtureError
		}
		switch relation {
		case "equal":
			overlap.SharedRoot = overlap.Consumers[0].Path
		case "inside":
			overlap.SharedRoot = filepath.Join(overlap.Consumers[0].Path, "runtime")
		case "contains":
			overlap.SharedRoot = filepath.Dir(overlap.Consumers[0].Path)
		}
		path := filepath.Join(root, "overlap-"+relation, "manifest.json")
		if err = writeConformanceManifestV04(path, overlap); err != nil {
			return err
		}
		runError := conformance.Run(context.Background(), path, &bytes.Buffer{})
		if runError == nil || !strings.Contains(runError.Error(), "overlap") {
			return fmt.Errorf("%s shared-root relation was not rejected: %w", relation, runError)
		}
		for _, consumer := range overlap.Consumers {
			if strings.Contains(runError.Error(), consumer.Path) {
				return errors.New("shared-root overlap error exposed a checkout path")
			}
		}
	}

	return nil
}

func requireV04ConformanceCallerSessionIsolation(root string) error {
	caseRoot := filepath.Join(root, "caller-session")
	manifest, _, err := conformanceFixtureV04(caseRoot)
	if err != nil {
		return err
	}
	plan := v04Plan(1, 256*policy.MiB)
	caller, err := guard.AcquireReservation(
		context.Background(), manifest.SharedRoot, "", policy.TaskService, profileBalanced, "caller", plan, 20, 0,
	)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(manifest.SharedRoot, caller) }()

	explicitConfig := filepath.Join(caseRoot, "operator-override.json")
	for index := range manifest.Consumers {
		manifest.Consumers[index].Gates = []conformance.Command{{Arguments: []string{
			shellPath, "-c",
			`test -z "${HIPPO_SESSION+x}" && test -z "${HIPPO_PROFILE+x}" && test -z "${HIPPO_CONCURRENCY+x}" && test -z "${HIPPO_RESERVED_MEMORY_BYTES+x}" && test -z "${HIPPO_DEFAULT_CONFIG+x}" && test "$HIPPO_CONFIG" = "$1"`,
			conformanceLabel, explicitConfig,
		}}}
	}
	manifestPath := filepath.Join(caseRoot, "manifest.json")
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}
	protocolEnvironment := map[string]string{
		"HIPPO_SESSION": caller.Token, "HIPPO_PROFILE": profileBalanced,
		"HIPPO_CONCURRENCY": "1", "HIPPO_RESERVED_MEMORY_BYTES": strconv.FormatInt(256*policy.MiB, 10),
		"HIPPO_DEFAULT_CONFIG": filepath.Join(caseRoot, "caller-repository-default.json"),
		"HIPPO_CONFIG":         explicitConfig,
	}
	previous := make(map[string]string, len(protocolEnvironment))
	present := make(map[string]bool, len(protocolEnvironment))
	for name, value := range protocolEnvironment {
		previous[name], present[name] = os.LookupEnv(name)
		if err = os.Setenv(name, value); err != nil {
			return err
		}
	}
	defer func() {
		for name := range protocolEnvironment {
			if present[name] {
				_ = os.Setenv(name, previous[name])
			} else {
				_ = os.Unsetenv(name)
			}
		}
	}()
	if runError := conformance.Run(context.Background(), manifestPath, &bytes.Buffer{}); runError != nil {
		return fmt.Errorf("conformance consumer inherited caller reservation state: %w", runError)
	}

	owners := make([]*guard.Session, 0, 3)
	defer func() {
		for _, owner := range owners {
			_ = guard.ReleaseReservation(manifest.SharedRoot, owner)
		}
	}()
	for index := range 3 {
		owner, acquireError := guard.AcquireReservation(
			context.Background(), manifest.SharedRoot, "", policy.TaskEphemeral, profileBalanced,
			fmt.Sprintf("consumer-%d", index+1), plan, 20, 0,
		)
		if acquireError != nil {
			return fmt.Errorf("independent consumer reservation %d: %w", index+1, acquireError)
		}
		owners = append(owners, owner)
		inherited, inheritError := guard.AcquireReservation(
			context.Background(), manifest.SharedRoot, owner.Token, policy.TaskEphemeral, profileBalanced, "nested", plan, 20, 0,
		)
		if inheritError != nil || inherited == nil || !inherited.Inherited || inherited.Token != owner.Token {
			return fmt.Errorf("consumer %d nested reservation did not inherit only its outer owner: %w", index+1, inheritError)
		}
	}
	deferred, deferError := guard.AcquireReservation(
		context.Background(), manifest.SharedRoot, "", policy.TaskEphemeral, profileBalanced, "consumer-4", plan, 20, 50*time.Millisecond,
	)
	if deferred != nil {
		_ = guard.ReleaseReservation(manifest.SharedRoot, deferred)
	}
	if deferred != nil || !errors.Is(deferError, guard.ErrReservationDeferred) {
		return fmt.Errorf("independent consumer overlap did not exhaust real capacity: %w", deferError)
	}

	return nil
}

func requireV04ReplacedConformanceCheckout(root string) error {
	caseRoot := filepath.Join(root, "replaced-checkout")
	manifest, _, err := conformanceFixtureV04(caseRoot)
	if err != nil {
		return err
	}
	checkout := manifest.Consumers[0].Path
	original := checkout + "-original"
	gateMarker := filepath.Join(caseRoot, "replacement-gate-started")
	manifest.Consumers[0].Bootstrap = []conformance.Command{{Arguments: []string{
		shellPath, "-c", `mv "$1" "$2" && cp -R "$2" "$1"`, conformanceLabel, checkout, original,
	}}}
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		shellPath, "-c", childStartedArgScript, conformanceLabel, gateMarker,
	}}}
	manifestPath := filepath.Join(caseRoot, "manifest.json")
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}

	runError := conformance.Run(context.Background(), manifestPath, &bytes.Buffer{})
	if runError == nil {
		return errors.New("conformance accepted a replacement checkout directory with identical Git state")
	}
	if strings.Contains(runError.Error(), caseRoot) || strings.Contains(runError.Error(), checkout) {
		return errors.New("replacement checkout identity error exposed a private path")
	}
	if _, statError := os.Stat(gateMarker); !errors.Is(statError, os.ErrNotExist) {
		return errors.New("conformance started a gate in the replacement checkout")
	}

	return nil
}

func requireV04ReplacedConformanceSharedRoot(root, replacement string) error {
	caseRoot := filepath.Join(root, strings.ReplaceAll(replacement, " ", "-"))
	manifest, _, err := conformanceFixtureV04(caseRoot)
	if err != nil {
		return err
	}
	original := manifest.SharedRoot + "-original"
	replacementTarget := manifest.SharedRoot + "-replacement"
	commandMarker := filepath.Join(caseRoot, "coordination-command-started")
	var script string
	switch replacement {
	case "an empty directory":
		script = `mv "$1" "$2" && mkdir "$1"`
	case "a symlink":
		script = `mv "$1" "$2" && mkdir "$3" && ln -s "$3" "$1"`
	default:
		return errors.New("unknown shared-root replacement fixture")
	}
	manifest.Consumers[0].Bootstrap = []conformance.Command{{Arguments: []string{
		shellPath, "-c", script, conformanceLabel, manifest.SharedRoot, original, replacementTarget,
	}}}
	manifest.CoordinationChecks = []conformance.Check{{
		Consumer: manifest.Consumers[0].Name,
		Command: conformance.Command{Arguments: []string{
			shellPath, "-c", childStartedArgScript, conformanceLabel, commandMarker,
		}},
	}}
	manifestPath := filepath.Join(caseRoot, "manifest.json")
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}

	runError := conformance.Run(context.Background(), manifestPath, &bytes.Buffer{})
	if runError == nil {
		return fmt.Errorf("conformance accepted shared-root replacement with %s", replacement)
	}
	if strings.Contains(runError.Error(), caseRoot) || strings.Contains(runError.Error(), manifest.SharedRoot) {
		return errors.New("shared-root identity error exposed a private path")
	}
	if _, statError := os.Stat(commandMarker); !errors.Is(statError, os.ErrNotExist) {
		return errors.New("conformance started coordination against the replacement shared root")
	}

	return nil
}

func requireV04ConformanceCancellation(root string) error {
	manifest, _, err := conformanceFixtureV04(root)
	if err != nil {
		return err
	}
	mutation := filepath.Join(manifest.Consumers[0].Path, "cancelled-mutation")
	descendantPID := filepath.Join(root, "descendant-pid")
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		shellPath, "-c",
		`printf changed > "$1"; sh -c 'trap "" TERM; printf "%s" "$$" > "$1"; while :; do sleep 0.05; done' child "$2" & wait`,
		conformanceLabel, mutation, descendantPID,
	}}}
	manifestPath := filepath.Join(root, "manifest.json")
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- conformance.Run(ctx, manifestPath, &bytes.Buffer{}) }()
	deadline := time.Now().Add(time.Second)
	var childPID int
	for {
		data, readError := os.ReadFile(descendantPID)
		if readError == nil {
			_, _ = fmt.Sscan(string(data), &childPID)
			if childPID > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			cancel()

			return errors.New("conformance descendant did not start")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	runError := <-done
	if runError == nil || !strings.Contains(runError.Error(), "checkout changed") {
		_ = syscall.Kill(childPID, syscall.SIGKILL)

		return fmt.Errorf("cancelled conformance did not reconcile with a fresh context: %w", runError)
	}
	deadline = time.Now().Add(time.Second)
	for {
		signalError := syscall.Kill(childPID, 0)
		if errors.Is(signalError, syscall.ESRCH) {
			return nil
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(childPID, syscall.SIGKILL)

			return errors.New("cancelled conformance left a live descendant")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func requireV04ReapedGroupRetirement(string) error {
	return errors.Join(
		runInternalConformanceRegressionV04("TestSignalProcessGroupAcceptsAnExitedButUnreapedGroup"),
		runInternalConformanceRegressionV04("TestCommandGroupRetirementToleratesDelayedReaping"),
	)
}

func runInternalConformanceRegressionV04(name string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	command := exec.Command("go", "test", "./internal/conformance", "-run", "^"+name+"$", "-count=1")
	command.Dir = moduleRoot
	if output, runError := command.CombinedOutput(); runError != nil {
		return fmt.Errorf("internal conformance regression %s: %s: %w", name, bytes.TrimSpace(output), runError)
	}

	return nil
}

func runIntegrationConformanceRegressionV04(name string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	command := exec.Command("go", "test", "./tests/integration", "-run", "^"+name+"$", "-count=1", "-v")
	command.Dir = moduleRoot
	if output, runError := command.CombinedOutput(); runError != nil {
		return fmt.Errorf("integration conformance regression %s: %s: %w", name, bytes.TrimSpace(output), runError)
	}

	return nil
}

func requireV04ConformanceLeaderFirstCancellation(string) error {
	return runIntegrationConformanceRegressionV04("TestCompiledConformanceLeaderFirstCancellationRetiresDescendant")
}

func requireV04ConformanceCompletedDescendant(_, exit string) error {
	return runIntegrationConformanceRegressionV04("TestCompiledConformanceCompletedLeaderRetiresDescendant/" + exit)
}

func requireV04TamperedPinnedBinary(string) error {
	return runIntegrationConformanceRegressionV04("TestCompiledConformanceRejectsTamperedPinnedBinary")
}

func requireV04ParallelPinnedBinary(string) error {
	return runIntegrationConformanceRegressionV04("TestCompiledConformanceParallelGateCannotReplacePinnedBinary")
}

func requireV04CapacitySkipIntegrity(failure string) error {
	return runIntegrationConformanceRegressionV04(
		"TestCompiledConformanceCapacitySkipDoesNotHideIntegrity/" + strings.ReplaceAll(failure, " ", "_"),
	)
}

func requireV04ConformanceSetupPrivacy(surface string) error {
	return runIntegrationConformanceRegressionV04(
		"TestCompiledConformanceSetupErrorsStayPrivate/" + strings.ReplaceAll(surface, " ", "_"),
	)
}

func requireV04BoundedConformanceReap(root string) error {
	if err := runInternalConformanceRegressionV04("TestExecuteBoundsPostKillReapFailure"); err != nil {
		return err
	}

	return requireV04ConformanceCancellation(filepath.Join(root, "real-cancellation"))
}

func requireV04VerifiedBinaryCleanupFailure(root string) error {
	manifest, _, err := conformanceFixtureV04(root)
	if err != nil {
		return err
	}
	storagePath := filepath.Join(root, "verified-storage-path")
	mutation := filepath.Join(manifest.Consumers[0].Path, "cleanup-mutation")
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		shellPath, "-c", `directory=$(dirname "$HIPPO_BIN"); printf '%s' "$directory" > "$1"; chmod 700 "$directory"; rm -f "$HIPPO_BIN"; rmdir "$directory"; ln -s "$1-missing-target" "$directory"; printf changed > "$2"; exit 9`,
		conformanceLabel, storagePath, mutation,
	}}}
	manifestPath := filepath.Join(root, "manifest.json")
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}
	runError := conformance.Run(context.Background(), manifestPath, &bytes.Buffer{})
	privateStorage, readError := os.ReadFile(storagePath)
	if readError != nil {
		return readError
	}
	defer func() {
		_ = os.Chmod(string(privateStorage), 0o700) //nolint:gosec // The path was emitted by this fixture's private temporary-copy command.
		_ = os.RemoveAll(string(privateStorage))    //nolint:gosec // The path was emitted by this fixture's private temporary-copy command.
	}()
	if runError == nil {
		return errors.New("verified binary cleanup failure was discarded")
	}
	errorText := runError.Error()
	if !strings.Contains(errorText, "verified HIPPO binary cleanup failed") ||
		!strings.Contains(errorText, `consumer "consumer-1" gate`) || !strings.Contains(errorText, "exit 9") ||
		!strings.Contains(errorText, "checkout changed") {
		return fmt.Errorf("verified binary cleanup failure was discarded: %w", runError)
	}
	if strings.Contains(errorText, string(privateStorage)) || strings.Contains(errorText, root) {
		return errors.New("verified binary cleanup failure exposed a private path")
	}

	return nil
}

func requireV04PreCancelledConformanceCommand(root string) error {
	if err := runInternalConformanceRegressionV04("TestExecuteDoesNotStartWithPreCancelledContext"); err != nil {
		return err
	}

	return requireV04ConformanceCancellation(filepath.Join(root, "reconciliation"))
}

func requireV04ConformanceBinaryReplacement(root string) error {
	manifest, binary, err := conformanceFixtureV04(root)
	if err != nil {
		return err
	}
	replacement := filepath.Join(root, "replacement")
	executed := filepath.Join(root, "changed-binary-executed")
	script := fmt.Sprintf("#!/bin/sh\nprintf executed > %q\n", executed)
	if err = os.WriteFile(replacement, []byte(script), 0o700); err != nil {
		return err
	}
	mutation := filepath.Join(manifest.Consumers[0].Path, "binary-mutation")
	manifest.Consumers[0].Bootstrap = []conformance.Command{{Arguments: []string{
		shellPath, "-c", `cp "$1" "$2"; printf changed > "$3"`, conformanceLabel, replacement, binary, mutation,
	}}}
	manifest.CoordinationChecks = []conformance.Check{{
		Consumer: manifest.Consumers[0].Name,
		Command:  conformance.Command{Arguments: []string{binary}},
	}}
	manifestPath := filepath.Join(root, "manifest.json")
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}
	runError := conformance.Run(context.Background(), manifestPath, &bytes.Buffer{})
	if runError == nil || !strings.Contains(runError.Error(), "binary") || !strings.Contains(runError.Error(), "checkout changed") {
		return fmt.Errorf("changed binary was not rejected and reconciled: %w", runError)
	}
	if _, statError := os.Stat(executed); !errors.Is(statError, os.ErrNotExist) {
		return errors.New("changed HIPPO bytes executed after bootstrap")
	}

	return nil
}

func requireV04PinnedBinaryIdentity(root string) error {
	manifest, binary, err := conformanceFixtureV04(root)
	if err != nil {
		return err
	}
	originalMarker := filepath.Join(root, "verified-original")
	replacementMarker := filepath.Join(root, "unverified-replacement")
	ready := filepath.Join(root, "command-ready")
	continued := filepath.Join(root, "command-continue")
	original := fmt.Sprintf("#!/bin/sh\nprintf original > %q\n", originalMarker)
	if err = os.WriteFile(binary, []byte(original), 0o700); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(original))
	manifest.HIPPOSHA256 = hex.EncodeToString(digest[:])
	replacement := filepath.Join(root, "replacement")
	replacementScript := fmt.Sprintf("#!/bin/sh\nprintf replacement > %q\n", replacementMarker)
	if err = os.WriteFile(replacement, []byte(replacementScript), 0o700); err != nil {
		return err
	}
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		shellPath, "-c", `printf ready > "$1"; while [ ! -f "$2" ]; do sleep 0.005; done; "$HIPPO_BIN"`,
		conformanceLabel, ready, continued,
	}}}
	manifestPath := filepath.Join(root, "manifest.json")
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- conformance.Run(context.Background(), manifestPath, &bytes.Buffer{}) }()
	deadline := time.Now().Add(time.Second)
	for {
		if _, statError := os.Stat(ready); statError == nil {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("conformance command did not reach the post-verification boundary")
		}
		time.Sleep(time.Millisecond)
	}
	if err = os.Rename(replacement, binary); err != nil {
		return err
	}
	if err = os.WriteFile(continued, []byte("continue\n"), 0o600); err != nil {
		return err
	}
	runError := <-done
	if runError == nil || !strings.Contains(runError.Error(), "binary identity changed") {
		return fmt.Errorf("changed source identity was not reported: %w", runError)
	}
	if _, statError := os.Stat(originalMarker); statError != nil {
		return fmt.Errorf("pinned verified binary did not execute: %w", statError)
	}
	if _, statError := os.Stat(replacementMarker); !errors.Is(statError, os.ErrNotExist) {
		return errors.New("changed source bytes executed after verification")
	}
	if strings.Contains(runError.Error(), binary) || strings.Contains(runError.Error(), root) {
		return errors.New("changed binary error exposed a private path")
	}

	return nil
}

func binaryValidationManifestV04(root, binary, checksum string) (string, error) {
	manifest, _, err := conformanceFixtureV04(filepath.Join(root, fixtureOwner))
	if err != nil {
		return "", err
	}
	manifest.HIPPOBinary = binary
	manifest.HIPPOSHA256 = checksum
	manifestPath := filepath.Join(root, "manifest.json")

	return manifestPath, writeConformanceManifestV04(manifestPath, manifest)
}

func requirePrivateBinaryValidationErrorV04(runError error, privatePath string) error {
	if runError == nil {
		return errors.New("invalid HIPPO binary identity was accepted")
	}
	if strings.Contains(runError.Error(), privatePath) {
		return errors.New("HIPPO binary validation error exposed its private path")
	}

	return nil
}

func requireV04ConformanceBinaryFIFO(root string) error {
	privatePath := filepath.Join(root, "private-binary-fifo")
	if err := unix.Mkfifo(privatePath, 0o600); err != nil {
		return err
	}
	manifestPath, err := binaryValidationManifestV04(root, privatePath, strings.Repeat("0", 64))
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- conformance.Run(context.Background(), manifestPath, &bytes.Buffer{}) }()
	select {
	case runError := <-done:
		return requirePrivateBinaryValidationErrorV04(runError, privatePath)
	case <-time.After(100 * time.Millisecond):
		writer, openError := os.OpenFile(privatePath, os.O_WRONLY, 0)
		if openError != nil {
			return openError
		}
		_, _ = writer.WriteString("not a binary")
		_ = writer.Close()
		<-done

		return errors.New("HIPPO FIFO validation blocked while opening or hashing the special file")
	}
}

func requireV04ConformanceBinaryDirectory(root string) error {
	privatePath := filepath.Join(root, "private-binary-directory")
	if err := os.MkdirAll(privatePath, 0o700); err != nil {
		return err
	}
	manifestPath, err := binaryValidationManifestV04(root, privatePath, strings.Repeat("0", 64))
	if err != nil {
		return err
	}

	return requirePrivateBinaryValidationErrorV04(conformance.Run(context.Background(), manifestPath, &bytes.Buffer{}), privatePath)
}

func requireV04ConformanceBinaryMode(root string) error {
	privatePath := filepath.Join(root, "private-non-executable")
	data := []byte("regular but not executable\n")
	if err := os.WriteFile(privatePath, data, 0o600); err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	manifestPath, err := binaryValidationManifestV04(root, privatePath, hex.EncodeToString(digest[:]))
	if err != nil {
		return err
	}

	return requirePrivateBinaryValidationErrorV04(conformance.Run(context.Background(), manifestPath, &bytes.Buffer{}), privatePath)
}

func requireSafeConformanceStartErrorV04(runError error, consumerName, phase string, privatePaths ...string) error {
	if runError == nil {
		return errors.New("invalid conformance command boundary was accepted")
	}
	message := runError.Error()
	if !strings.Contains(message, consumerName) || !strings.Contains(message, phase) || !strings.Contains(message, "start") {
		return fmt.Errorf("conformance start error omitted safe consumer/phase/category metadata: %w", runError)
	}
	for _, privatePath := range privatePaths {
		if strings.Contains(message, privatePath) {
			return errors.New("conformance start error exposed a private machine path")
		}
	}

	return nil
}

func requireSafeConformanceIdentityErrorV04(runError error, consumerName, phase string, privatePaths ...string) error {
	if runError == nil {
		return errors.New("changed conformance filesystem identity was accepted")
	}
	message := runError.Error()
	if !strings.Contains(message, consumerName) || !strings.Contains(message, phase) || !strings.Contains(message, "identity") {
		return fmt.Errorf("conformance identity error omitted safe consumer/phase/category metadata: %w", runError)
	}
	for _, privatePath := range privatePaths {
		if strings.Contains(message, privatePath) {
			return errors.New("conformance identity error exposed a private machine path")
		}
	}

	return nil
}

func requireV04MissingCommandPrivacy(root string) error {
	manifest, _, err := conformanceFixtureV04(root)
	if err != nil {
		return err
	}
	privateExecutable := filepath.Join(root, "private-missing-executable")
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{privateExecutable}}}
	manifestPath := filepath.Join(root, "manifest.json")
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}

	return requireSafeConformanceStartErrorV04(
		conformance.Run(context.Background(), manifestPath, &bytes.Buffer{}), manifest.Consumers[0].Name, "gate", privateExecutable, root,
	)
}

func requireV04InvalidCheckoutPrivacy(root string) error {
	manifest, _, err := conformanceFixtureV04(root)
	if err != nil {
		return err
	}
	privateCheckout := manifest.Consumers[0].Path
	movedCheckout := filepath.Join(root, "private-moved-checkout")
	manifest.Consumers[0].Bootstrap = []conformance.Command{{Arguments: []string{
		shellPath, "-c", `mv "$1" "$2"`, conformanceLabel, privateCheckout, movedCheckout,
	}}}
	manifestPath := filepath.Join(root, "manifest.json")
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}

	return requireSafeConformanceIdentityErrorV04(
		conformance.Run(context.Background(), manifestPath, &bytes.Buffer{}), manifest.Consumers[0].Name, "bootstrap", privateCheckout, movedCheckout, root,
	)
}

func requireV04ParallelConformanceBootstrap(root string) error {
	manifest, _, err := conformanceFixtureV04(root)
	if err != nil {
		return err
	}
	gateMarker := filepath.Join(root, "gate-started")
	firstPaths := make([]string, len(manifest.Consumers))
	for index := range manifest.Consumers {
		firstPaths[index] = filepath.Join(root, fmt.Sprintf("first-%d", index))
	}
	for index := range manifest.Consumers {
		first := firstPaths[index]
		second := filepath.Join(root, fmt.Sprintf("second-%d", index))
		failure := "exit 0"
		if index == 0 || index == 2 {
			failure = "exit 1"
		}
		manifest.Consumers[index].Bootstrap = []conformance.Command{
			{Arguments: []string{
				shellPath, "-c",
				`printf first > "$1"; attempts=0; while [ ! -f "$2" ] || [ ! -f "$3" ] || [ ! -f "$4" ] || [ ! -f "$5" ]; do attempts=$((attempts + 1)); [ "$attempts" -lt 200 ] || exit 91; sleep 0.005; done`,
				conformanceLabel, first, firstPaths[0], firstPaths[1], firstPaths[2], firstPaths[3],
			}},
			{Arguments: []string{shellPath, "-c", `test -f "$1"; printf second > "$2"; ` + failure, conformanceLabel, first, second}},
		}
		manifest.Consumers[index].Gates = []conformance.Command{{Arguments: []string{shellPath, "-c", `printf gate > "$1"`, conformanceLabel, gateMarker}}}
	}
	manifestPath := filepath.Join(root, "manifest.json")
	if err = writeConformanceManifestV04(manifestPath, manifest); err != nil {
		return err
	}
	runError := conformance.Run(context.Background(), manifestPath, &bytes.Buffer{})
	if runError == nil {
		return errors.New("multiple bootstrap failures were not aggregated")
	}
	errorText := runError.Error()
	firstFailure := strings.Index(errorText, `consumer "consumer-1"`)
	secondFailure := strings.Index(errorText, `consumer "consumer-3"`)
	if firstFailure < 0 || secondFailure <= firstFailure {
		return fmt.Errorf("bootstrap failures were not deterministic and complete: %w", runError)
	}
	for index := range manifest.Consumers {
		if _, statError := os.Stat(filepath.Join(root, fmt.Sprintf("second-%d", index))); statError != nil {
			return fmt.Errorf("consumer %d bootstrap lane did not run sequentially: %w", index+1, statError)
		}
	}
	if _, statError := os.Stat(gateMarker); !errors.Is(statError, os.ErrNotExist) {
		return errors.New("gate phase started after bootstrap failure")
	}

	return nil
}

func (driver *Driver) statusMarkerPrecedenceV04() error {
	if err := driver.preparePendingV04(); err != nil {
		return err
	}
	for _, name := range []string{"schema1-reservation", "schema2-exclusive", "schema1-empty", "schema2-empty"} {
		caseRoot := filepath.Join(driver.evidenceRoot, name)
		if err := os.MkdirAll(caseRoot, 0o700); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(driver.evidenceRoot, "schema1-reservation", "coordination-mode.json"), []byte(reviewReservationMarker+"\n"), 0o600); err != nil {
		return err
	}
	emptyLedger := `{"schemaVersion":2,"capacity":{"cpu":0,"memoryBytes":0},"nextSequence":0,"owners":[],"waiters":[]}`
	if err := os.WriteFile(filepath.Join(driver.evidenceRoot, "schema1-reservation", "reservations.json"), []byte(emptyLedger+"\n"), 0o600); err != nil {
		return err
	}
	exclusiveMarker := `{"schemaVersion":1,"mode":"exclusive"}`

	return os.WriteFile(filepath.Join(driver.evidenceRoot, "schema2-exclusive", "coordination-mode.json"), []byte(exclusiveMarker+"\n"), 0o600)
}

func (driver *Driver) requestStatusMarkerPrecedenceV04() error {
	for _, testCase := range []struct {
		name, schema, want string
	}{
		{name: "schema1-reservation", schema: schemaOneDocument, want: reservationMode},
		{name: "schema2-exclusive", schema: `{"schemaVersion":2}`, want: exclusiveMode},
		{name: "schema1-empty", schema: schemaOneDocument, want: exclusiveMode},
		{name: "schema2-empty", schema: `{"schemaVersion":2}`, want: reservationMode},
	} {
		configPath := filepath.Join(driver.evidenceRoot, testCase.name+".json")
		if err := os.WriteFile(configPath, []byte(testCase.schema), 0o600); err != nil {
			driver.v04Error = err

			return nil //nolint:nilerr // Step drivers record the production error for the subsequent Then assertion.
		}
		mode, err := driver.statusModeV04(configPath, filepath.Join(driver.evidenceRoot, testCase.name))
		if err != nil || mode != testCase.want {
			driver.v04Error = fmt.Errorf("%s status mode=%q want=%q error=%w", testCase.name, mode, testCase.want, err)

			return nil
		}
	}

	return nil
}

func (driver *Driver) requireStatusMarkerPrecedenceV04() error { return driver.v04Error }

func (driver *Driver) statusModeV04(configPath, root string) (string, error) {
	var output []byte
	if driver.mode == e2eMode {
		command := exec.Command(driver.binary, statusCommandName, jsonFlag, configFlag, configPath, diskPathFlag, ".")
		command.Env = append(os.Environ(), "HIPPO_ROOT="+root)
		var err error
		output, err = command.Output()
		if err != nil {
			return "", err
		}
	} else {
		base := time.Unix(0, 0)
		stdout := &bytes.Buffer{}
		code, err := (cli.Application{
			Stdout: stdout, Stderr: &bytes.Buffer{},
			Collector: &sequenceCollector{samples: []policy.Sample{healthySample(base), healthySample(base.Add(time.Second))}},
			Sleep:     func(time.Duration) {}, Environment: []string{"HIPPO_ROOT=" + root},
		}).Run(context.Background(), []string{statusCommandName, jsonFlag, configFlag, configPath, diskPathFlag, "."})
		if err != nil || code != 0 {
			return "", fmt.Errorf("status exit=%d: %w", code, err)
		}
		output = stdout.Bytes()
	}
	var payload struct {
		Coordination struct {
			Mode string `json:"mode"`
		} `json:"coordination"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return "", err
	}

	return payload.Coordination.Mode, nil
}

func (driver *Driver) compiledWaiterOverflowStatusV04() error {
	if err := driver.preparePendingV04(); err != nil {
		return err
	}
	if driver.mode == e2eMode {
		return driver.compiledBinary()
	}

	return nil
}

func (driver *Driver) requestCompiledWaiterOverflowStatusV04() error {
	root := driver.evidenceRoot
	maximum := guard.ReservationVector{CPU: math.MaxInt, MemoryBytes: math.MaxInt64}
	floor := guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
	owner, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, profileBalanced, "",
		guard.ReservationPlan{Capacity: maximum, Requested: floor, Allocated: floor}, 20, 0,
	)
	if err != nil {
		driver.v04Error = err

		return nil
	}
	defer func() { _ = guard.ReleaseReservation(root, owner) }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan error, 2)
	plan := guard.ReservationPlan{Capacity: maximum, Requested: maximum, Allocated: maximum}
	for range 2 {
		go func() {
			session, acquireError := guard.AcquireReservation(ctx, root, "", policy.TaskEphemeral, profileBalanced, "", plan, 20, 10*time.Second)
			if session != nil {
				_ = guard.ReleaseReservation(root, session) //nolint:contextcheck // Test cleanup must outlive the canceled contender context.
			}
			results <- acquireError
		}()
	}
	deadline := time.Now().Add(time.Second)
	for {
		data, readError := os.ReadFile(filepath.Join(root, "reservations.json"))
		if readError == nil && bytes.Count(data, []byte(`"profile":"balanced"`)) >= 3 {
			break
		}
		if time.Now().After(deadline) {
			driver.v04Error = errors.New("compiled status overflow waiters did not enqueue")

			return nil
		}
		time.Sleep(time.Millisecond)
	}
	configPath := filepath.Join(root, "status.json")
	if err = os.WriteFile(configPath, []byte(`{"schemaVersion":2}`), 0o600); err != nil {
		driver.v04Error = err

		return nil
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if driver.mode == e2eMode {
		command := exec.Command(driver.binary, statusCommandName, jsonFlag, configFlag, configPath, diskPathFlag, ".")
		command.Env, command.Stdout, command.Stderr = append(os.Environ(), "HIPPO_ROOT="+root), stdout, stderr
		err = command.Run()
		driver.exitCode = exitCode(err)
	} else {
		base := time.Unix(0, 0)
		driver.exitCode, err = (cli.Application{
			Stdout: stdout, Stderr: stderr, Sleep: func(time.Duration) {},
			Collector:   &sequenceCollector{samples: []policy.Sample{healthySample(base), healthySample(base.Add(time.Second))}},
			Environment: []string{"HIPPO_ROOT=" + root},
		}).Run(context.Background(), []string{statusCommandName, jsonFlag, configFlag, configPath, diskPathFlag, "."})
	}
	cancel()
	<-results
	<-results
	driver.output, driver.errorOutput = stdout.String(), stderr.String()
	if driver.exitCode == 0 || !strings.Contains(stderr.String(), "aggregate") || strings.Contains(stderr.String(), owner.Token) || strings.Contains(stderr.String(), root) {
		driver.v04Error = fmt.Errorf("overflowing waiter status was not a private failure: exit=%d stdout=%q stderr=%q error=%w", driver.exitCode, stdout.String(), stderr.String(), err)
	}

	return nil
}

func (driver *Driver) requireCompiledWaiterOverflowStatusV04() error { return driver.v04Error }

var (
	_ = conformance.Manifest{}
	_ = syscall.SIGTERM
	_ = unix.LOCK_EX
)
