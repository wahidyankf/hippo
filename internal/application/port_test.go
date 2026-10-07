package application_test

import (
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
)

type portFixture struct {
	application.PortRuntime

	attempts int
}

func (fixture *portFixture) BeginPort(application.RunConfig) (application.PortAdmission, error) {
	return application.PortAdmission{Attempt: func() (application.PortResult, error) {
		fixture.attempts++
		if fixture.attempts == 1 {
			return application.PortResult{Retry: true}, nil
		}
		return application.PortResult{Lease: &application.PortLease{ID: "owned"}}, nil
	}}, nil
}

func TestPortApplicationOwnsStaleRetry(t *testing.T) {
	fixture := &portFixture{}
	services := application.RunServices{Ports: fixture}
	lease, err := services.AcquirePort(application.RunConfig{LeasePort: 3000})
	if err != nil || lease == nil || lease.ID != "owned" || fixture.attempts != 2 {
		t.Fatalf("lease=%v err=%v attempts=%d", lease, err, fixture.attempts)
	}
}
