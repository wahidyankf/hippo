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
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/conformance"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

const reviewReservationMarker = `{"schemaVersion":1,"mode":"reservation"}`

func (driver *Driver) malformedServiceCompatibilityV04() error {
	if err := driver.preparePendingV04(); err != nil {
		return err
	}
	sessions := filepath.Join(driver.evidenceRoot, "sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		return err
	}
	driver.configPath = filepath.Join(sessions, strings.Repeat("a", 32)+".json")
	driver.v04State = []byte("{\"schemaVersion\":1,\"pid\":")

	return os.WriteFile(driver.configPath, driver.v04State, 0o600)
}

func (driver *Driver) inspectMalformedServiceCompatibilityV04() error {
	driver.v04Session, driver.v04Error = guard.AcquireReservation(
		context.Background(), driver.evidenceRoot, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)

	return nil
}

func (driver *Driver) requireMalformedServiceCompatibilityV04() error {
	if driver.v04Session != nil || !guard.IsCoordinationDeferred(driver.v04Error) {
		return fmt.Errorf("malformed service compatibility state did not fail closed: session=%+v error=%w", driver.v04Session, driver.v04Error)
	}
	data, err := os.ReadFile(driver.configPath)
	if err != nil || !bytes.Equal(data, driver.v04State) {
		return fmt.Errorf("malformed service compatibility state changed: %q: %w", data, err)
	}

	return nil
}

func validReviewOwner(tokenValue, class string, sequence uint64, requested, allocated guard.ReservationVector) map[string]any {
	return map[string]any{
		tokenField: tokenValue, pidField: os.Getpid(), classField: class, profileField: profileBalanced,
		requestedField: requested, "allocated": allocated, sequenceField: sequence, maxActiveOwnersFld: 20,
	}
}

func validReviewWaiter(tokenValue string, sequence uint64, requested guard.ReservationVector) map[string]any {
	return map[string]any{
		tokenField: tokenValue, pidField: os.Getpid(), classField: string(policy.TaskEphemeral), profileField: profileBalanced,
		requestedField: requested, sequenceField: sequence, maxActiveOwnersFld: 20,
	}
}

func corruptReviewLedger(kind string) map[string]any {
	floor := guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
	ledger := map[string]any{
		schemaVersionField: 2,
		capacityField:      guard.ReservationVector{CPU: 4, MemoryBytes: policy.GiB},
		nextSequenceField:  uint64(2),
		ownersField:        []any{},
		waitersField:       []any{},
	}
	switch kind {
	case tokenField:
		ledger[ownersField] = []any{validReviewOwner("invalid", string(policy.TaskEphemeral), 1, floor, floor)}
	case classField:
		ledger[ownersField] = []any{validReviewOwner(strings.Repeat("a", 32), "unknown", 1, floor, floor)}
	case sequenceField:
		ledger[waitersField] = []any{
			validReviewWaiter(strings.Repeat("a", 32), 2, floor),
			validReviewWaiter(strings.Repeat("b", 32), 1, floor),
		}
	case "vector":
		negative := guard.ReservationVector{CPU: -1, MemoryBytes: 256 * policy.MiB}
		ledger[ownersField] = []any{validReviewOwner(strings.Repeat("a", 32), string(policy.TaskEphemeral), 1, negative, floor)}
	case "overflow":
		maximum := guard.ReservationVector{CPU: math.MaxInt, MemoryBytes: math.MaxInt64}
		ledger[capacityField] = maximum
		ledger[ownersField] = []any{
			validReviewOwner(strings.Repeat("a", 32), string(policy.TaskEphemeral), 1, maximum, maximum),
			validReviewOwner(strings.Repeat("b", 32), string(policy.TaskService), 2, floor, floor),
		}
	case "total":
		large := guard.ReservationVector{CPU: 3, MemoryBytes: 768 * policy.MiB}
		ledger[ownersField] = []any{
			validReviewOwner(strings.Repeat("a", 32), string(policy.TaskEphemeral), 1, large, large),
			validReviewOwner(strings.Repeat("b", 32), string(policy.TaskService), 2, large, large),
		}
	case "structure":
		expanded := guard.ReservationVector{CPU: 2, MemoryBytes: 512 * policy.MiB}
		owner := validReviewOwner(strings.Repeat("a", 32), string(policy.TaskEphemeral), 1, floor, expanded)
		owner["shedding"] = true
		ledger[ownersField] = []any{owner}
	}

	return ledger
}

func (driver *Driver) prepareCorruptReservationLedgerV04(kind string) error {
	if err := driver.preparePendingV04(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(driver.evidenceRoot, "coordination-mode.json"), []byte(reviewReservationMarker+"\n"), 0o600); err != nil {
		return err
	}
	data, err := json.Marshal(corruptReviewLedger(kind))
	if err != nil {
		return err
	}
	driver.configPath = filepath.Join(driver.evidenceRoot, "reservations.json")
	driver.v04State = append(driver.v04State[:0], data...)
	driver.v04State = append(driver.v04State, '\n')
	driver.requestedProfile = kind

	return os.WriteFile(driver.configPath, driver.v04State, 0o600)
}

func (driver *Driver) inspectCorruptReservationLedgerV04() error {
	_, driver.v04Error = guard.ReservationStatus(context.Background(), driver.evidenceRoot)

	return nil
}

func (driver *Driver) requireCorruptReservationLedgerV04(kind string) error {
	if driver.requestedProfile != kind {
		return fmt.Errorf("prepared corruption %q, want %q", driver.requestedProfile, kind)
	}
	if driver.v04Error == nil {
		return fmt.Errorf("%s-corrupt reservation ledger was accepted", kind)
	}
	data, err := os.ReadFile(driver.configPath)
	if err != nil || !bytes.Equal(data, driver.v04State) {
		return fmt.Errorf("%s-corrupt reservation ledger changed: %q: %w", kind, data, err)
	}

	return nil
}

func requireV04MaximumWidthAdmission(root string) error {
	maximum := guard.ReservationVector{CPU: math.MaxInt, MemoryBytes: math.MaxInt64}
	ownerPlan := guard.ReservationPlan{Capacity: maximum, Requested: maximum, Allocated: maximum}
	owner, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, profileBalanced, "", ownerPlan, 20, 0,
	)
	if err != nil || owner == nil {
		return fmt.Errorf("maximum-width owner: session=%+v error=%w", owner, err)
	}
	defer func() { _ = guard.ReleaseReservation(root, owner) }()
	minimum := guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
	candidate, acquireError := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		guard.ReservationPlan{Capacity: maximum, Requested: minimum, Allocated: minimum}, 20, 0,
	)
	if candidate != nil {
		_ = guard.ReleaseReservation(root, candidate)
	}
	if candidate != nil || !errors.Is(acquireError, guard.ErrReservationDeferred) {
		return fmt.Errorf("maximum-width allocation wrapped: session=%+v error=%w", candidate, acquireError)
	}
	totals, statusError := guard.ReservationStatus(context.Background(), root)
	if statusError != nil || totals.Allocated != maximum {
		return fmt.Errorf("maximum-width totals changed: totals=%+v error=%w", totals, statusError)
	}

	return nil
}

func (driver *Driver) mixedOwnerLimitsV04() error { return driver.preparePendingV04() }

func (driver *Driver) requestLooserOwnerLimitV04() error {
	root := driver.evidenceRoot
	plan := v04Plan(1, 256*policy.MiB)
	owner, err := guard.AcquireReservation(context.Background(), root, "", policy.TaskService, profileBalanced, "", plan, 20, 0)
	if err != nil || owner == nil {
		driver.v04Error = fmt.Errorf("initial owner admission: %w", err)

		return nil
	}
	strictResult, strictErrors := make(chan *guard.Session, 1), make(chan error, 1)
	go func() {
		strict, strictError := guard.AcquireReservation(
			context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "", plan, 1, time.Second,
		)
		strictResult <- strict
		strictErrors <- strictError
	}()
	deadline := time.Now().Add(time.Second)
	for {
		totals, statusError := guard.ReservationStatus(context.Background(), root)
		if statusError == nil && totals.WaitingOwners == 1 {
			break
		}
		if time.Now().After(deadline) {
			driver.v04Error = errors.New("strict owner-limit waiter did not enqueue")
			_ = guard.ReleaseReservation(root, owner)

			return nil
		}
		time.Sleep(time.Millisecond)
	}
	loose, acquireError := guard.AcquireReservation(context.Background(), root, "", policy.TaskTransactional, profileBalanced, "", plan, 20, 0)
	if loose != nil {
		_ = guard.ReleaseReservation(root, loose)
	}
	if loose != nil || !errors.Is(acquireError, guard.ErrReservationDeferred) {
		driver.v04Error = fmt.Errorf("looser configuration bypassed strict waiter: session=%+v error=%w", loose, acquireError)
	}
	if releaseError := guard.ReleaseReservation(root, owner); releaseError != nil && driver.v04Error == nil {
		driver.v04Error = releaseError
	}
	strict, strictError := <-strictResult, <-strictErrors
	if strictError != nil || strict == nil {
		if driver.v04Error == nil {
			driver.v04Error = fmt.Errorf("strict owner-limit waiter was not admitted: session=%+v error=%w", strict, strictError)
		}

		return nil
	}
	defer func() { _ = guard.ReleaseReservation(root, strict) }()
	loose, acquireError = guard.AcquireReservation(context.Background(), root, "", policy.TaskTransactional, profileBalanced, "", plan, 20, 0)
	if loose != nil {
		_ = guard.ReleaseReservation(root, loose)
	}
	if loose != nil || !errors.Is(acquireError, guard.ErrReservationDeferred) {
		driver.v04Error = fmt.Errorf("looser configuration bypassed strict live owner: session=%+v error=%w", loose, acquireError)
	}

	return nil
}

func (driver *Driver) requireConservativeOwnerLimitV04() error { return driver.v04Error }

func requireV04ActiveEpochCapacity(root string) error { //nolint:cyclop,gocognit // The matrix keeps CPU and memory lower-cap owner/waiter epochs symmetrical.
	for _, testCase := range []struct {
		name     string
		capacity guard.ReservationVector
	}{
		{name: "cpu", capacity: guard.ReservationVector{CPU: 2, MemoryBytes: policy.GiB}},
		{name: "memory", capacity: guard.ReservationVector{CPU: 4, MemoryBytes: 384 * policy.MiB}},
	} {
		caseRoot := filepath.Join(root, testCase.name)
		originalCapacity := guard.ReservationVector{CPU: 4, MemoryBytes: policy.GiB}
		ownerVector := guard.ReservationVector{CPU: 3, MemoryBytes: 512 * policy.MiB}
		owner, err := guard.AcquireReservation(
			context.Background(), caseRoot, "", policy.TaskService, profileBalanced, "",
			guard.ReservationPlan{Capacity: originalCapacity, Requested: ownerVector, Allocated: ownerVector}, 20, 0,
		)
		if err != nil || owner == nil {
			return fmt.Errorf("%s active-epoch owner: session=%+v error=%w", testCase.name, owner, err)
		}
		waiterResult, waiterErrors := make(chan *guard.Session, 1), make(chan error, 1)
		go func() {
			request := guard.ReservationVector{CPU: 2, MemoryBytes: 512 * policy.MiB}
			waiter, waiterError := guard.AcquireReservation(
				context.Background(), caseRoot, "", policy.TaskEphemeral, profileBalanced, "",
				guard.ReservationPlan{Capacity: originalCapacity, Requested: request, Allocated: request}, 20, 2*time.Second,
			)
			waiterResult <- waiter
			waiterErrors <- waiterError
		}()
		deadline := time.Now().Add(time.Second)
		for {
			totals, statusError := guard.ReservationStatus(context.Background(), caseRoot)
			if statusError == nil && totals.WaitingOwners == 1 {
				break
			}
			if time.Now().After(deadline) {
				_ = guard.ReleaseReservation(caseRoot, owner)

				return fmt.Errorf("%s active-epoch waiter did not enqueue", testCase.name)
			}
			time.Sleep(time.Millisecond)
		}
		ledgerPath := filepath.Join(caseRoot, "reservations.json")
		before, err := os.ReadFile(ledgerPath)
		if err != nil {
			_ = guard.ReleaseReservation(caseRoot, owner)

			return err
		}
		minimum := guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
		lower, lowerError := guard.AcquireReservation(
			context.Background(), caseRoot, "", policy.TaskTransactional, profileBalanced, "",
			guard.ReservationPlan{Capacity: testCase.capacity, Requested: minimum, Allocated: minimum}, 20, 100*time.Millisecond,
		)
		if lower != nil || !errors.Is(lowerError, guard.ErrReservationDeferred) {
			if lower != nil {
				_ = guard.ReleaseReservation(caseRoot, lower)
			}
			_ = guard.ReleaseReservation(caseRoot, owner)

			return fmt.Errorf("%s lower cap was not deferred: session=%+v error=%w", testCase.name, lower, lowerError)
		}
		after, readError := os.ReadFile(ledgerPath)
		totals, statusError := guard.ReservationStatus(context.Background(), caseRoot)
		if readError != nil || statusError != nil || !bytes.Equal(before, after) || totals.Capacity != originalCapacity || totals.WaitingOwners != 1 {
			//nolint:errorlint // The diagnostic intentionally reports both persisted and decoded active-epoch outcomes.
			return fmt.Errorf("%s lower cap changed active epoch: before=%q after=%q totals=%+v read=%v status=%w", testCase.name, before, after, totals, readError, statusError)
		}
		if err = guard.ReleaseReservation(caseRoot, owner); err != nil {
			return err
		}
		waiter, waiterError := <-waiterResult, <-waiterErrors
		if waiterError != nil || waiter == nil {
			return fmt.Errorf("%s preserved FIFO waiter was not admitted: session=%+v error=%w", testCase.name, waiter, waiterError)
		}
		if err = guard.ReleaseReservation(caseRoot, waiter); err != nil {
			return err
		}
		lower, lowerError = guard.AcquireReservation(
			context.Background(), caseRoot, "", policy.TaskTransactional, profileBalanced, "",
			guard.ReservationPlan{Capacity: testCase.capacity, Requested: minimum, Allocated: minimum}, 20, 0,
		)
		if lowerError != nil || lower == nil {
			return fmt.Errorf("%s lower cap did not start after idle: session=%+v error=%w", testCase.name, lower, lowerError)
		}
		totals, statusError = guard.ReservationStatus(context.Background(), caseRoot)
		if statusError != nil || totals.Capacity != testCase.capacity {
			_ = guard.ReleaseReservation(caseRoot, lower)

			return fmt.Errorf("%s idle epoch capacity=%+v want=%+v error=%w", testCase.name, totals.Capacity, testCase.capacity, statusError)
		}
		if err = guard.ReleaseReservation(caseRoot, lower); err != nil {
			return err
		}
	}

	return nil
}

func (driver *Driver) remoteSheddingOwnerV04() error { return driver.preparePendingV04() }

func (driver *Driver) exerciseRemoteSheddingV04() error {
	return driver.exerciseOwnerSideSheddingV04(guard.CapacityDeferredExitCode)
}

func (driver *Driver) exerciseOwnerSideSheddingV04(selectedExit int) error {
	root := driver.evidenceRoot
	marker := filepath.Join(root, "owner-term")
	ready := filepath.Join(root, "owner-ready")
	settings := v04FastPolicy()
	settings.SampleInterval = 2 * time.Millisecond
	settings.TerminationGrace = 50 * time.Millisecond
	settings.AdmissionWindow = 100 * time.Millisecond
	type runResult struct {
		code int
		err  error
	}
	result := make(chan runResult, 1)
	base := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	finished := false
	defer func() {
		cancel()
		if !finished {
			select {
			case <-result:
			case <-time.After(2 * time.Second):
			}
		}
	}()
	go func() {
		code, runError := guard.Run(ctx, guard.RunConfig{
			Command: shellPath, Arguments: []string{"-c", `trap 'printf x > "$OWNER_TERM_MARKER"' TERM; printf ready > "$OWNER_READY_MARKER"; while :; do sleep 0.01; done`},
			TaskClass: policy.TaskEphemeral, Environment: append(os.Environ(), "OWNER_TERM_MARKER="+marker, "OWNER_READY_MARKER="+ready),
			EvidenceRoot: root, DiskPath: ".", Collector: &sequenceCollector{samples: []policy.Sample{
				healthySample(base), healthySample(base.Add(time.Millisecond)), healthySample(base.Add(2 * time.Millisecond)),
			}}, Policy: settings, Resolution: policy.Resolution{
				RequestedProfile: profileBalanced, ResolvedProfile: profileBalanced, Concurrency: 1,
			}, ReservationPolicy: v04ReservationPolicy(), ReservationPlan: v04Plan(1, 256*policy.MiB),
			ChildStdin: bytes.NewBuffer(nil), ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		})
		result <- runResult{code: code, err: runError}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, statError := os.Stat(ready); statError == nil {
			break
		}
		if time.Now().After(deadline) {
			driver.v04Error = errors.New("owner-side shedding child did not install its TERM trap")

			return nil
		}
		time.Sleep(time.Millisecond)
	}
	deadline = time.Now().Add(time.Second)
	var victim guard.ReservationOwner
	for {
		candidate, selected, selectError := guard.SelectPressureVictim(root, selectedExit)
		if selectError != nil {
			driver.v04Error = selectError

			return nil
		}
		if selected {
			victim = candidate

			break
		}
		if time.Now().After(deadline) {
			driver.v04Error = errors.New("owner-side shedding candidate was not activated")

			return nil
		}
		time.Sleep(time.Millisecond)
	}
	if next, selected, selectError := guard.SelectPressureVictim(root, selectedExit); selectError != nil || selected {
		driver.v04Error = fmt.Errorf("additional victim cascaded while first remained owned: %+v selected=%v error=%w", next, selected, selectError)
	}
	if observeError := guard.WaitPressureVictimRelease(root, victim, time.Second); observeError != nil && driver.v04Error == nil {
		driver.v04Error = observeError
	}
	run := <-result
	finished = true
	if (run.err != nil || run.code != selectedExit) && driver.v04Error == nil {
		driver.v04Error = fmt.Errorf("owner-side shedding exit=%d want=%d error=%w", run.code, selectedExit, run.err)
	}
	if _, statError := os.Stat(marker); statError != nil && driver.v04Error == nil {
		driver.v04Error = fmt.Errorf("owning guard did not deliver TERM before bounded KILL: %w", statError)
	}
	totals, statusError := guard.ReservationStatus(context.Background(), root)
	if (statusError != nil || totals.ActiveOwners != 0) && driver.v04Error == nil {
		driver.v04Error = fmt.Errorf("owner was not reaped and released: totals=%+v error=%w", totals, statusError)
	}

	return nil
}

func (driver *Driver) requireRemoteSheddingV04() error { return driver.v04Error }

func (driver *Driver) storageSheddingOwnerV04() error { return driver.preparePendingV04() }

func (driver *Driver) exerciseStorageOwnerSheddingV04() error {
	return driver.exerciseOwnerSideSheddingV04(guard.StorageBlockedExitCode)
}

func (driver *Driver) requireStorageOwnerSheddingV04() error { return driver.v04Error }

func (driver *Driver) replacedRemoteOwnerV04() error { return driver.preparePendingV04() }

func (driver *Driver) exerciseReplacedRemoteOwnerV04() error {
	root := driver.evidenceRoot
	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		driver.v04Error = err

		return nil
	}
	command := exec.Command(shellPath, "-c", `while :; do sleep 1; done`)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = command.Start(); err != nil {
		_ = guard.ReleaseReservation(root, session)
		driver.v04Error = err

		return nil
	}
	cleanup := func() { _ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	defer cleanup()
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	if err = guard.ActivateReservation(root, session, command.Process.Pid); err != nil {
		_ = guard.ReleaseReservation(root, session)
		driver.v04Error = err

		return nil
	}
	victim, selected, selectError := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
	if selectError != nil || !selected {
		_ = guard.ReleaseReservation(root, session)
		driver.v04Error = fmt.Errorf("select replaceable victim: selected=%v error=%w", selected, selectError)

		return nil
	}
	if err = guard.ReleaseReservation(root, session); err != nil {
		driver.v04Error = err

		return nil
	}
	completeError := guard.WaitPressureVictimRelease(root, victim, 20*time.Millisecond)
	select {
	case waitError := <-exited:
		driver.v04Error = fmt.Errorf("unowned process group was signaled after identity replacement: %w", waitError)
	case <-time.After(40 * time.Millisecond):
		if completeError != nil {
			driver.v04Error = fmt.Errorf("identity disappearance did not end observation: %w", completeError)
		}
		cleanup()
		<-exited
	}

	return nil
}

func (driver *Driver) requireReplacedRemoteOwnerV04() error { return driver.v04Error }

func (driver *Driver) unresponsiveRemoteOwnerV04() error { return driver.preparePendingV04() }

func (driver *Driver) exerciseUnresponsiveRemoteOwnerV04() error {
	root := driver.evidenceRoot
	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		driver.v04Error = err

		return nil
	}
	defer func() { _ = guard.ReleaseReservation(root, session) }()
	command := exec.Command(shellPath, "-c", `while :; do sleep 1; done`)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = command.Start(); err != nil {
		driver.v04Error = err

		return nil
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	defer func() {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		select {
		case <-exited:
		case <-time.After(time.Second):
		}
	}()
	if err = guard.ActivateReservation(root, session, command.Process.Pid); err != nil {
		driver.v04Error = err

		return nil
	}
	victim, selected, selectionError := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
	if selectionError != nil || !selected {
		driver.v04Error = fmt.Errorf("select unresponsive remote owner: selected=%v error=%w", selected, selectionError)

		return nil
	}
	waitError := guard.WaitPressureVictimRelease(root, victim, 20*time.Millisecond)
	select {
	case childError := <-exited:
		driver.v04Error = fmt.Errorf("remote selector signaled another owner's child: %w", childError)
	case <-time.After(40 * time.Millisecond):
		if waitError == nil {
			driver.v04Error = errors.New("unresponsive owner did not produce a bounded observation error")
		}
		if next, nextSelected, nextError := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode); nextError != nil || nextSelected {
			driver.v04Error = fmt.Errorf("unresponsive shedding owner did not remain global barrier: victim=%+v selected=%v error=%w", next, nextSelected, nextError)
		}
	}

	return nil
}

func (driver *Driver) requireUnresponsiveRemoteOwnerV04() error { return driver.v04Error }

func reviewManifestBinary() (string, string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(data)

	return binary, hex.EncodeToString(digest[:]), nil
}

func (driver *Driver) aliasedConsumerManifestV04() error {
	if err := driver.preparePendingV04(); err != nil {
		return err
	}
	target := filepath.Join(driver.evidenceRoot, "target")
	alias := filepath.Join(driver.evidenceRoot, "alias")
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	if err := os.Symlink(target, alias); err != nil {
		return err
	}
	binary, checksum, err := reviewManifestBinary()
	if err != nil {
		return err
	}
	manifest := conformance.Manifest{
		SchemaVersion: 1, HIPPOBinary: binary, HIPPOSHA256: checksum,
		SharedRoot: filepath.Join(driver.evidenceRoot, "shared"),
	}
	for index, path := range []string{target, alias, filepath.Join(driver.evidenceRoot, "third"), filepath.Join(driver.evidenceRoot, "fourth")} {
		if err = os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		manifest.Consumers = append(manifest.Consumers, conformance.Consumer{
			Name: fmt.Sprintf("consumer-%d", index+1), Path: path,
			Gates: []conformance.Command{{Arguments: []string{trueCommandName}}},
		})
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	driver.configPath = filepath.Join(driver.evidenceRoot, "alias-manifest.json")
	driver.v04State = []byte(target)

	return os.WriteFile(driver.configPath, data, 0o600)
}

func (driver *Driver) validateAliasedConsumerManifestV04() error {
	driver.v04Error = conformance.Run(context.Background(), driver.configPath, &bytes.Buffer{})

	return nil
}

func (driver *Driver) requireAliasedConsumerRejectedV04() error {
	if driver.v04Error == nil || !strings.Contains(driver.v04Error.Error(), "unique") {
		return fmt.Errorf("canonical checkout alias was not rejected as a duplicate: %w", driver.v04Error)
	}
	if strings.Contains(driver.v04Error.Error(), string(driver.v04State)) {
		return fmt.Errorf("canonical checkout alias error exposed a path: %w", driver.v04Error)
	}

	return nil
}

func initializeReviewCheckout(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	for _, arguments := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-q", "--allow-empty", "-m", fixtureOwner},
	} {
		command := exec.Command("git", arguments...)
		command.Dir = path
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("initialize checkout: %s: %w", output, err)
		}
	}

	return nil
}

func (driver *Driver) failingConformancePhasesV04() error { return driver.preparePendingV04() }

func reviewFailureManifest(caseRoot, binary, checksum, phase string) (conformance.Manifest, error) {
	manifest := conformance.Manifest{
		SchemaVersion: 1, HIPPOBinary: binary, HIPPOSHA256: checksum, SharedRoot: filepath.Join(caseRoot, "shared"),
	}
	for index := range 4 {
		path := filepath.Join(caseRoot, fmt.Sprintf("consumer-%d", index+1))
		if err := initializeReviewCheckout(path); err != nil {
			return conformance.Manifest{}, err
		}
		manifest.Consumers = append(manifest.Consumers, conformance.Consumer{
			Name: fmt.Sprintf("consumer-%d", index+1), Path: path,
			Gates: []conformance.Command{{Arguments: []string{trueCommandName}}},
		})
	}
	mutation := filepath.Join(manifest.Consumers[3].Path, "mutation")
	failure := conformance.Command{
		Arguments: []string{shellPath, "-c", `printf changed > "$1"; exit 1`, conformanceLabel, mutation},
	}
	switch phase {
	case "bootstrap":
		manifest.Consumers[0].Bootstrap = []conformance.Command{failure}
	case "coordination":
		manifest.CoordinationChecks = []conformance.Check{{Consumer: manifest.Consumers[0].Name, Command: failure}}
	case "gate":
		manifest.Consumers[0].Gates = []conformance.Command{failure}
	}

	return manifest, nil
}

func (driver *Driver) exerciseFailingConformancePhasesV04() error {
	binary, checksum, err := reviewManifestBinary()
	if err != nil {
		driver.v04Error = err

		return nil //nolint:nilerr // Step drivers record the production error for the subsequent Then assertion.
	}
	for _, phase := range []string{"bootstrap", "coordination", "gate"} {
		caseRoot := filepath.Join(driver.evidenceRoot, phase+"-case")
		manifest, manifestError := reviewFailureManifest(caseRoot, binary, checksum, phase)
		if manifestError != nil {
			driver.v04Error = manifestError

			return nil //nolint:nilerr // Step drivers record the production error for the subsequent Then assertion.
		}
		data, marshalError := json.Marshal(manifest)
		if marshalError != nil {
			driver.v04Error = marshalError

			return nil //nolint:nilerr // Step drivers record the production error for the subsequent Then assertion.
		}
		path := filepath.Join(caseRoot, "manifest.json")
		if writeError := os.WriteFile(path, data, 0o600); writeError != nil {
			driver.v04Error = writeError

			return nil //nolint:nilerr // Step drivers record the production error for the subsequent Then assertion.
		}
		runError := conformance.Run(context.Background(), path, &bytes.Buffer{})
		if runError == nil || !strings.Contains(runError.Error(), phase) || !strings.Contains(runError.Error(), "checkout changed") {
			driver.v04Error = fmt.Errorf("%s failure did not reconcile every pre-snapshotted checkout: %w", phase, runError)

			return nil
		}
	}

	return nil
}

func (driver *Driver) requireFailingConformanceReconciledV04() error { return driver.v04Error }

func (driver *Driver) corruptStatusCoordinationV04() error {
	if err := driver.preparePendingV04(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(driver.evidenceRoot, "coordination-mode.json"), []byte(reviewReservationMarker+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(driver.evidenceRoot, "reservations.json"), []byte("corrupt-ledger\n"), 0o600); err != nil {
		return err
	}
	driver.configPath = filepath.Join(driver.evidenceRoot, "hippo.json")

	return os.WriteFile(driver.configPath, []byte(`{"schemaVersion":2}`), 0o600)
}

func (driver *Driver) requestCorruptCoordinationStatusV04() error {
	if driver.mode == e2eMode {
		command := exec.Command(driver.binary, statusCommandName, jsonFlag, configFlag, driver.configPath, diskPathFlag, ".")
		command.Env = append(os.Environ(), "HIPPO_ROOT="+driver.evidenceRoot)
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		command.Stdout, command.Stderr = stdout, stderr
		err := command.Run()
		driver.output, driver.errorOutput = stdout.String(), stderr.String()
		if err != nil {
			driver.exitCode = 1
			if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
				driver.exitCode = exitError.ExitCode()
			}
		}

		return nil
	}
	base := time.Unix(0, 0)
	collector := &sequenceCollector{samples: []policy.Sample{healthySample(base), healthySample(base.Add(time.Second))}}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, err := (cli.Application{
		Stdout: stdout, Stderr: stderr, Collector: collector, Sleep: func(time.Duration) {},
		Environment: []string{"HIPPO_ROOT=" + driver.evidenceRoot},
	}).Run(context.Background(), []string{statusCommandName, jsonFlag, configFlag, driver.configPath, diskPathFlag, "."})
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()
	driver.v04Error = err

	return nil
}

func (driver *Driver) requireCorruptCoordinationStatusV04() error {
	if driver.exitCode == 0 || driver.v04Error == nil && driver.mode != e2eMode ||
		strings.Contains(driver.output, `"coordination"`) || !strings.Contains(driver.errorOutput, "coordination status") {
		return fmt.Errorf("corrupt coordination became status success/zero totals: exit=%d stdout=%q stderr=%q error=%w", driver.exitCode, driver.output, driver.errorOutput, driver.v04Error)
	}

	return nil
}

type peakReviewCollector struct {
	root    string
	index   int
	second  *guard.Session
	acquire error
}

func (collector *peakReviewCollector) Collect(ctx context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	if err := ctx.Err(); err != nil {
		return policy.Reading{}, err
	}
	if collector.index == 3 && collector.second == nil && collector.acquire == nil {
		collector.second, collector.acquire = guard.AcquireReservation(
			ctx, collector.root, "", policy.TaskService, profileBalanced, "", v04Plan(1, 256*policy.MiB), 20, 0,
		)
	}
	collector.index++

	return policy.Reading{CPUState: previous, Sample: healthySample(time.Now())}, collector.acquire
}

func (driver *Driver) risingOwnerCountV04() error { return driver.preparePendingV04() }

func (driver *Driver) sampleLifetimeOwnerPeakV04() error {
	collector := &peakReviewCollector{root: driver.evidenceRoot}
	policySettings := v04FastPolicy()
	policySettings.SampleInterval = 2 * time.Millisecond
	policySettings.AdmissionWindow = 100 * time.Millisecond
	exitCode, runError := guard.Run(context.Background(), guard.RunConfig{
		Command: shellPath, Arguments: []string{"-c", "sleep 0.05"}, TaskClass: policy.TaskEphemeral,
		EvidenceRoot: driver.evidenceRoot, Collector: collector, Policy: policySettings,
		Resolution:        policy.Resolution{RequestedProfile: profileBalanced, ResolvedProfile: profileBalanced, Concurrency: 1},
		ReservationPolicy: v04ReservationPolicy(), ReservationPlan: v04Plan(1, 256*policy.MiB),
		ChildStdin: bytes.NewBuffer(nil), ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
	})
	if collector.second != nil {
		defer func() { _ = guard.ReleaseReservation(driver.evidenceRoot, collector.second) }()
	}
	if runError != nil || exitCode != 0 {
		driver.v04Error = fmt.Errorf("peak-owner guarded run: exit=%d error=%w", exitCode, runError)

		return nil
	}
	entries, err := os.ReadDir(driver.evidenceRoot)
	if err != nil {
		driver.v04Error = err

		return nil
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".summary.json") {
			continue
		}
		data, readError := os.ReadFile(filepath.Join(driver.evidenceRoot, entry.Name()))
		if readError != nil {
			driver.v04Error = readError

			return nil
		}
		var summary guard.EvidenceSummary
		if decodeError := json.Unmarshal(data, &summary); decodeError != nil {
			driver.v04Error = decodeError

			return nil
		}
		if summary.PeakOwnerCount != 2 {
			driver.v04Error = fmt.Errorf("lifetime peak owners=%d, want 2", summary.PeakOwnerCount)
		}

		return nil
	}
	driver.v04Error = errors.New("development summary was not written")

	return nil
}

func (driver *Driver) requireLifetimeOwnerPeakV04() error { return driver.v04Error }
