package support

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

// unknownProfileName is a profile no built-in or configured profile carries.
const unknownProfileName = "nosuch-profile"

func (driver *Driver) publicCLIAddObserverBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^text status is requested for that shared root$`, driver.textStatusForSharedRoot),
		step(`^text status prints a summary line and one row for the labeled owner without private paths$`, driver.requireTextStatusRows),
		step(`^watch waits between snapshots with (--interval [0-9a-z.]+|no --interval flag)$`, driver.watchWaitsBetweenSnapshots),
		step(`^the watch pauses for ([0-9a-z.]+) before its next snapshot$`, driver.requirePauseForInterval),
		step(`^text development monitoring is requested$`, driver.textMonitorRequested),
		step(`^monitoring prints the initial state and one state line per transition$`, driver.requireTextTransitions),
		step(`^JSON status is requested with a missing config named by (--config|HIPPO_CONFIG|HIPPO_DEFAULT_CONFIG)$`, driver.statusWithMissingConfig),
		step(`^status exits (\d+)$`, driver.requireStatusExit),
		step(`^a profile named by (--profile flag|defaultProfile in the file) that does not exist$`, driver.unknownProfileNamed),
		step(`^JSON status is requested for that profile$`, driver.statusForUnknownProfile),
		step(`^it exits 125 naming (hippo\.policy\.replan-required|hippo\.config\.unreadable)$`, driver.requireUnknownProfileCode),
		step(`^configuration files for (every source|HIPPO_CONFIG and HIPPO_DEFAULT_CONFIG|HIPPO_DEFAULT_CONFIG alone) each declare their own default profile$`, driver.setConfigurationSources),
		step(`^JSON status is requested with those configuration sources$`, driver.statusWithConfigurationSources),
		step(`^status reports the profile of the (--config|HIPPO_CONFIG|HIPPO_DEFAULT_CONFIG) source$`, driver.requireSourceProfile),
	}
}

// configurationSources lists, for each source the scenario can name, the default
// profile its file declares. Each source gets a different profile, so the
// profile status reports names the one source that was read.
var configurationSources = map[string]string{
	configFlag:             "constrained",
	"HIPPO_CONFIG":         "minimal",
	"HIPPO_DEFAULT_CONFIG": "balanced",
}

// configurationSourceSets names which sources each scenario row sets.
var configurationSourceSets = map[string][]string{
	"every source":                          {configFlag, "HIPPO_CONFIG", "HIPPO_DEFAULT_CONFIG"},
	"HIPPO_CONFIG and HIPPO_DEFAULT_CONFIG": {"HIPPO_CONFIG", "HIPPO_DEFAULT_CONFIG"},
	"HIPPO_DEFAULT_CONFIG alone":            {"HIPPO_DEFAULT_CONFIG"},
}

// setConfigurationSources writes one configuration file per set source, each
// declaring that source's profile, and records where each one is.
func (driver *Driver) setConfigurationSources(set string) error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root
	driver.addCLI.configurationPaths = map[string]string{}
	for _, source := range configurationSourceSets[set] {
		path := filepath.Join(root, source+".json")
		document := fmt.Sprintf(`{"schemaVersion":2,"defaultProfile":%q}`, configurationSources[source])
		if err = os.WriteFile(path, []byte(document), 0o600); err != nil {
			return err
		}
		driver.addCLI.configurationPaths[source] = path
	}

	return nil
}

// statusWithConfigurationSources asks for status with every configuration source
// the scenario set, passing --config as a flag and the others as variables.
func (driver *Driver) statusWithConfigurationSources() error {
	arguments := []string{statusCommandName, jsonFlag, diskPathFlag, "."}
	values := map[string]string{hippoRootEnvironment: driver.leaseRoot}
	for source, path := range driver.addCLI.configurationPaths {
		if source == configFlag {
			arguments = append(arguments, configFlag, path)

			continue
		}
		values[source] = path
	}

	return driver.runStatusWith(values, arguments...)
}

// requireSourceProfile holds the profile status reports to the one source the
// scenario expects to be read.
func (driver *Driver) requireSourceProfile(source string) error {
	if driver.exitCode != 0 {
		return fmt.Errorf("status exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}
	var payload struct {
		Profile struct {
			ResolvedProfile string `json:"resolvedProfile"`
		} `json:"profile"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return err
	}
	if want := configurationSources[source]; payload.Profile.ResolvedProfile != want {
		return fmt.Errorf("status resolved profile %q, want %q from %s", payload.Profile.ResolvedProfile, want, source)
	}

	return nil
}

// runStatusWith runs status with the given variables: in process at the unit and
// integration boundaries, through the compiled binary at the end-to-end one.
func (driver *Driver) runStatusWith(values map[string]string, arguments ...string) error {
	if driver.mode == contract.E2E {
		return driver.runBinaryWith(values, arguments...)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := bootstrap.WithDefaults(cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: environmentPairs(values),
		Collector: &sequenceCollector{samples: []policy.Sample{healthySample(time.Now()), healthySample(time.Now())}},
		Sleep:     func(time.Duration) {},
	}).Run(context.Background(), arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

// textStatusForSharedRoot runs the text form of status against the root the
// labeled-owner scenario staged.
func (driver *Driver) textStatusForSharedRoot() error {
	return driver.runStatusWith(map[string]string{hippoRootEnvironment: driver.leaseRoot}, statusCommandName, diskPathFlag, ".")
}

// requireTextStatusRows holds the text status to its published shape: one
// summary line, then one row for the labeled owner, and no path of the root.
func (driver *Driver) requireTextStatusRows() error {
	if driver.exitCode != 0 {
		return fmt.Errorf("text status exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}
	lines := strings.Split(strings.TrimSuffix(driver.output, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "state=") || !strings.Contains(lines[0], "owners=1 waiters=0") {
		return fmt.Errorf("want a summary line and one owner row, got %q", driver.output)
	}
	if !strings.Contains(lines[1], " run=") || !strings.Contains(lines[1], " source=hippo ") {
		return fmt.Errorf("owner row does not name the labeled source: %q", lines[1])
	}
	if strings.Contains(driver.output, driver.leaseRoot) {
		return fmt.Errorf("text status exposed the shared root path: %q", driver.output)
	}

	return nil
}

// maxRecordedPauses bounds the watch probe: a watch that never pauses for the
// wanted interval ends here rather than running forever.
const maxRecordedPauses = 50

// watchWaitsBetweenSnapshots runs watch in process with its pause injected. The
// pause records each wait and ends the run at the first wait of the requested
// length. Observing a snapshot also pauses for a second between its two samples,
// so the probe waits for the interval itself, not the first pause it sees.
func (driver *Driver) watchWaitsBetweenSnapshots(interval string) error {
	arguments := []string{watchCommandName}
	want := 5 * time.Second
	if value, set := strings.CutPrefix(interval, intervalFlagName+" "); set {
		arguments = append(arguments, intervalFlagName, value)
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		want = parsed
	}
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	waits := []time.Duration{}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := bootstrap.WithDefaults(cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{hippoRootEnvironment + "=" + driver.leaseRoot},
		Collector: &sequenceCollector{samples: driver.samples},
		Sleep: func(wait time.Duration) {
			waits = append(waits, wait)
			if wait == want || len(waits) >= maxRecordedPauses {
				interrupt(status.Interruption{Signal: int(syscall.SIGINT)})
			}
		},
	}).Run(ctx, arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()
	driver.addCLI.waits = waits

	return nil
}

// requirePauseForInterval holds the watch to pausing for exactly the wanted
// interval before its next snapshot, which the probe reached and then stopped at.
func (driver *Driver) requirePauseForInterval(want string) error {
	floor, err := time.ParseDuration(want)
	if err != nil {
		return err
	}
	waits := driver.addCLI.waits
	if len(waits) == 0 || waits[len(waits)-1] != floor || len(waits) >= maxRecordedPauses {
		return fmt.Errorf("watch pauses %v, want the interval %s as its last pause", waits, floor)
	}

	return nil
}

func (driver *Driver) textMonitorRequested() error {
	ctx, cancel := context.WithCancel(context.Background())
	collector := &sequenceCollector{samples: driver.samples, cancel: cancel, cancelAfter: len(driver.samples)}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, err := bootstrap.WithDefaults(cli.Application{
		Stdout: stdout, Stderr: stderr, Collector: collector,
	}).Run(ctx, []string{monitorCommandName, intervalFlagName, "1ms", diskPathFlag, "."})
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return err
}

// stateToken matches the state word of one text transition line.
var stateToken = regexp.MustCompile(` state=(\S+)`)

// requireTextTransitions holds the text monitor to the scenario's two sampled
// states: the initial state, then one line for the transition to the other.
func (driver *Driver) requireTextTransitions() error {
	lines := strings.Split(strings.TrimSpace(driver.output), "\n")
	if driver.exitCode != 0 || len(lines) != 2 {
		return fmt.Errorf("exit=%d lines=%d stdout=%q stderr=%q", driver.exitCode, len(lines), driver.output, driver.errorOutput)
	}
	first, second := stateToken.FindStringSubmatch(lines[0]), stateToken.FindStringSubmatch(lines[1])
	if first == nil || second == nil || first[1] == second[1] {
		return fmt.Errorf("transition lines do not name two states: %q", driver.output)
	}

	return nil
}

// statusWithMissingConfig asks for status with a configuration file that does
// not exist, named by the variable or flag the scenario gives.
func (driver *Driver) statusWithMissingConfig(source string) error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root
	missing := filepath.Join(root, "absent", "hippo.local.json")
	arguments := []string{statusCommandName, jsonFlag, diskPathFlag, "."}
	values := map[string]string{hippoRootEnvironment: root}
	switch source {
	case configFlag:
		arguments = append(arguments, configFlag, missing)
	case "HIPPO_CONFIG":
		values["HIPPO_CONFIG"] = missing
	case "HIPPO_DEFAULT_CONFIG":
		values["HIPPO_DEFAULT_CONFIG"] = missing
	default:
		return fmt.Errorf("unknown configuration source %q", source)
	}

	return driver.runStatusWith(values, arguments...)
}

func (driver *Driver) requireStatusExit(want string) error {
	code, err := strconv.Atoi(want)
	if err != nil {
		return err
	}
	if driver.exitCode != code {
		return fmt.Errorf("status exit=%d stderr=%q, want %d", driver.exitCode, driver.errorOutput, code)
	}

	return nil
}

// unknownProfileNamed stages the unknown profile name: as the --profile flag,
// or as the default profile of a configuration file the scenario writes.
func (driver *Driver) unknownProfileNamed(source string) error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root
	driver.addCLI.unknownProfileSource = source
	if source == "defaultProfile in the file" {
		document := fmt.Sprintf(`{"schemaVersion":2,"defaultProfile":%q}`, unknownProfileName)

		return os.WriteFile(filepath.Join(root, "hippo.local.json"), []byte(document), 0o600)
	}

	return nil
}

func (driver *Driver) statusForUnknownProfile() error {
	arguments := []string{statusCommandName, jsonFlag, diskPathFlag, "."}
	if driver.addCLI.unknownProfileSource == "--profile flag" {
		arguments = append(arguments, "--profile", unknownProfileName)
	} else {
		arguments = append(arguments, configFlag, filepath.Join(driver.leaseRoot, "hippo.local.json"))
	}

	return driver.runStatusWith(map[string]string{hippoRootEnvironment: driver.leaseRoot}, arguments...)
}

func (driver *Driver) requireUnknownProfileCode(code string) error {
	if driver.exitCode != 125 || !strings.Contains(driver.errorOutput, "hippo: ["+code+"]") {
		return fmt.Errorf("exit=%d stderr=%q, want 125 naming %s", driver.exitCode, driver.errorOutput, code)
	}
	if strings.Contains(driver.errorOutput, "hippo.config.unresolvable") {
		return fmt.Errorf("the reserved unresolvable code was returned: %q", driver.errorOutput)
	}

	return nil
}
