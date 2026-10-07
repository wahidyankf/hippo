// Package application coordinates HIPPO use cases through owned ports.
package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/wahidyankf/hippo/internal/status"
)

// ConformanceCommand describes an argv-safe command.
type ConformanceCommand struct {
	Arguments []string `json:"arguments"`
}

// ConformanceConsumer contains a consumer's validated checkout and commands.
type ConformanceConsumer struct {
	Name               string               `json:"name"`
	Path               string               `json:"path"`
	Bootstrap          []ConformanceCommand `json:"bootstrap,omitempty"`
	Gates              []ConformanceCommand `json:"gates"`
	DeferralRetryProbe ConformanceCommand   `json:"deferralRetryProbe,omitzero"`
}

// ConformanceCheck describes a coordination command from a named consumer.
type ConformanceCheck struct {
	Consumer          string             `json:"consumer"`
	Command           ConformanceCommand `json:"command"`
	AllowCapacitySkip bool               `json:"allowCapacitySkip,omitempty"`
}

// ConformanceManifest contains the validated generic four-consumer input.
type ConformanceManifest struct {
	SchemaVersion      int                   `json:"schemaVersion"`
	HIPPOBinary        string                `json:"hippoBinary"`
	HIPPOSHA256        string                `json:"hippoSha256"`
	SharedRoot         string                `json:"sharedRoot"`
	Consumers          []ConformanceConsumer `json:"consumers"`
	CoordinationChecks []ConformanceCheck    `json:"coordinationChecks"`
}

// ConformanceCheckoutState is the portable checkout fingerprint.
type ConformanceCheckoutState struct{ Head, Dirty string }

// ConformanceDeferralFixture owns a private saturated admission fixture.
type ConformanceDeferralFixture struct {
	Root    func() string
	Release func() error
	Close   func() error
}

// ConformanceEnvironment opens a validated conformance session.
type ConformanceEnvironment interface {
	Open(ctx context.Context, path string) (ConformanceSession, error)
}

// ConformanceSession provides bounded environmental operations without exposing host handles.
type ConformanceSession struct {
	Manifest            func() ConformanceManifest
	Verify              func(name string) error
	VerifyBinary        func() error
	Snapshot            func(ctx context.Context, name string) (ConformanceCheckoutState, error)
	Execute             func(ctx context.Context, name string, command ConformanceCommand, root string, output io.Writer) error
	Receipts            func() (map[string]struct{}, error)
	NeverStartedReceipt func(before map[string]struct{}) (bool, error)
	DeferralFixture     func() (ConformanceDeferralFixture, error)
	Close               func() error
}

// ConformanceService executes manifest-driven consumer verification.
type ConformanceService struct {
	Environment ConformanceEnvironment
	Now         func() time.Time
	Wait        func(context.Context, time.Duration)
}

const (
	conformanceReconciliationLimit = 5 * time.Second
	conformanceDeferralProbeHold   = 2 * time.Second
	conformanceCapacityDiagnostic  = "hippo: [" + string(status.CodeLimitCapacityDeferred) + "]"
)

type conformanceLockedWriter struct {
	mutex  sync.Mutex
	target io.Writer
}

func (writer *conformanceLockedWriter) Write(data []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	return writer.target.Write(data)
}

// CleanConformanceCapacitySkip accepts only an unwrapped command exit with proof of no start.
func CleanConformanceCapacitySkip(err error, output []byte, verified bool) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		return len(children) == 1 && CleanConformanceCapacitySkip(children[0], output, verified)
	}
	// Wrapped errors deliberately fail the clean skip proof.
	failure, ok := err.(interface{ ConformanceFailure() (string, int, bool) })
	if !ok {
		return false
	}
	category, exit, hasCause := failure.ConformanceFailure()
	return category == "exited" && exit == status.LimitShed && !hasCause &&
		bytes.Contains(output, []byte(conformanceCapacityDiagnostic)) && verified
}

// Run executes phases and reconciles checkout state even when the caller cancels.
func (service ConformanceService) Run(ctx context.Context, path string, output io.Writer) (returnError error) {
	session, err := service.Environment.Open(ctx, path)
	if err != nil {
		return err
	}
	defer func() { returnError = errors.Join(returnError, session.Close()) }()
	if service.Now == nil {
		service.Now = time.Now
	}
	if service.Wait == nil {
		service.Wait = waitConformanceHold
	}
	manifest := session.Manifest()
	executionOutput := io.Discard
	if output != nil {
		executionOutput = &conformanceLockedWriter{target: output}
	}
	states := make(map[string]ConformanceCheckoutState, len(manifest.Consumers))
	for _, consumer := range manifest.Consumers {
		states[consumer.Name], err = session.Snapshot(ctx, consumer.Name)
		if err != nil {
			return err
		}
	}
	result := service.executeManifest(ctx, session, manifest, executionOutput)
	cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), conformanceReconciliationLimit)
	defer cancelCleanup()
	result = errors.Join(result, session.VerifyBinary(), session.Verify(""), reconcileConformanceCheckouts(cleanupContext, session, manifest.Consumers, states)) //nolint:contextcheck // Reconciliation deliberately survives caller cancellation.
	if result == nil && output != nil {
		_, result = fmt.Fprintln(output, "four-consumer conformance passed with unchanged checkouts")
	}
	return result
}

func reconcileConformanceCheckouts(ctx context.Context, session ConformanceSession, consumers []ConformanceConsumer, states map[string]ConformanceCheckoutState) error {
	var result error
	for _, consumer := range consumers {
		after, err := session.Snapshot(ctx, consumer.Name)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if after != states[consumer.Name] {
			result = errors.Join(result, fmt.Errorf("consumer %q checkout changed during conformance", consumer.Name))
		}
	}
	return result
}

func executeConformancePhase(ctx context.Context, session ConformanceSession, consumers []ConformanceConsumer, commands func(ConformanceConsumer) []ConformanceCommand, phase string, output io.Writer) error {
	errorsByConsumer := make([]error, len(consumers))
	var group sync.WaitGroup
	for index := range consumers {
		group.Go(func() {
			consumer := consumers[index]
			for _, command := range commands(consumer) {
				if err := session.Verify(consumer.Name); err != nil {
					errorsByConsumer[index] = fmt.Errorf("consumer %q %s failed: %w", consumer.Name, phase, err)
					return
				}
				if err := session.VerifyBinary(); err != nil {
					errorsByConsumer[index] = err
					return
				}
				executeError := session.Execute(ctx, consumer.Name, command, "", output)
				if err := errors.Join(executeError, session.Verify(consumer.Name)); err != nil {
					errorsByConsumer[index] = fmt.Errorf("consumer %q %s failed: %w", consumer.Name, phase, err)
					return
				}
			}
		})
	}
	group.Wait()
	return errors.Join(errorsByConsumer...)
}

func (service ConformanceService) verifyDeferralRetry(ctx context.Context, session ConformanceSession, consumer ConformanceConsumer, output io.Writer) (returnError error) {
	if len(consumer.DeferralRetryProbe.Arguments) == 0 {
		return nil
	}
	fixture, err := session.DeferralFixture()
	if err != nil {
		return err
	}
	defer func() { returnError = errors.Join(returnError, fixture.Close()) }()
	freed := make(chan error, 1)
	go func() {
		service.Wait(ctx, conformanceDeferralProbeHold)
		freed <- fixture.Release()
	}()
	started := service.Now()
	executeError := session.Execute(ctx, consumer.Name, consumer.DeferralRetryProbe, fixture.Root(), output)
	elapsed := service.Now().Sub(started)
	releaseError := <-freed
	if releaseError != nil {
		return releaseError
	}
	if executeError != nil {
		return fmt.Errorf("consumer %q did not survive a capacity deferral, so it would read exit 124 as an admission: %w", consumer.Name, executeError)
	}
	if elapsed < conformanceDeferralProbeHold {
		return fmt.Errorf("consumer %q probe returned in %s, before capacity could free, so it never waited on HIPPO admission", consumer.Name, elapsed.Round(time.Millisecond))
	}
	_, err = fmt.Fprintf(output, "consumer %q retried a capacity deferral instead of reading it as an admission\n", consumer.Name)
	return err
}

func executeConformanceCheck(ctx context.Context, session ConformanceSession, check ConformanceCheck, output io.Writer) error {
	receiptsBefore := map[string]struct{}{}
	if check.AllowCapacitySkip {
		var receiptError error
		receiptsBefore, receiptError = session.Receipts()
		if receiptError != nil {
			return fmt.Errorf("coordination check for consumer %q failed: %w", check.Consumer, receiptError)
		}
	}
	if err := session.Verify(check.Consumer); err != nil {
		return fmt.Errorf("coordination check for consumer %q failed: %w", check.Consumer, err)
	}
	if err := session.VerifyBinary(); err != nil {
		return err
	}
	commandOutput := &bytes.Buffer{}
	destination := io.Writer(commandOutput)
	if output != nil {
		destination = io.MultiWriter(output, commandOutput)
	}
	executeError := session.Execute(ctx, check.Consumer, check.Command, "", destination)
	joinedError := errors.Join(executeError, session.Verify(check.Consumer))
	if joinedError == nil {
		return nil
	}
	if check.AllowCapacitySkip {
		verified, receiptError := session.NeverStartedReceipt(receiptsBefore)
		if receiptError != nil {
			return fmt.Errorf("coordination check for consumer %q failed: %w", check.Consumer, receiptError)
		}
		if CleanConformanceCapacitySkip(joinedError, commandOutput.Bytes(), verified) {
			_, err := fmt.Fprintf(output, "coordination check for consumer %q skipped: live host capacity is unsuitable\n", check.Consumer)
			return err
		}
	}
	return fmt.Errorf("coordination check for consumer %q failed: %w", check.Consumer, joinedError)
}

func (service ConformanceService) executeManifest(ctx context.Context, session ConformanceSession, manifest ConformanceManifest, output io.Writer) error {
	if err := executeConformancePhase(ctx, session, manifest.Consumers, func(consumer ConformanceConsumer) []ConformanceCommand { return consumer.Bootstrap }, "bootstrap", output); err != nil {
		return err
	}
	for _, consumer := range manifest.Consumers {
		if err := service.verifyDeferralRetry(ctx, session, consumer, output); err != nil {
			return err
		}
		if err := session.Verify(consumer.Name); err != nil {
			return fmt.Errorf("deferral retry probe for consumer %q disturbed state: %w", consumer.Name, err)
		}
	}
	for _, check := range manifest.CoordinationChecks {
		if err := executeConformanceCheck(ctx, session, check, output); err != nil {
			return err
		}
	}
	return executeConformancePhase(ctx, session, manifest.Consumers, func(consumer ConformanceConsumer) []ConformanceCommand { return consumer.Gates }, "gate", output)
}

func waitConformanceHold(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
