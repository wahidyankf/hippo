package runtime //nolint:testpackage // The launcher's usage capture is a private lifetime seam, exercised inside the production package.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
)

// usageBusyRound keeps one CPU-bound helper running until the helper has itself
// used one CPU-second, then reaps it, so the helper is a waited descendant of
// the guard's child. A wall-clock bound would not do: a helper that spins for
// one second of wall time gets only the share of a core the host leaves it, and
// a contended host delivers well under a CPU-second. ps reports the helper's
// own CPU time, whole seconds on Linux and hundredths on macOS, and the payload
// polls it until it reads at least one second. The poll is capped, so a host
// without ps fails the assertion below rather than hanging.
const usageBusyRound = `/usr/bin/yes >/dev/null & p=$!; n=0; while [ $n -lt 1200 ]; do ` +
	`set -- $(/bin/ps -o time= -p $p); case $1 in 0:00.*|00:00:00|"") ;; *) break;; esac; ` +
	`n=$((n+1)); /bin/sleep 0.05; done; kill $p; wait $p 2>/dev/null`

// usageFloor is the lower bound the helper must clear. The helper ends only
// after one full CPU-second, so the margin below that covers how a kernel
// rounds the usage it reports, and nothing else.
const usageFloor = 800 * time.Millisecond

// startUsageEngine returns an engine whose launches use the real launcher, with
// one owned session, and the run configuration for a shell payload.
func startUsageEngine(t *testing.T, script string) (*Engine, *application.Lease, application.RunConfig) {
	t.Helper()
	engine := NewEngine()
	engine.hooks.startLifetime = startSupervisedLifetime
	engine.leases["usage"] = &Session{Token: "usage"}
	config := application.RunConfig{
		Command: "/bin/sh", Arguments: []string{"-c", script}, WorkingDirectory: t.TempDir(),
		Environment: os.Environ(), Now: time.Now, Sleep: func(time.Duration) {},
	}

	return engine, &application.Lease{Token: "usage"}, config
}

// TestLifetimeRecordsWaitedChildUsageBeforeExit reads the launcher's exit only
// after the wait goroutine has recorded its usage. Run under -race, it proves
// the capture is ordered before the exit is observed.
func TestLifetimeRecordsWaitedChildUsageBeforeExit(t *testing.T) {
	config := RunConfig{WorkingDirectory: t.TempDir(), Arguments: []string{"-c", usageBusyRound + "; exit 0"}}
	lifetime, err := startSupervisedLifetime(context.Background(), config, "/bin/sh", os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if exitError := <-lifetime.exited; exitError != nil {
		t.Fatalf("payload exited with %v", exitError)
	}
	cpu, measured := lifetime.usage.snapshot()
	if !measured || cpu < usageFloor {
		t.Fatalf("launcher usage = %v measured=%v, want at least %v once reaped", cpu, measured, usageFloor)
	}
}

// TestEngineCompletionCarriesWaitedChildUsage checks the completion the engine
// delivers for a passed, a failed, and a waited-grandchild payload.
func TestEngineCompletionCarriesWaitedChildUsage(t *testing.T) {
	for _, test := range []struct {
		name, script string
		exitCode     int
	}{
		{name: "passed", script: usageBusyRound + "; exit 0"},
		{name: "failed", script: usageBusyRound + "; exit 3", exitCode: 3},
		{name: "grandchild", script: "/bin/sh -c '" + usageBusyRound + "'; exit 0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine, lease, config := startUsageEngine(t, test.script)
			workload, err := engine.Start(context.Background(), config, lease, nil)
			if err != nil {
				t.Fatal(err)
			}
			completion := <-workload.Done
			if completion.ExitCode != test.exitCode || !completion.CPUMeasured || completion.CPU < usageFloor {
				t.Fatalf("completion = %+v, want exit %d with at least %v measured CPU", completion, test.exitCode, usageFloor)
			}
		})
	}
}

// TestEngineStopReportsKilledGroupAsLowerBound stops a group whose helper was
// never waited for. The launcher is reaped, so the usage is measured. The
// helper's CPU may or may not be included, so the test checks only the bound
// that always holds: a single busy helper cannot use more CPU than the wall
// time the group existed before the stop.
func TestEngineStopReportsKilledGroupAsLowerBound(t *testing.T) {
	engine, lease, config := startUsageEngine(t, "/usr/bin/yes >/dev/null & /bin/sleep 30")
	started := time.Now()
	workload, err := engine.Start(context.Background(), config, lease, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	completion, stopError := engine.Stop(config, workload)
	elapsed := time.Since(started)
	if stopError != nil {
		t.Fatalf("stop failed: %v", stopError)
	}
	if !completion.CPUMeasured || completion.CPU > elapsed+usageFloor {
		t.Fatalf("killed-group completion = %+v, want measured usage within %v of wall time", completion, elapsed)
	}
}

// TestEngineCompletionWithoutLauncherUsageIsUnmeasured covers a launcher whose
// usage was never recorded: the completion must say so rather than report zero.
func TestEngineCompletionWithoutLauncherUsageIsUnmeasured(t *testing.T) {
	engine, lease, config := startUsageEngine(t, "exit 0")
	engine.hooks.startLifetime = func(context.Context, RunConfig, string, []string, ...*os.File) (*supervisedLifetime, error) {
		lifetime := &supervisedLifetime{exited: make(chan error, 1)}
		lifetime.exited <- nil

		return lifetime, nil
	}
	workload, err := engine.Start(context.Background(), config, lease, nil)
	if err != nil {
		t.Fatal(err)
	}
	if completion := <-workload.Done; completion.CPUMeasured || completion.CPU != 0 {
		t.Fatalf("unmeasured completion = %+v, want no usage", completion)
	}
}
