package support

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	resourceconfig "github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// The optional very-heavy tier is declared by a schema-3 file. Every bound in
// these fixtures fits a two-CPU runner: the tier's maximum is clamped to the
// safe capacity the host offers, and its minimum is one CPU and 256 MiB.
const (
	veryHeavyTierName     = "very-heavy"
	lightTierName         = "light"
	veryHeavyDeclaration  = `"very-heavy":{"minimumCpu":1,"maximumCpu":2,"minimumMemoryMiB":256,"maximumMemoryMiB":512,"queueDeadline":"4h"}`
	tierPayloadMarker     = "tier-payload-ran"
	tierOutputFlag        = "--output"
	tierJSONValue         = "json"
	tierReserveCPUFlag    = "--reserve-cpu"
	tierConfigFileName    = "hippo.json"
	tierEnvironmentConfig = "HIPPO_CONFIG"
	tierDeadlineHours     = 4
)

// tierDeclarations maps each phrase a scenario uses to the declaration it stands for.
var tierDeclarations = map[string]string{
	"a maximum CPU above the pool": `"very-heavy":{"minimumCpu":1,"maximumCpu":3,"minimumMemoryMiB":256,"maximumMemoryMiB":512,"queueDeadline":"4h"}`,
	"null":                         `"very-heavy":null`,
	"an empty object":              `"very-heavy":{}`,
	"a queue deadline that is not a duration": `"very-heavy":{"minimumCpu":1,"maximumCpu":2,"minimumMemoryMiB":256,"maximumMemoryMiB":512,"queueDeadline":"later"}`,
	"a key spelled with an underscore":        `"very_heavy":{"minimumCpu":1,"maximumCpu":2,"minimumMemoryMiB":256,"maximumMemoryMiB":512,"queueDeadline":"4h"}`,
}

// tierDocument is a schema-3 file whose pool and required tiers fit a two-CPU
// runner, followed by the optional declaration when there is one.
func tierDocument(optional string) string {
	document := `{"schemaVersion":3,"coordination":{"maxCpu":2,"maxMemoryMiB":1024,"baseActiveOwners":2,"maxActiveOwners":3,` +
		`"promotion":{"completedRuns":1,"minimumSources":1,"minimumAvailableMemoryMiB":256,"maximumCpuP95Percent":98},` +
		`"emergencyAvailableMemoryMiB":256,"tiers":{` +
		`"light":{"minimumCpu":1,"maximumCpu":1,"minimumMemoryMiB":256,"maximumMemoryMiB":256,"queueDeadline":"30m"},` +
		`"standard":{"minimumCpu":1,"maximumCpu":1,"minimumMemoryMiB":256,"maximumMemoryMiB":256,"queueDeadline":"90m"},` +
		`"heavy":{"minimumCpu":1,"maximumCpu":2,"minimumMemoryMiB":256,"maximumMemoryMiB":512,"queueDeadline":"4h"}`
	if optional != "" {
		document += "," + optional
	}

	return document + "}}}"
}

// tierHistoryRow is the part of one history row the tier scenarios read.
type tierHistoryRow struct {
	ResourceTier string `json:"resourceTier"`
	Outcome      string `json:"outcome"`
}

// tierFailureBody is the machine-readable failure a refused run writes to stderr.
type tierFailureBody struct {
	ExitCode int `json:"exitCode"`
	Error    struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	} `json:"error"`
}

func (driver *Driver) resourceTierBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a schema three configuration that declares the very-heavy tier within the pool$`, func() error {
			return driver.stageTierConfiguration(veryHeavyDeclaration)
		}),
		step(`^a schema three configuration without the very-heavy tier$`, func() error {
			return driver.stageTierConfiguration("")
		}),
		step(`^a schema three configuration whose very-heavy tier is (.+)$`, driver.stageTierDeclaration),
		step(`^a schema two configuration$`, func() error {
			return driver.stageTierDocument(`{"schemaVersion":2}`)
		}),
		step(`^a schema one configuration$`, func() error {
			return driver.stageTierDocument(`{"schemaVersion":1}`)
		}),
		step(`^no configuration file$`, driver.noTierConfiguration),
		step(`^a guarded run requests the very-heavy tier with a command that exits zero$`, func() error {
			return driver.runTierRun(veryHeavyTierName, nil)
		}),
		step(`^a guarded run requests the very-heavy tier with output json and a command that exits zero$`, func() error {
			return driver.runTierRun(veryHeavyTierName, []string{tierOutputFlag, tierJSONValue})
		}),
		step(`^a guarded run requests the very-heavy tier with an explicit CPU of three and a command that exits zero$`, func() error {
			return driver.runTierRun(veryHeavyTierName, []string{tierReserveCPUFlag, "3"})
		}),
		step(`^a guarded run requests the tier very_heavy with a command that exits zero$`, func() error {
			return driver.runTierRun("very_heavy", nil)
		}),
		step(`^a guarded run requests the light tier with a command that exits zero$`, func() error {
			return driver.runTierRun(lightTierName, nil)
		}),
		step(`^history is filtered by the very-heavy tier$`, func() error {
			return driver.runTierInvocation(driver.leaseRoot, []string{historyCommandName, jsonFlag, resourceTierFlag, veryHeavyTierName})
		}),
		step(`^the tier run exits (\d+)$`, driver.requireTierExit),
		step(`^the tier run exits (\d+) naming (hippo\.[a-z.-]+) with "([^"]+)"$`, driver.requireTierRefusal),
		step(`^the tier run reports exit code 2 and retryable false in its JSON error$`, driver.requireTierJSONRefusal),
		step(`^the tier history row reads (\S+) with outcome (\S+)$`, driver.requireTierHistoryRow),
		step(`^the very-heavy tier queue deadline is four hours$`, driver.requireVeryHeavyDeadline),
		step(`^the tier payload did not run and no history row was written$`, driver.requireTierNoEffects),
		step(`^the tier history filter is not refused as a usage mistake$`, driver.requireTierFilterAccepted),
	}
}

// stageTierConfiguration gives the scenario a fresh evidence root and a schema-3
// file with the three required tiers and the optional declaration, if any.
func (driver *Driver) stageTierConfiguration(optional string) error {
	return driver.stageTierDocument(tierDocument(optional))
}

// noTierConfiguration gives the scenario a fresh evidence root and no configuration file.
func (driver *Driver) noTierConfiguration() error {
	driver.configPath = ""

	return driver.emptyGuardedRunRoot()
}

// stageTierDeclaration stages a schema-3 file whose very-heavy tier is one of the
// malformed declarations a scenario names.
func (driver *Driver) stageTierDeclaration(phrase string) error {
	declaration, known := tierDeclarations[phrase]
	if !known {
		return fmt.Errorf("no very-heavy declaration is named %q", phrase)
	}

	return driver.stageTierConfiguration(declaration)
}

// stageTierDocument writes document as the scenario's configuration in a fresh evidence root.
func (driver *Driver) stageTierDocument(document string) error {
	if err := driver.emptyGuardedRunRoot(); err != nil {
		return err
	}
	driver.configPath = filepath.Join(driver.leaseRoot, tierConfigFileName)

	return os.WriteFile(driver.configPath, []byte(document), 0o600)
}

// runTierRun starts a guarded run of tier whose payload writes a marker file
// and exits zero, so a refused run is visible as a missing marker.
func (driver *Driver) runTierRun(tier string, extra []string) error {
	if driver.leaseRoot == "" {
		if err := driver.emptyGuardedRunRoot(); err != nil {
			return err
		}
	}
	marker := filepath.Join(driver.leaseRoot, tierPayloadMarker)
	arguments := []string{
		runCommandName, "--class", string(policy.TaskEphemeral), resourceTierFlag, tier,
		sourceFlagName, hippoFixtureName, diskPathFlag, driver.leaseRoot,
	}
	if driver.configPath != "" {
		arguments = append(arguments, configFlag, driver.configPath)
	}
	arguments = append(arguments, extra...)
	arguments = append(arguments, "--", shellPath, "-c", "printf ran > '"+marker+"'; exit 0")

	return driver.runTierInvocation(driver.leaseRoot, arguments)
}

// runTierInvocation runs the compiled binary, or the guard in process, over root.
// Unlike the shared helpers, it clears HIPPO_CONFIG, so no workstation file can
// stand in for the scenario's configuration.
func (driver *Driver) runTierInvocation(root string, arguments []string) error {
	if driver.mode != e2eMode {
		return driver.runGuardInRoot(root, arguments...)
	}

	return driver.runBinaryWith(map[string]string{hippoRootEnvironment: root, tierEnvironmentConfig: ""}, arguments...)
}

func (driver *Driver) requireTierExit(expected string) error {
	want, err := strconv.Atoi(expected)
	if err != nil {
		return err
	}
	if driver.exitCode != want {
		return fmt.Errorf("tier run exit=%d stderr=%q, want %d", driver.exitCode, driver.errorOutput, want)
	}

	return nil
}

// requireTierRefusal checks the exit status, the named code, and the phrase in stderr.
func (driver *Driver) requireTierRefusal(expected, code, phrase string) error {
	if err := driver.requireTierExit(expected); err != nil {
		return err
	}
	if !strings.Contains(driver.errorOutput, "["+code+"]") || !strings.Contains(driver.errorOutput, phrase) {
		return fmt.Errorf("tier run stderr %q, want code %s and %q", driver.errorOutput, code, phrase)
	}

	return nil
}

// requireTierJSONRefusal reads the JSON failure body a refused run writes after its text line.
func (driver *Driver) requireTierJSONRefusal() error {
	index := strings.Index(driver.errorOutput, "\n{")
	if index < 0 {
		return fmt.Errorf("tier run wrote no JSON failure body: stderr=%q", driver.errorOutput)
	}
	var body tierFailureBody
	if err := json.Unmarshal([]byte(driver.errorOutput[index+1:]), &body); err != nil {
		return fmt.Errorf("tier run wrote an unreadable JSON failure body: %w", err)
	}
	if body.ExitCode != 2 || driver.exitCode != 2 || body.Error.Code != "hippo.args.invalid" || body.Error.Retryable {
		return fmt.Errorf("tier run JSON failure %+v with exit %d, want exitCode 2, hippo.args.invalid, not retryable", body, driver.exitCode)
	}

	return nil
}

// tierHistoryRows lists the history rows the evidence root holds. A root with no
// rows prints no JSON, and that is an empty list.
func (driver *Driver) tierHistoryRows() ([]tierHistoryRow, error) {
	if err := driver.runTierInvocation(driver.leaseRoot, []string{historyCommandName, jsonFlag}); err != nil {
		return nil, err
	}
	if strings.TrimSpace(driver.output) == "" {
		return nil, nil
	}
	var payload struct {
		Rows []tierHistoryRow `json:"rows"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return nil, fmt.Errorf("history output unreadable (exit %d): %w", driver.exitCode, err)
	}

	return payload.Rows, nil
}

func (driver *Driver) requireTierHistoryRow(tier, outcome string) error {
	rows, err := driver.tierHistoryRows()
	if err != nil {
		return err
	}
	if len(rows) != 1 || rows[0].ResourceTier != tier || rows[0].Outcome != outcome {
		return fmt.Errorf("history rows %+v, want one row that reads tier %s with outcome %s", rows, tier, outcome)
	}

	return nil
}

// requireVeryHeavyDeadline loads the scenario's file and reads the tier's queue deadline.
func (driver *Driver) requireVeryHeavyDeadline() error {
	loaded, err := resourceconfig.Load(driver.configPath, true)
	if err != nil {
		return err
	}
	tier, declared := loaded.Coordination.Tiers[coordination.ResourceTierVeryHeavy.String()]
	if !declared || tier.QueueDeadline != tierDeadlineHours*time.Hour {
		return fmt.Errorf("very-heavy tier = %+v declared=%t, want a four-hour queue deadline", tier, declared)
	}

	return nil
}

// requireTierNoEffects checks that no payload ran and no history row was written.
func (driver *Driver) requireTierNoEffects() error {
	if _, err := os.Stat(filepath.Join(driver.leaseRoot, tierPayloadMarker)); err == nil {
		return errors.New("the refused run's payload ran")
	}
	rows, err := driver.tierHistoryRows()
	if err != nil {
		return err
	}
	if len(rows) != 0 {
		return fmt.Errorf("a refused run left history rows %+v", rows)
	}

	return nil
}

// requireTierFilterAccepted checks that history did not refuse the tier as a usage mistake.
func (driver *Driver) requireTierFilterAccepted() error {
	if driver.exitCode == 2 || strings.Contains(driver.errorOutput, "resource tier") {
		return fmt.Errorf("history refused the very-heavy filter: exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}

	return nil
}
