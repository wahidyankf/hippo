package support

import (
	"context"
	"io"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/adapters/conformance"
	"github.com/wahidyankf/hippo/internal/application"
)

// RunConformance binds the production application to its real environment for corpus tests.
func RunConformance(ctx context.Context, path string, output io.Writer) error {
	return (application.ConformanceService{Environment: conformance.Environment{}}).Run(ctx, path, output)
}

// MainConformance exercises CLI presentation with the production application and real ports.
func MainConformance(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	return cli.ConformanceMain(ctx, arguments, stdout, stderr, application.ConformanceService{Environment: conformance.Environment{}})
}
