package support

import (
	"fmt"
	"strings"

	"github.com/wahidyankf/hippo/tests/contract"
)

// schemaThreeOmissions gives, for each required pool, owner, promotion, or tier
// field a scenario can drop, the exact text of that field in tierDocument. A
// scenario drops one field from an otherwise valid schema-3 file.
var schemaThreeOmissions = map[string]string{
	"a pool limit":       `"maxCpu":2,`,
	"an owner limit":     `"baseActiveOwners":2,`,
	"the promotion gate": `"promotion":{"completedRuns":1,"minimumSources":1,"minimumAvailableMemoryMiB":256,"maximumCpuP95Percent":98},`,
	"the heavy tier":     `,"heavy":{"minimumCpu":1,"maximumCpu":2,"minimumMemoryMiB":256,"maximumMemoryMiB":512,"queueDeadline":"4h"}`,
}

// memoryPolicySettings gives, for each memoryPolicy setting a scenario names, the
// block that sets only that setting, so the one defect is the refused value.
var memoryPolicySettings = map[string]string{
	"warningPercent 0":    `"memoryPolicy":{"warningPercent":0},`,
	"criticalPercent 101": `"memoryPolicy":{"criticalPercent":101},`,
	"warningFloorMiB -1":  `"memoryPolicy":{"warningFloorMiB":-1},`,
}

// schemaTwoCountSwapDocument is a schema-2 file that names countSwap, which only
// schema 3 may carry.
const schemaTwoCountSwapDocument = `{"schemaVersion":2,"coordination":{"countSwap":true}}`

// customProfileDocument is a schema-2 file whose custom profile lowers the
// immutable memory reserve floor below 128 MiB.
const customProfileDocument = `{"schemaVersion":2,"defaultProfile":"custom","profiles":{"custom":{"extends":"balanced","memoryReserveMinMiB":64}}}`

func (driver *Driver) configAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a schema two configuration file that names countSwap$`, func() error {
			return driver.stageTierDocument(schemaTwoCountSwapDocument)
		}),
		step(`^a schema three configuration whose memoryPolicy sets (warningPercent 0|criticalPercent 101|warningFloorMiB -1)$`, driver.stageMemoryPolicy),
		step(`^a schema three configuration whose swapUsedMaxMultiple is (0|-1)$`, driver.stageSwapUsedMaxMultiple),
		step(`^a configuration whose custom profile sets memoryReserveMinMiB to 64$`, func() error {
			return driver.stageTierDocument(customProfileDocument)
		}),
		step(`^JSON status is requested with that configuration file$`, driver.statusWithStagedConfiguration),
		step(`^a schema three configuration without (a pool limit|an owner limit|the promotion gate|the heavy tier)$`, driver.stageSchemaThreeWithout),
	}
}

// stageMemoryPolicy writes a valid schema-3 file whose memoryPolicy sets only the
// named, refused value.
func (driver *Driver) stageMemoryPolicy(setting string) error {
	block, known := memoryPolicySettings[setting]
	if !known {
		return fmt.Errorf("unknown memoryPolicy setting %q", setting)
	}

	return driver.stageTierDocument(insertBeforeTiers(tierDocument(""), block))
}

// stageSwapUsedMaxMultiple writes a valid schema-3 file whose swap gate is the
// named value.
func (driver *Driver) stageSwapUsedMaxMultiple(value string) error {
	return driver.stageTierDocument(insertBeforeTiers(tierDocument(""), `"swapUsedMaxMultiple":`+value+`,`))
}

// insertBeforeTiers places fragment in the coordination object, before its tiers.
func insertBeforeTiers(document, fragment string) string {
	return strings.Replace(document, `"tiers":{`, fragment+`"tiers":{`, 1)
}

// stageSchemaThreeWithout writes the valid schema-3 file with one required field
// removed. A removal that does not match the fixture exactly is refused, so the
// scenario can never pass on a file it did not change.
func (driver *Driver) stageSchemaThreeWithout(missing string) error {
	fragment, known := schemaThreeOmissions[missing]
	if !known {
		return fmt.Errorf("unknown required field %q", missing)
	}
	document := tierDocument("")
	if strings.Count(document, fragment) != 1 {
		return fmt.Errorf("the schema-three fixture does not hold %s exactly once", missing)
	}

	return driver.stageTierDocument(strings.Replace(document, fragment, "", 1))
}

// statusWithStagedConfiguration asks for status with the configuration file the
// scenario staged, named by --config, in a root the scenario owns.
func (driver *Driver) statusWithStagedConfiguration() error {
	arguments := []string{statusCommandName, jsonFlag, diskPathFlag, ".", configFlag, driver.configPath}

	return driver.runStatusWith(map[string]string{hippoRootEnvironment: driver.leaseRoot}, arguments...)
}
