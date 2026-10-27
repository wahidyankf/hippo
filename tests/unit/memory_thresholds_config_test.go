package unit_test

import (
	"strings"
	"testing"

	resourceconfig "github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/internal/policy"
)

// TestMemoryPolicyConfigurationIsSchemaThreeOnly proves the RAM-proportional
// thresholds and the swap gate load from a schema-3 block, refuse bad values,
// and are refused outside schema 3 even when spelled to mean nothing.
func TestMemoryPolicyConfigurationIsSchemaThreeOnly(t *testing.T) {
	loadThresholds := func(block, multiple string) (resourceconfig.Result, error) {
		document := strings.Replace(veryHeavyPool, `"coordination":{`, `"coordination":{"memoryPolicy":{`+block+`},"swapUsedMaxMultiple":`+multiple+",", 1) + "}}}"

		return resourceconfig.Load(writeConfig(t, document), true)
	}

	loaded, err := loadThresholds(
		`"warningPercent":25,"criticalPercent":5,"warningFloorMiB":5120,"criticalFloorMiB":1024`, "2",
	)
	if err != nil {
		t.Fatalf("valid thresholds refused: %v", err)
	}
	memory := loaded.Coordination.MemoryPolicy
	if memory.WarningPercent != 25 || memory.CriticalPercent != 5 ||
		memory.WarningFloorBytes != 5120*policy.MiB || memory.CriticalFloorBytes != 1024*policy.MiB {
		t.Fatalf("memory policy = %+v, want the configured percents and floors", memory)
	}
	if loaded.Coordination.SwapUsedMaxMultiple != 2 {
		t.Fatalf("swap multiple = %v, want 2", loaded.Coordination.SwapUsedMaxMultiple)
	}

	partial, err := loadThresholds(`"warningPercent":25`, "0.5")
	if err != nil {
		t.Fatalf("partial thresholds refused: %v", err)
	}
	if partial.Coordination.MemoryPolicy != (policy.MemoryPolicy{WarningPercent: 25}) {
		t.Fatalf("partial policy = %+v, want only the warning percent", partial.Coordination.MemoryPolicy)
	}
	if partial.Coordination.SwapUsedMaxMultiple != 0.5 {
		t.Fatalf("partial multiple = %v, want 0.5", partial.Coordination.SwapUsedMaxMultiple)
	}

	plain, err := resourceconfig.Load(writeConfig(t, veryHeavyPool+"}}}"), true)
	if err != nil {
		t.Fatalf("file without thresholds refused: %v", err)
	}
	if plain.Coordination.MemoryPolicy != (policy.MemoryPolicy{}) || plain.Coordination.SwapUsedMaxMultiple != 0 {
		t.Fatalf("absent knobs = %+v and %v, want zeros",
			plain.Coordination.MemoryPolicy, plain.Coordination.SwapUsedMaxMultiple)
	}

	cases := []struct {
		name     string
		block    string
		multiple string
		message  string
	}{
		{"warning percent zero", `"warningPercent":0`, "2", "warning memory percent is invalid"},
		{"warning percent negative", `"warningPercent":-5`, "2", "warning memory percent is invalid"},
		{"warning percent over a hundred", `"warningPercent":101`, "2", "warning memory percent is invalid"},
		{"critical percent zero", `"criticalPercent":0`, "2", "critical memory percent is invalid"},
		{"critical percent negative", `"criticalPercent":-1`, "2", "critical memory percent is invalid"},
		{"critical percent over a hundred", `"criticalPercent":100.5`, "2", "critical memory percent is invalid"},
		{"warning percent text", `"warningPercent":"25"`, "2", "cannot unmarshal"},
		{"warning floor negative", `"warningFloorMiB":-1`, "2", "MiB value exceeds representable bytes"},
		{"critical floor negative", `"criticalFloorMiB":-1`, "2", "MiB value exceeds representable bytes"},
		{"swap multiple zero", `"warningPercent":25`, "0", "swap used multiple must be positive"},
		{"swap multiple negative", `"warningPercent":25`, "-1", "swap used multiple must be positive"},
		{"swap multiple text", `"warningPercent":25`, `"2"`, "cannot unmarshal"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := loadThresholds(testCase.block, testCase.multiple); err == nil ||
				!strings.Contains(err.Error(), testCase.message) {
				t.Fatalf("error = %v, want %q", err, testCase.message)
			}
		})
	}

	if _, err := loadThresholds(`"warningPercent":25,"warningPct":10`, "2"); err == nil ||
		!strings.Contains(err.Error(), `unknown field "warningPct"`) {
		t.Fatalf("error = %v, want the unknown field refusal", err)
	}

	schemaTwo := `{"schemaVersion":2,"coordination":{"memoryPolicy":{"warningPercent":25}}}`
	if _, err := resourceconfig.Load(writeConfig(t, schemaTwo), true); err == nil ||
		!strings.Contains(err.Error(), "memoryPolicy requires schema 3") {
		t.Fatalf("schema two memoryPolicy error = %v, want the schema-three refusal", err)
	}
	schemaTwoMultiple := `{"schemaVersion":2,"coordination":{"swapUsedMaxMultiple":0.5}}`
	if _, err := resourceconfig.Load(writeConfig(t, schemaTwoMultiple), true); err == nil ||
		!strings.Contains(err.Error(), "swapUsedMaxMultiple requires schema 3") {
		t.Fatalf("schema two swapUsedMaxMultiple error = %v, want the schema-three refusal", err)
	}
	schemaTwoOff := `{"schemaVersion":2,"coordination":{"swapUsedMaxMultiple":0}}`
	if _, err := resourceconfig.Load(writeConfig(t, schemaTwoOff), true); err == nil {
		t.Fatal("schema two accepted a meaningless swapUsedMaxMultiple")
	}
}
