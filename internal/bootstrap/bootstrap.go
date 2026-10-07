// Package bootstrap composes concrete adapters for the command entry points.
package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	resourceconfig "github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/internal/adapters/conformance"
	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/internal/adapters/health"
	"github.com/wahidyankf/hippo/internal/adapters/host"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	app "github.com/wahidyankf/hippo/internal/application"
)

// testBuildVersion is the identity only the repository's own test builds
// stamp through ldflags. A release build carries its tag and a plain build
// carries "dev", so neither can take it by accident.
const testBuildVersion = "v0.0.0-test"

// linuxEvidenceRootEnvironment names a directory whose proc and sys trees a
// test build reads in place of the live host's, so a compiled end-to-end run
// samples fixed Linux evidence instead of the runner's load. Only a build
// carrying testBuildVersion honours it, and only for an absolute path.
const linuxEvidenceRootEnvironment = "HIPPO_TEST_LINUX_EVIDENCE_ROOT"

// defaultCollector samples the live host, except in a test build given an
// absolute Linux evidence root.
func defaultCollector(version string, environment []string) host.SystemCollector {
	root := environmentMap(environment)[linuxEvidenceRootEnvironment]
	if version != testBuildVersion || !filepath.IsAbs(root) {
		return host.SystemCollector{}
	}

	return host.SystemCollector{ReadFile: host.RootedFileReader(root)}
}

func environmentMap(environment []string) map[string]string {
	result := map[string]string{}

	for _, entry := range environment {
		for index := range entry {
			if entry[index] == '=' {
				result[entry[:index]] = entry[index+1:]
				break
			}
		}
	}

	return result
}

// WithDefaults composes production adapters for one CLI invocation.
func WithDefaults(application cli.Application) cli.Application {
	application.Compose = WithDefaults
	if application.Stdin == nil {
		application.Stdin = os.Stdin
	}
	if application.Stdout == nil {
		application.Stdout = os.Stdout
	}
	if application.Stderr == nil {
		application.Stderr = os.Stderr
	}

	if application.Environment == nil {
		application.Environment = os.Environ()
	}

	if application.Now == nil {
		application.Now = time.Now
	}

	if application.Version == "" {
		application.Version = cli.Version
	}
	if application.Commit == "" {
		application.Commit = cli.Commit
	}
	if application.Collector == nil {
		application.Collector = defaultCollector(application.Version, application.Environment)
	}

	application.Configuration = resourceconfig.Provider{}
	engine := runtimeadapter.NewEngine()
	application.RunServices = app.RunServices{Coordination: engine, Workload: engine, Ports: engine, Evidence: engine, Clock: runtimeadapter.RunClock{}}
	application.ReleaseServices = app.ReleaseServices{Configuration: application.Configuration, Collector: application.Collector, Environment: environmentMap(application.Environment), Clock: runtimeadapter.ReleaseClock{NowFunc: application.Now, Pause: application.Sleep}, Health: health.Probe{}, Evidence: evidence.ReleaseRepository{}}
	application.Observer = app.ObservationServices{Configuration: application.Configuration, Collector: application.Collector, Runtime: engine, Evidence: evidence.HistoryRepository{}, Clock: runtimeadapter.ObservationClock{NowFunc: application.Now, Pause: application.Sleep}, Environment: environmentMap(application.Environment)}
	application.RunEntry = app.RunEntryServices{Configuration: application.Configuration, Observation: application.Observer, Run: application.RunServices, Collector: application.Collector, Environment: application.Environment, Now: application.Now, Sleep: application.Sleep, Stdin: application.Stdin, Stdout: application.Stdout, Stderr: application.Stderr, PortLeaseRoot: resourceconfig.PortLeaseRoot(environmentMap(application.Environment))}
	return application
}

// Execute runs the composed production command boundary.
func Execute(ctx context.Context, args []string) int {
	code, _ := WithDefaults(cli.Application{}).Run(ctx, args)
	return code
}

// ConformanceService composes the real consumer-conformance capabilities.
func ConformanceService() app.ConformanceService {
	return app.ConformanceService{Environment: conformance.Environment{}}
}
