package conformance //nolint:testpackage // Private lifecycle fixtures bind the real application to its environment.

import (
	"context"
	"io"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/application"
)

func Main(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	return cli.ConformanceMain(ctx, arguments, stdout, stderr, application.ConformanceService{Environment: Environment{Host: system.Production}, Now: time.Now})
}

func cleanCapacitySkip(err error, output []byte, verified bool) bool {
	return application.CleanConformanceCapacitySkip(err, output, verified)
}
