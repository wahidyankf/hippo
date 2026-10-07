package application_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
)

type portContractFixture struct {
	application.PortRuntime

	beginError error
	results    []application.PortResult
	errors     []error
	attempts   int
}

func (fixture *portContractFixture) BeginPort(application.RunConfig) (application.PortAdmission, error) {
	return application.PortAdmission{Attempt: func() (application.PortResult, error) {
		index := fixture.attempts
		fixture.attempts++
		return fixture.results[index], fixture.errors[index]
	}}, fixture.beginError
}

func TestPortContractBoundedAcquisition(t *testing.T) {
	lease := &application.PortLease{ID: "owned"}
	for _, test := range []struct {
		name       string
		beginError error
		results    []application.PortResult
		failures   []error
		attempts   int
		want       *application.PortLease
		errorText  string
	}{
		{name: "begin-refused", beginError: errRunContract, errorText: "injected"},
		{name: "first-admitted", results: []application.PortResult{{Lease: lease}}, failures: []error{nil}, attempts: 1, want: lease},
		{name: "attempt-refused", results: []application.PortResult{{}}, failures: []error{errRunContract}, attempts: 1, errorText: "injected"},
		{name: "stale-then-refused", results: []application.PortResult{{Retry: true}, {}}, failures: []error{nil, errRunContract}, attempts: 2, errorText: "injected"},
		{name: "bounded-exhaustion", results: []application.PortResult{{Retry: true}, {Retry: true}}, failures: []error{nil, nil}, attempts: 2, errorText: "port 4321 could not be leased"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := &portContractFixture{beginError: test.beginError, results: test.results, errors: test.failures}
			actual, err := (application.RunServices{Ports: fixture}).AcquirePort(application.RunConfig{LeasePort: 4321})
			if actual != test.want || fixture.attempts != test.attempts {
				t.Fatalf("lease=%v attempts=%d", actual, fixture.attempts)
			}
			if test.errorText == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.errorText) {
				t.Fatalf("error=%v want=%q", err, test.errorText)
			}
			if test.errorText == "injected" && !errors.Is(err, errRunContract) {
				t.Fatal("lost original error")
			}
		})
	}
}
