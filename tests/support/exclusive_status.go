package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

func (driver *Driver) liveExclusiveStatusOwner() error {
	if err := driver.preparePendingV04(); err != nil {
		return err
	}
	session, err := guard.AcquireSession(
		context.Background(), driver.evidenceRoot, "", policy.TaskEphemeral, time.Second,
	)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("exclusive compatibility owner was deferred")
	}
	driver.exclusiveStatusSession = session
	driver.exclusiveStatusState = map[string][]byte{}
	for _, path := range []string{
		filepath.Join(driver.evidenceRoot, "coordination-mode.json"),
		filepath.Join(driver.evidenceRoot, "heavy.lock", "owner.json"),
		session.RecordPath,
	} {
		data, readError := os.ReadFile(path)
		if readError != nil {
			return readError
		}
		driver.exclusiveStatusState[path] = data
	}

	return nil
}

func (driver *Driver) inspectExclusiveStatusOwner() error {
	arguments := []string{statusCommandName, jsonFlag, diskPathFlag, "."}
	if driver.mode == e2eMode {
		commandEnvironment := environmentWith(map[string]string{hippoRootEnvironment: driver.evidenceRoot})
		command := exec.Command(driver.binary, arguments...)
		command.Env = commandEnvironment
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		command.Stdout, command.Stderr = stdout, stderr
		err := command.Run()
		driver.exitCode, driver.output, driver.errorOutput = 0, stdout.String(), stderr.String()
		if err != nil {
			driver.exitCode = 1
			if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
				driver.exitCode = exitError.ExitCode()
			}
		}

		return nil
	}

	base := time.Unix(0, 0)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, err := (cli.Application{
		Stdout: stdout, Stderr: stderr,
		Collector: &sequenceCollector{samples: []policy.Sample{healthySample(base), healthySample(base.Add(time.Second))}},
		Sleep:     func(time.Duration) {}, Environment: []string{hippoRootEnvironment + "=" + driver.evidenceRoot},
	}).Run(context.Background(), arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return err
}

func (driver *Driver) requireExclusiveStatusOwner() error {
	var payload struct {
		Coordination guard.ReservationTotals `json:"coordination"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return err
	}
	totals := payload.Coordination
	if driver.exitCode != 0 || totals.Mode != exclusiveMode || totals.ActiveOwners != 1 ||
		totals.Ephemeral != 1 || totals.LegacyEntries != 1 || len(totals.Owners) != 1 ||
		!totals.Owners[0].Legacy || totals.Owners[0].Class != policy.TaskEphemeral {
		return fmt.Errorf("exclusive status did not expose its live legacy owner: exit=%d totals=%+v", driver.exitCode, totals)
	}
	for path, before := range driver.exclusiveStatusState {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			return fmt.Errorf("exclusive status changed compatibility state: %w", err)
		}
	}

	return nil
}

func (driver *Driver) invalidExclusiveStatusOwner(state string) error {
	if err := driver.liveExclusiveStatusOwner(); err != nil {
		return err
	}
	contents := map[string][]byte{
		"malformed": []byte("{\"schemaVersion\":1\n"),
		"future":    []byte("{\"schemaVersion\":2}\n"),
	}[state]
	if contents == nil {
		return fmt.Errorf("unknown invalid compatibility state %q", state)
	}
	if err := os.WriteFile(driver.exclusiveStatusSession.RecordPath, contents, 0o600); err != nil {
		return err
	}
	driver.exclusiveStatusState[driver.exclusiveStatusSession.RecordPath] = contents

	return nil
}

func (driver *Driver) inspectInvalidExclusiveStatusOwner() error {
	driver.v04Error = driver.inspectExclusiveStatusOwner()

	return nil
}

func (driver *Driver) requireInvalidExclusiveStatusOwner(code string) error {
	expected, err := strconv.Atoi(code)
	if err != nil {
		return err
	}
	if driver.exitCode != expected {
		if driver.v04Error != nil {
			return fmt.Errorf("invalid exclusive status exit=%d want=%d: %w", driver.exitCode, expected, driver.v04Error)
		}

		return fmt.Errorf("invalid exclusive status exit=%d want=%d", driver.exitCode, expected)
	}
	for path, before := range driver.exclusiveStatusState {
		after, readError := os.ReadFile(path)
		if readError != nil || !bytes.Equal(before, after) {
			return fmt.Errorf("invalid exclusive status changed compatibility state: %w", readError)
		}
	}

	return nil
}
