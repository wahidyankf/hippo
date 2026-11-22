package application_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/identity"
)

// The advisory paths of the display lifecycle are reached only when an annotation operation fails, so each test
// drives one failing double and reports the advisory it leaves behind.

func TestRegisterDisplayRecordsPublicationFailureAsAdvisory(t *testing.T) {
	var advisories application.DisplayAdvisories
	effects := application.DisplayRegistrationEffects{
		Publish: func(string, identity.DisplayLabel) error { return errors.New("publish refused") },
		Commit:  func() error { return nil },
		Remove:  func(string) error { t.Fatal("removal called after a successful commit"); return nil },
	}
	request := application.DisplayRegistration{
		RunID: "run-1", Kind: application.DisplayReservationRegistration, Ownership: application.DisplayOwnershipOwned,
	}
	if err := application.RegisterDisplay(request, effects, &advisories); err != nil {
		t.Fatalf("advisory publication failure returned %v", err)
	}
	var stderr bytes.Buffer
	advisories.Report(&stderr)
	if stderr.String() != publicationAdvisory {
		t.Fatalf("publication advisory %q", stderr.String())
	}
}

func TestRegisterDisplayCompensatesFailedExclusiveCommitAndRecordsRemoval(t *testing.T) {
	var advisories application.DisplayAdvisories
	commitError := errors.New("commit refused")
	removed := ""
	effects := application.DisplayRegistrationEffects{
		Publish: func(string, identity.DisplayLabel) error { return nil },
		Commit:  func() error { return commitError },
		Remove:  func(runID string) error { removed = runID; return errors.New("remove refused") },
	}
	request := application.DisplayRegistration{
		RunID: "run-2", Kind: application.DisplayExclusiveRegistration, Ownership: application.DisplayOwnershipOwned,
	}
	if err := application.RegisterDisplay(request, effects, &advisories); !errors.Is(err, commitError) {
		t.Fatalf("commit error %v, want %v", err, commitError)
	}
	if removed != "run-2" {
		t.Fatalf("compensation removed %q", removed)
	}
	var stderr bytes.Buffer
	advisories.Report(&stderr)
	if stderr.String() != removalAdvisory {
		t.Fatalf("removal advisory %q", stderr.String())
	}
}

func TestRetireDisplayRecordsRemovalFailureAsAdvisory(t *testing.T) {
	var advisories application.DisplayAdvisories
	effects := application.DisplayRetirementEffects{
		RemoveOwnership: func() (application.DisplayRetirementResult, error) {
			return application.DisplayRetirementResult{Confirmed: true}, nil
		},
		Remove: func(string) error { return errors.New("remove refused") },
	}
	request := application.DisplayRetirement{RunID: "run-3", Ownership: application.DisplayOwnershipOwned}
	if err := application.RetireDisplay(request, effects, &advisories); err != nil {
		t.Fatalf("advisory removal failure returned %v", err)
	}
	var stderr bytes.Buffer
	advisories.Report(&stderr)
	if stderr.String() != removalAdvisory {
		t.Fatalf("removal advisory %q", stderr.String())
	}
}

func TestDiscardUnregisteredDisplayOnlyRemovesOwnedContext(t *testing.T) {
	var advisories application.DisplayAdvisories
	calls := 0
	remove := func(string) error { calls++; return errors.New("remove refused") }
	application.DiscardUnregisteredDisplay(
		application.DisplayRetirement{RunID: "run-4", Ownership: application.DisplayOwnershipForeign}, remove, &advisories)
	if calls != 0 {
		t.Fatalf("foreign context removed %d times", calls)
	}
	application.DiscardUnregisteredDisplay(
		application.DisplayRetirement{RunID: "run-4", Ownership: application.DisplayOwnershipOwned}, remove, &advisories)
	var stderr bytes.Buffer
	advisories.Report(&stderr)
	if calls != 1 || stderr.String() != removalAdvisory {
		t.Fatalf("owned discard calls %d, advisory %q", calls, stderr.String())
	}
}
