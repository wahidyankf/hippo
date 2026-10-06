package unit_test

import (
	"strconv"
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

// TestCoordinationModeIsRefusedWhereTheDocumentIsDecoded pins where an unknown mode is refused. Each document also
// names a profile that extends one that does not exist, which building the catalog refuses; that build runs after the
// document is decoded, so the error names the mode only when the mode is refused first, at decode.
func TestCoordinationModeIsRefusedWhereTheDocumentIsDecoded(t *testing.T) {
	for _, mode := range []string{"exclusive", "batch", "Reservation", " reservation", "reservation "} {
		t.Run(mode, func(t *testing.T) {
			document := `{"schemaVersion":2,"profiles":{"local":{"extends":"missing"}},"coordination":{"mode":` + strconv.Quote(mode) + `}}`
			_, err := resourceconfig.Load(writeConfig(t, document), true)
			if err == nil || !strings.Contains(err.Error(), "coordination mode "+strconv.Quote(mode)) {
				t.Fatalf("a document naming the coordination mode %q was refused as %v, want a refusal that names the mode", mode, err)
			}
		})
	}
}

// TestAnEmptyOrAbsentCoordinationModeLoadsAsTheDefault holds what the decoder has always accepted: no mode, an empty
// one, and the one named mode all load a schema-2 document with the default, reservation coordination.
func TestAnEmptyOrAbsentCoordinationModeLoadsAsTheDefault(t *testing.T) {
	for name, document := range map[string]string{
		"an empty mode":   `{"schemaVersion":2,"coordination":{"mode":""}}`,
		"a null mode":     `{"schemaVersion":2,"coordination":{"mode":null}}`,
		"no mode":         `{"schemaVersion":2,"coordination":{}}`,
		"no coordination": `{"schemaVersion":2}`,
		"the named mode":  `{"schemaVersion":2,"coordination":{"mode":"reservation"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := resourceconfig.Load(writeConfig(t, document), true)
			if err != nil || result.Coordination.Mode != "reservation" || result.Coordination.SchemaVersion != 2 {
				t.Fatalf("%s loaded as %+v (%v), want schema 2 with reservation coordination", name, result.Coordination, err)
			}
		})
	}
}
