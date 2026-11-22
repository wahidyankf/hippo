package conformance

import (
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/application"
)

// Environment materializes validated manifests and their environmental resources. Host is the system every session
// reads, verifies, and runs commands through; the zero value has no ports and must not open a session.
type Environment struct {
	Host system.System
}

type session struct {
	host               system.System
	manifest           Manifest
	identity           binaryIdentity
	checkoutIdentities map[string]checkoutIdentity
	sharedRootIdentity checkoutIdentity
	environment        []string
}

// Open validates input and captures concrete binary, checkout and shared-root identities.
func (source Environment) Open(_ context.Context, manifestPath string) (application.ConformanceSession, error) {
	host := source.Host
	data, err := host.Files.ReadFile(filepath.Clean(manifestPath))
	if err != nil {
		return application.ConformanceSession{}, errors.New("conformance manifest is unavailable")
	}
	manifest, err := decodeManifest(data)
	if err != nil {
		return application.ConformanceSession{}, err
	}
	if err = validateRawManifest(manifest); err != nil {
		return application.ConformanceSession{}, err
	}
	manifest, err = canonicalizeManifest(host, manifest, filepath.Dir(filepath.Clean(manifestPath)))
	if err != nil {
		return application.ConformanceSession{}, err
	}
	if err = validateManifest(host, manifest); err != nil {
		return application.ConformanceSession{}, err
	}
	identities, err := captureCheckoutIdentities(host, manifest.Consumers)
	if err != nil {
		return application.ConformanceSession{}, err
	}
	identity, err := captureBinaryIdentity(host, manifest)
	if err != nil {
		return application.ConformanceSession{}, err
	}
	if err = host.Files.MkdirAll(filepath.Clean(manifest.SharedRoot), 0o700); err != nil {
		return application.ConformanceSession{}, errors.Join(errors.New("conformance shared root is unavailable"), cleanupBinary(host, identity))
	}
	sharedIdentity, err := captureSharedRootIdentity(host, manifest.SharedRoot)
	if err != nil {
		return application.ConformanceSession{}, errors.Join(err, cleanupBinary(host, identity))
	}
	environment := removeEnvironment(host.Environment.Environ(), "HIPPO_SESSION", "HIPPO_PROFILE", "HIPPO_CONCURRENCY", "HIPPO_RESERVED_MEMORY_BYTES", "HIPPO_DEFAULT_CONFIG", "HIPPO_DEFAULT_IDENTITY")
	environment = replaceEnvironment(environment, "HIPPO_ROOT", manifest.SharedRoot)
	run := &session{host: host, manifest: manifest, identity: identity, checkoutIdentities: identities, sharedRootIdentity: sharedIdentity, environment: environment}
	return run.capabilities(), nil
}

func cleanupBinary(host system.System, identity binaryIdentity) error {
	if err := identity.cleanup(host); err != nil {
		return errors.New("verified HIPPO binary cleanup failed")
	}
	return nil
}
func (run *session) Manifest() application.ConformanceManifest { return run.manifest }
func (run *session) consumer(name string) Consumer {
	for _, consumer := range run.manifest.Consumers {
		if consumer.Name == name {
			return consumer
		}
	}
	return Consumer{}
}

func (run *session) Verify(name string) error {
	var checkoutError error
	if name != "" {
		checkoutError = verifyCheckoutIdentity(run.host, run.consumer(name), run.checkoutIdentities[name])
	}
	return errors.Join(checkoutError, verifySharedRootIdentity(run.host, run.manifest.SharedRoot, run.sharedRootIdentity))
}
func (run *session) VerifyBinary() error { return run.identity.verify(run.host) }
func (run *session) Snapshot(ctx context.Context, name string) (application.ConformanceCheckoutState, error) {
	state, err := snapshot(run.host, ctx, run.consumer(name), run.checkoutIdentities[name])
	return application.ConformanceCheckoutState{Head: state.head, Dirty: state.dirty}, err
}

func (run *session) Execute(ctx context.Context, name string, command application.ConformanceCommand, root string, output io.Writer) error {
	environment := run.environment
	if root != "" {
		environment = replaceEnvironment(environment, "HIPPO_ROOT", root)
	}
	return executeVerifiedCommand(run.host, ctx, run.consumer(name).Path, command, environment, output, run.identity)
}

func (run *session) Receipts() (map[string]struct{}, error) {
	return receiptSnapshot(run.host, run.manifest.SharedRoot)
}

func (run *session) NeverStartedReceipt(before map[string]struct{}) (bool, error) {
	return newNeverStartedReceipt(run.host, run.manifest.SharedRoot, before)
}
func (run *session) Close() error { return cleanupBinary(run.host, run.identity) }

const (
	capacityHoldName        = "conformance-capacity-held"
	neverStartedReceiptName = "conformance-never-started-receipt.json"
)

var neverStartedReceiptDocument = []byte("{\"schemaVersion\":1,\"state\":\"never-started\",\"reason\":\"capacity\"}\n")

type deferralFixture struct {
	host system.System
	root string
}

func (run *session) DeferralFixture() (application.ConformanceDeferralFixture, error) {
	root, err := run.host.Files.MkdirTemp("", "hippo-deferral-probe-")
	if err != nil {
		return application.ConformanceDeferralFixture{}, errors.New("deferral retry probe root is unavailable")
	}
	fixture := &deferralFixture{host: run.host, root: root}
	if err = run.host.Files.WriteFile(filepath.Join(root, capacityHoldName), []byte("held\n"), 0o600); err != nil {
		return application.ConformanceDeferralFixture{}, errors.Join(errors.New("deferral retry probe root could not be saturated"), fixture.Close())
	}
	if err = run.host.Files.WriteFile(filepath.Join(root, neverStartedReceiptName), neverStartedReceiptDocument, 0o600); err != nil {
		return application.ConformanceDeferralFixture{}, errors.Join(errors.New("deferral retry probe receipt is unavailable"), fixture.Close())
	}
	return application.ConformanceDeferralFixture{Root: fixture.Root, Release: fixture.Release, Close: fixture.Close}, nil
}
func (fixture *deferralFixture) Root() string { return fixture.root }
func (fixture *deferralFixture) Release() error {
	if err := fixture.host.Files.Remove(filepath.Join(fixture.root, capacityHoldName)); err != nil {
		return errors.New("deferral retry probe capacity release failed")
	}
	return nil
}

func (fixture *deferralFixture) Close() error {
	if err := fixture.host.Files.RemoveAll(fixture.root); err != nil {
		return errors.New("deferral retry probe cleanup failed")
	}
	return nil
}

// ConformanceFailure provides portable failure facts for the application's clean-skip decision.
func (failure *commandError) ConformanceFailure() (string, int, bool) {
	return failure.category, failure.exitCode, failure.cause != nil
}

func (run *session) capabilities() application.ConformanceSession {
	return application.ConformanceSession{
		Manifest: run.Manifest, Verify: run.Verify, VerifyBinary: run.VerifyBinary,
		Snapshot: run.Snapshot, Execute: run.Execute, Receipts: run.Receipts,
		NeverStartedReceipt: run.NeverStartedReceipt, DeferralFixture: run.DeferralFixture, Close: run.Close,
	}
}
