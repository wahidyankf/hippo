package support

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	guard "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

// The command paths the child-status scenarios run. Each lives in the scenario's
// own root, so no scenario depends on what the host has installed.
const (
	unexecutableCommandName = "not-executable"
	missingCommandName      = "missing-command"
)

func (driver *Driver) executionAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^an empty guarded run root$`, driver.emptyGuardedRunRoot),
		step(`^a guarded command that exits zero runs in that root$`, func() error {
			return driver.runGuardedCommandIn(driver.leaseRoot, shellPath, "-c", "exit 0")
		}),
		step(`^the guard writes nothing to standard error$`, driver.requireSilentStandardError),
		step(`^a guarded run root holding a command file that cannot be executed$`, driver.stageUnexecutableCommand),
		step(`^a guarded run executes that command file$`, func() error {
			return driver.runGuardedCommandIn(driver.leaseRoot, filepath.Join(driver.leaseRoot, unexecutableCommandName))
		}),
		step(`^a guarded run executes a command path that does not exist$`, func() error {
			return driver.runGuardedCommandIn(driver.leaseRoot, filepath.Join(driver.leaseRoot, missingCommandName))
		}),
		step(`^it exits 126 naming hippo\.child\.not-executable$`, func() error {
			return driver.requireGuardRefusal(126, "hippo.child.not-executable")
		}),
		step(`^it exits 127 naming hippo\.child\.not-found$`, func() error {
			return driver.requireGuardRefusal(127, "hippo.child.not-found")
		}),
		step(`^a guarded child ends itself with signal (TERM|KILL)$`, func(signal string) error {
			return driver.runGuardedCommandIn(driver.leaseRoot, shellPath, "-c", "kill -"+signal+" $$")
		}),
		step(`^the guard exits with status (\d+)$`, driver.requireGuardStatus),
		step(`^a service port leased by a guarded run whose child exits zero$`, func() error {
			return driver.runLeasedPayload("first")
		}),
		step(`^another guarded run leases that same service port$`, func() error {
			return driver.runLeasedPayload("second")
		}),
		step(`^the second run exits zero and its child starts$`, func() error {
			return driver.requireLeasedPayloadStarted("second")
		}),
		step(`^a guarded child records its HIPPO_RESERVED_MEMORY_BYTES variable$`, func() error {
			return driver.recordGuardedChildVariable("HIPPO_RESERVED_MEMORY_BYTES")
		}),
		step(`^the child reads a positive byte count$`, driver.requirePositiveByteCount),
		step(`^a guarded child records its HIPPO_BIN variable$`, func() error {
			return driver.recordGuardedChildVariable("HIPPO_BIN")
		}),
		step(`^HIPPO_BIN names the running executable by absolute path$`, driver.requireBinNamesRunningExecutable),
		step(`^a guarded run names no resource tier and a command that exits zero$`, driver.runWithoutResourceTier),
		step(`^it exits 2 naming hippo\.args\.invalid and its payload does not start$`, func() error {
			if err := driver.requireGuardRefusal(2, "hippo.args.invalid"); err != nil {
				return err
			}
			if _, err := os.Stat(filepath.Join(driver.leaseRoot, refusedPayloadMarker)); err == nil {
				return errors.New("the refused run's payload ran")
			}

			return nil
		}),
		step(`^an owner waits for admission for sixty-five seconds$`, func() error {
			return driver.runWaitingForAdmission(65 * time.Second)
		}),
		step(`^the queue reports its run ID, position, and remaining deadline at 0, 30, and 60 seconds$`, driver.requireHeartbeatsEveryThirtySeconds),
		step(`^a guarded child runs with a stale HIPPO_SESSION value ahead of that session's token$`, driver.runWithStaleSessionAhead),
		step(`^TMPDIR names an empty scenario directory$`, driver.emptyLeaseTemporaryDirectory),
		step(`^a guarded run leases a service port and its child exits zero$`, func() error {
			return driver.runLeasedPayload("first")
		}),
		step(`^the port lease root is the hippo-port-leases directory inside TMPDIR$`, driver.requirePortLeaseRootInTemporaryDirectory),
	}
}

// leasedServicePort is the port the lease scenarios hold. Each run names it as its
// own minimum and maximum, so the lease is the only thing that can decide the run.
const leasedServicePort = "23001"

// leaseTemporaryName is the TMPDIR a lease scenario hands its runs, inside the
// scenario's own root.
const leaseTemporaryName = "temporary"

// emptyLeaseTemporaryDirectory stages the scenario root and an empty TMPDIR under
// it, so the port lease root a run creates is the scenario's and never the host's.
func (driver *Driver) emptyLeaseTemporaryDirectory() error {
	if err := driver.emptyGuardedRunRoot(); err != nil {
		return err
	}

	return os.MkdirAll(filepath.Join(driver.leaseRoot, leaseTemporaryName), 0o700)
}

// runLeasedPayload runs the public run command with a port lease on the scenario's
// service port. The payload writes its own marker, so each owner's start is visible.
func (driver *Driver) runLeasedPayload(owner string) error {
	if driver.leaseRoot == "" {
		if err := driver.emptyLeaseTemporaryDirectory(); err != nil {
			return err
		}
	}
	marker := filepath.Join(driver.leaseRoot, owner+"-started")
	temporary := filepath.Join(driver.leaseRoot, leaseTemporaryName)

	return driver.runCommandWith(
		map[string]string{hippoRootEnvironment: driver.leaseRoot, "TMPDIR": temporary},
		runCommandName, diskPathFlag, ".", resourceTierFlag, standardTierName,
		leasePortFlagName, leasedServicePort, leaseMinimumFlagName, leasedServicePort,
		leaseMaximumFlagName, leasedServicePort, leaseOwnerFlagName, owner,
		"--", shellPath, "-c", "printf started > '"+marker+"'; exit 0",
	)
}

// requireLeasedPayloadStarted holds the run to a zero status and to its own payload's marker.
func (driver *Driver) requireLeasedPayloadStarted(owner string) error {
	if driver.exitCode != 0 {
		return fmt.Errorf("leased run exit=%d stderr=%q, want exit 0", driver.exitCode, driver.errorOutput)
	}
	if _, err := os.Stat(filepath.Join(driver.leaseRoot, owner+"-started")); err != nil {
		return fmt.Errorf("the %s run's payload did not start: %w", owner, err)
	}

	return nil
}

// queueHeartbeatPattern reads one queue heartbeat the guard writes to standard error.
var queueHeartbeatPattern = regexp.MustCompile(`HIPPO queued run=(\S+) position=(\d+) remaining=(\S+)\.`)

// requireHeartbeatsEveryThirtySeconds holds a wait of sixty-five seconds to three
// heartbeats, one at each thirty-second mark, each naming its run, position, and
// the time that remains.
func (driver *Driver) requireHeartbeatsEveryThirtySeconds() error {
	if driver.exitCode != status.LimitShed {
		return fmt.Errorf("exit=%d stderr=%q, want the deferral exit %d", driver.exitCode, driver.errorOutput, status.LimitShed)
	}
	matches := queueHeartbeatPattern.FindAllStringSubmatch(driver.errorOutput, -1)
	if len(matches) != 3 {
		return fmt.Errorf("queue heartbeats=%d, want three across sixty-five seconds: %s", len(matches), driver.errorOutput)
	}
	var previous time.Duration
	for index, match := range matches {
		remaining, err := time.ParseDuration(match[3])
		if err != nil {
			return err
		}
		if index > 0 && previous-remaining != 30*time.Second {
			return fmt.Errorf("heartbeat %d shows %s remaining after %s, want thirty seconds less", index, remaining, previous)
		}
		previous = remaining
	}
	if runs := driver.childRunCount(); runs != 0 {
		return fmt.Errorf("the queued owner launched its payload %d times, want zero", runs)
	}

	return nil
}

// staleSessionAhead is a HIPPO_SESSION value no live owner holds, listed before the live token.
const staleSessionAhead = "HIPPO_SESSION=stale-token-no-live-owner"

// runWithStaleSessionAhead runs a passing child whose environment names HIPPO_SESSION
// twice: a stale value first, then the live session's token.
func (driver *Driver) runWithStaleSessionAhead() error {
	if driver.heavySession == nil {
		return errors.New("no inherited session was staged")
	}
	err := driver.runGuardedShell("exit 0", []string{staleSessionAhead, "HIPPO_SESSION=" + driver.heavySession.Token})
	driver.inheritedSessions = guard.InheritedSession(driver.leaseRoot, driver.heavySession.Token)

	return err
}

// refusedPayloadMarker is written by a payload that runs, so a refused run must leave it absent.
const refusedPayloadMarker = "payload-ran"

// recordGuardedChildVariable runs an empty payload that copies one variable from its
// environment into a file in the scenario root, so the value the child received can be read.
// An unset variable is recorded as unset.
func (driver *Driver) recordGuardedChildVariable(name string) error {
	if driver.leaseRoot == "" {
		if err := driver.emptyGuardedRunRoot(); err != nil {
			return err
		}
	}
	target := filepath.Join(driver.leaseRoot, "recorded-"+name)
	script := `printf '%s' "${` + name + `-unset}" > '` + target + `'`
	arguments := []string{runCommandName, diskPathFlag, "."}
	if driver.configPath != "" {
		arguments = append(arguments, configFlag, driver.configPath)
	}
	arguments = append(arguments, "--", shellPath, "-c", script)

	return driver.runGuardInRoot(driver.leaseRoot, arguments...)
}

// readRecordedVariable returns the value the child recorded for name.
func (driver *Driver) readRecordedVariable(name string) (string, error) {
	value, err := os.ReadFile(filepath.Join(driver.leaseRoot, "recorded-"+name))
	if err != nil {
		return "", fmt.Errorf("the child recorded no %s: %w", name, err)
	}

	return string(value), nil
}

// requirePositiveByteCount holds the reservation the child received to a positive byte count.
func (driver *Driver) requirePositiveByteCount() error {
	if driver.exitCode != 0 {
		return fmt.Errorf("guard exit=%d stderr=%q, want exit 0", driver.exitCode, driver.errorOutput)
	}
	value, err := driver.readRecordedVariable("HIPPO_RESERVED_MEMORY_BYTES")
	if err != nil {
		return err
	}
	if count, parseError := strconv.ParseInt(value, 10, 64); parseError != nil || count <= 0 {
		return fmt.Errorf("HIPPO_RESERVED_MEMORY_BYTES is %q, want a positive byte count", value)
	}

	return nil
}

// requireBinNamesRunningExecutable holds the child's HIPPO_BIN to an absolute path that
// names the same file as the executable the scenario runs.
func (driver *Driver) requireBinNamesRunningExecutable() error {
	if driver.exitCode != 0 {
		return fmt.Errorf("guard exit=%d stderr=%q, want exit 0", driver.exitCode, driver.errorOutput)
	}
	value, err := driver.readRecordedVariable("HIPPO_BIN")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("HIPPO_BIN %q is not an absolute path", value)
	}
	running := driver.binary
	if driver.mode != contract.E2E {
		if running, err = os.Executable(); err != nil {
			return err
		}
	}
	got, err := os.Stat(value)
	if err != nil {
		return err
	}
	want, err := os.Stat(running)
	if err != nil {
		return err
	}
	if !os.SameFile(got, want) {
		return fmt.Errorf("HIPPO_BIN %q is not the running executable %q", value, running)
	}

	return nil
}

// runWithoutResourceTier runs a passing payload under the scenario's schema-three
// configuration and names no resource tier, so only the refusal can answer.
func (driver *Driver) runWithoutResourceTier() error {
	marker := filepath.Join(driver.leaseRoot, refusedPayloadMarker)

	return driver.runCommandWith(
		map[string]string{hippoRootEnvironment: driver.leaseRoot},
		runCommandName, taskClassFlag, taskClassEphemeral, sourceFlagName, hippoFixtureName,
		diskPathFlag, driver.leaseRoot, configFlag, driver.configPath,
		"--", shellPath, "-c", "printf ran > '"+marker+"'; exit 0",
	)
}

// requirePortLeaseRootInTemporaryDirectory holds the run's port lease root to the
// hippo-port-leases directory inside the TMPDIR the scenario named.
func (driver *Driver) requirePortLeaseRootInTemporaryDirectory() error {
	if driver.exitCode != 0 {
		return fmt.Errorf("leased run exit=%d stderr=%q, want exit 0", driver.exitCode, driver.errorOutput)
	}
	info, err := os.Stat(filepath.Join(driver.leaseRoot, leaseTemporaryName, "hippo-port-leases"))
	if err != nil || !info.IsDir() {
		return fmt.Errorf("no hippo-port-leases directory under TMPDIR: %w", err)
	}

	return nil
}

// runGuardedCommandIn runs the public run command over root with one payload, in
// process at the unit and integration boundaries and through the compiled binary
// at the end-to-end one.
func (driver *Driver) runGuardedCommandIn(root string, command ...string) error {
	arguments := append([]string{runCommandName, diskPathFlag, ".", resourceTierFlag, standardTierName, "--"}, command...)

	return driver.runGuardInRoot(root, arguments...)
}

// stageUnexecutableCommand writes a command file the owner cannot execute: it is
// present and readable, but it has no execute bit.
func (driver *Driver) stageUnexecutableCommand() error {
	if err := driver.emptyGuardedRunRoot(); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(driver.leaseRoot, unexecutableCommandName), []byte("exit 0\n"), 0o600)
}

// requireSilentStandardError holds a healthy run to a zero status and no
// standard error at all, so the guard's own notices cannot pass unnoticed.
func (driver *Driver) requireSilentStandardError() error {
	if driver.exitCode != 0 || driver.errorOutput != "" {
		return fmt.Errorf("exit=%d stderr=%q, want exit 0 and an empty standard error", driver.exitCode, driver.errorOutput)
	}

	return nil
}

// requireGuardRefusal checks the exit status and the bracketed code in the
// diagnostic, which is what a caller sees for a refused child.
func (driver *Driver) requireGuardRefusal(code int, name string) error {
	if driver.exitCode != code || !strings.Contains(driver.errorOutput, "["+name+"]") {
		return fmt.Errorf("exit=%d stderr=%q, want exit %d naming %s", driver.exitCode, driver.errorOutput, code, name)
	}

	return nil
}

// requireGuardStatus holds the guard's exit status to the one the scenario names.
func (driver *Driver) requireGuardStatus(value string) error {
	want, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	if driver.exitCode != want {
		return fmt.Errorf("guard exit=%d stderr=%q, want %d", driver.exitCode, driver.errorOutput, want)
	}

	return nil
}
