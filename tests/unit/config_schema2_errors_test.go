package unit_test

import (
	"strings"
	"testing"

	resourceconfig "github.com/wahidyankf/hippo/internal/config"
)

// overflowMiB exceeds math.MaxInt64/MiB so every MiB-to-bytes conversion must fail closed.
const overflowMiB = "9000000000000"

// TestSchemaTwoConfigurationRejectsUnconvertibleReserveQuantities proves each reserve override
// propagates its conversion failure instead of silently truncating an out-of-range quantity.
func TestSchemaTwoConfigurationRejectsUnconvertibleReserveQuantities(t *testing.T) {
	for _, field := range []string{
		"memoryReserveMinMiB",
		"memoryReserveMaxMiB",
		"noSwapMemoryReserveMinMiB",
		"noSwapMemoryReserveMaxMiB",
		"diskReserveMinMiB",
		"diskReserveMaxMiB",
	} {
		t.Run(field, func(t *testing.T) {
			document := `{"schemaVersion":2,"profiles":{"custom":{"extends":"balanced","` + field + `":` + overflowMiB + `}}}`
			_, err := resourceconfig.Load(writeConfig(t, document), true)
			if err == nil {
				t.Fatalf("%s accepted an unconvertible reserve quantity", field)
			}
			if !strings.Contains(err.Error(), `profile "custom"`) {
				t.Fatalf("%s error lost its profile attribution: %v", field, err)
			}
		})
	}
}

// TestConfigurationRejectsMalformedJSONDocuments proves the duplicate-field pre-pass reports
// malformed object and array payloads before the strict decoder runs.
func TestConfigurationRejectsMalformedJSONDocuments(t *testing.T) {
	for name, document := range map[string]string{
		"trailing object comma": `{"schemaVersion":2,"extra":{"a":1,}}`,
		"array duplicate field": `{"schemaVersion":2,"extra":[{"a":1,"a":2}]}`,
		"array malformed item":  `{"schemaVersion":2,"extra":[{"a":1,}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resourceconfig.Load(writeConfig(t, document), true); err == nil {
				t.Fatalf("malformed document was accepted: %s", document)
			}
		})
	}
}
