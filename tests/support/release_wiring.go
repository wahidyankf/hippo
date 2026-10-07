package support

import (
	"context"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/internal/adapters/health"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/policy"
)

// ReleaseServices wires the real release orchestration to its production adapters.
func ReleaseServices() application.ReleaseServices {
	return application.ReleaseServices{Clock: runtimeadapter.ReleaseClock{}, Health: health.Probe{}, Evidence: evidence.ReleaseRepository{}}
}

// RunReleaseMonitor executes the real application release-monitor use case.
func RunReleaseMonitor(ctx context.Context, config application.MonitorConfig) error {
	return ReleaseServices().RunMonitor(ctx, config)
}

// AssessReleaseFile executes the real application release-assessment use case.
func AssessReleaseFile(path string) (policy.ReleaseSummary, error) {
	return ReleaseServices().AssessFile(path)
}
