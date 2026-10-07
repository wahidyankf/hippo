package support

import (
	"context"

	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
)

// RunGuard exercises the production application with its actual runtime adapters.
func RunGuard(ctx context.Context, config runtimeadapter.RunConfig) (int, error) {
	engine := runtimeadapter.NewEngine()
	return (application.RunServices{Coordination: engine, Workload: engine, Ports: engine, Evidence: engine, Clock: runtimeadapter.RunClock{}}).Run(ctx, runtimeadapter.Input(config))
}
