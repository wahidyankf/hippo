package support

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	resourceconfig "github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/tests/contract"
)

// overlappingRunEvidence gives, for each evidence phrase, the summary fields of one
// completed run that overlapped another owner. The healthy run passes every
// promotion check; the swap run differs only in its swap-out count.
var overlappingRunEvidence = map[string]map[string]any{
	"passed healthy": {
		"peakOwnerCount": 2, "availableNonCompressedEstimateMinBytes": 12884901888,
		"memoryPressureLevelMax": 1, "cpuUtilizationP95Percent": 20.0, "swapOutsDelta": 0,
	},
	"recorded swap-outs": {
		"peakOwnerCount": 2, "availableNonCompressedEstimateMinBytes": 12884901888,
		"memoryPressureLevelMax": 1, "cpuUtilizationP95Percent": 20.0, "swapOutsDelta": 3,
	},
}

// coordinationLimitDocuments gives, for each coordination limit a scenario names,
// the schema-2 file that sets that one limit and nothing else. Each file holds
// one refused value, so the refusal can only come from that value.
var coordinationLimitDocuments = map[string]string{
	"maxMemoryMiB 255":   `{"schemaVersion":2,"coordination":{"maxMemoryMiB":255}}`,
	"maxActiveOwners 21": `{"schemaVersion":2,"coordination":{"maxActiveOwners":21}}`,
}

func (driver *Driver) reservationsAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a schema three configuration whose coordination mode is exclusive$`, func() error {
			return driver.stageTierDocument(strings.Replace(tierDocument(""), `"coordination":{`, `"coordination":{"mode":"exclusive",`, 1))
		}),
		step(`^a schema one configuration that names coordination$`, func() error {
			return driver.stageTierDocument(`{"schemaVersion":1,"coordination":{"maxCpu":2}}`)
		}),
		step(`^a schema two configuration whose coordination sets (maxMemoryMiB 255|maxActiveOwners 21)$`, driver.stageSchemaTwoCoordinationLimit),
		step(`^a schema two configuration that sets the balanced profile to seven automatic owner shares$`, func() error {
			return driver.stageTierDocument(`{"schemaVersion":2,"coordination":{"automaticOwnerShares":{"balanced":7}}}`)
		}),
		step(`^the configuration is loaded$`, driver.loadStagedConfiguration),
		step(`^the balanced profile holds seven automatic owner shares$`, driver.requireBalancedOwnerShares),
		step(`^a schema three configuration whose promotion gate needs one overlapping completed run$`, func() error {
			return driver.stageTierDocument(tierDocument(""))
		}),
		step(`^an overlapping completed run that (passed healthy|recorded swap-outs)$`, driver.stageOverlappingRun),
		step(`^the owner promotion is (open|closed)$`, driver.requireOwnerPromotion),
	}
}

// stageOverlappingRun writes one completed summary that overlapped another owner,
// into the scenario's evidence root, with the fields the phrase names.
func (driver *Driver) stageOverlappingRun(phrase string) error {
	fields, known := overlappingRunEvidence[phrase]
	if !known {
		return fmt.Errorf("no overlapping run is named %q", phrase)
	}

	return writeChildUsageSummary(driver.leaseRoot, "overlapping-run", time.Now(), fields)
}

// requireOwnerPromotion holds the status promotion to open or closed: open means
// the third owner slot is eligible.
func (driver *Driver) requireOwnerPromotion(state string) error {
	if driver.exitCode != 0 {
		return fmt.Errorf("status exit=%d stderr=%q, want exit 0", driver.exitCode, driver.errorOutput)
	}
	var payload struct {
		Promotion struct {
			Eligible bool   `json:"eligible"`
			Reason   string `json:"reason"`
		} `json:"promotion"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return err
	}
	if want := state == "open"; payload.Promotion.Eligible != want {
		return fmt.Errorf("promotion eligible=%t reason=%q, want %s", payload.Promotion.Eligible, payload.Promotion.Reason, state)
	}

	return nil
}

// balancedOwnerShares is the automatic owner share the scenario configures for the balanced profile.
const balancedOwnerShares = 7

// loadStagedConfiguration reads the scenario's configuration file the way the
// compiled binary reads it, and keeps the loaded result for the check that follows.
func (driver *Driver) loadStagedConfiguration() error {
	loaded, err := resourceconfig.Load(driver.configPath, true)
	if err != nil {
		return err
	}
	driver.addCLI.loadedConfiguration = &loaded

	return nil
}

// requireBalancedOwnerShares holds the loaded balanced share to the configured value,
// which differs from the share the balanced lineage gives when nothing is configured.
func (driver *Driver) requireBalancedOwnerShares() error {
	if driver.addCLI.loadedConfiguration == nil {
		return errors.New("no configuration was loaded")
	}
	if shares := driver.addCLI.loadedConfiguration.Coordination.OwnerShares[profileBalanced]; shares != balancedOwnerShares {
		return fmt.Errorf("balanced owner shares = %d, want the configured %d", shares, balancedOwnerShares)
	}

	return nil
}

// stageSchemaTwoCoordinationLimit writes the schema-2 file that sets only the named limit.
func (driver *Driver) stageSchemaTwoCoordinationLimit(limit string) error {
	document, known := coordinationLimitDocuments[limit]
	if !known {
		return fmt.Errorf("no schema-two coordination limit is named %q", limit)
	}

	return driver.stageTierDocument(document)
}
