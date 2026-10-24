package unit_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	resourceconfig "github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
)

// veryHeavyPool is a schema-3 coordination block whose pool holds the optional
// tier's bounds. Its required tiers are the ones every schema-3 file declares.
const veryHeavyPool = `{"schemaVersion":3,"coordination":{"maxCpu":8,"maxMemoryMiB":16384,"baseActiveOwners":2,"maxActiveOwners":3,"promotion":{"completedRuns":1,"minimumSources":1,"minimumAvailableMemoryMiB":10240,"maximumCpuP95Percent":75},"emergencyAvailableMemoryMiB":6144,"tiers":{` +
	`"light":{"minimumCpu":1,"maximumCpu":2,"minimumMemoryMiB":1024,"maximumMemoryMiB":2048,"queueDeadline":"30m"},` +
	`"standard":{"minimumCpu":2,"maximumCpu":4,"minimumMemoryMiB":3072,"maximumMemoryMiB":6144,"queueDeadline":"90m"},` +
	`"heavy":{"minimumCpu":4,"maximumCpu":8,"minimumMemoryMiB":8192,"maximumMemoryMiB":16384,"queueDeadline":"4h"}`

// veryHeavyDeclaration is one valid optional tier, sized for the pool above.
const veryHeavyDeclaration = `"very-heavy":{"minimumCpu":4,"maximumCpu":6,"minimumMemoryMiB":6144,"maximumMemoryMiB":10240,"queueDeadline":"4h"}`

func withVeryHeavy(declaration string) string {
	return veryHeavyPool + "," + declaration + "}}}"
}

func TestSchemaThreeAcceptsDeclaredVeryHeavyTierWithItsOwnBounds(t *testing.T) {
	loaded, err := resourceconfig.Load(writeConfig(t, withVeryHeavy(veryHeavyDeclaration)), true)
	if err != nil {
		t.Fatalf("declared very-heavy tier refused: %v", err)
	}
	tier, declared := loaded.Coordination.Tiers[coordination.ResourceTierVeryHeavy.String()]
	if !declared || len(loaded.Coordination.Tiers) != 4 {
		t.Fatalf("tiers = %v, want the three required tiers and very-heavy", loaded.Coordination.Tiers)
	}
	if tier.Maximum.CPU != 6 || tier.Minimum.CPU != 4 || tier.QueueDeadline != 4*time.Hour {
		t.Fatalf("very-heavy tier = %+v, want its declared bounds and a 4h deadline", tier)
	}
}

func TestSchemaThreeRefusesMalformedDeclaredVeryHeavyTiers(t *testing.T) {
	cases := []struct {
		name        string
		declaration string
		message     string
	}{
		{
			name:        "maximum CPU above the pool",
			declaration: `"very-heavy":{"minimumCpu":4,"maximumCpu":9,"minimumMemoryMiB":6144,"maximumMemoryMiB":10240,"queueDeadline":"4h"}`,
			message:     "schema 3 very-heavy resource tier is outside the pool",
		},
		{
			name:        "null declaration is outside the pool",
			declaration: `"very-heavy":null`,
			message:     "schema 3 very-heavy resource tier is outside the pool",
		},
		{
			name:        "empty declaration is outside the pool",
			declaration: `"very-heavy":{}`,
			message:     "schema 3 very-heavy resource tier is outside the pool",
		},
		{
			name:        "invalid queue deadline",
			declaration: `"very-heavy":{"minimumCpu":4,"maximumCpu":6,"minimumMemoryMiB":6144,"maximumMemoryMiB":10240,"queueDeadline":"never"}`,
			message:     "schema 3 very-heavy queue deadline is invalid",
		},
		{
			name:        "underscore typo is an unknown tier",
			declaration: `"very_heavy":{"minimumCpu":4,"maximumCpu":6,"minimumMemoryMiB":6144,"maximumMemoryMiB":10240,"queueDeadline":"4h"}`,
			message:     "schema 3 contains an unknown resource tier",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := resourceconfig.Load(writeConfig(t, withVeryHeavy(test.declaration)), true)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Load error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestSchemaThreeStillRequiresTheThreeRequiredTiersBesideAnOptionalOne(t *testing.T) {
	document := `{"schemaVersion":3,"coordination":{"maxCpu":8,"maxMemoryMiB":16384,"baseActiveOwners":2,"maxActiveOwners":3,"promotion":{"completedRuns":1,"minimumSources":1,"minimumAvailableMemoryMiB":10240,"maximumCpuP95Percent":75},"emergencyAvailableMemoryMiB":6144,"tiers":{` +
		`"heavy":{"minimumCpu":4,"maximumCpu":8,"minimumMemoryMiB":8192,"maximumMemoryMiB":16384,"queueDeadline":"4h"},` + veryHeavyDeclaration + `}}}`
	_, err := resourceconfig.Load(writeConfig(t, document), true)
	if err == nil || !strings.Contains(err.Error(), "schema 3 requires light resource tier") {
		t.Fatalf("Load error = %v, want the missing light tier refused", err)
	}
}

func TestThreeTierSchemaThreeConfigsLoadAndHashAsBefore(t *testing.T) {
	document := veryHeavyPool + `}}}`
	path := writeConfig(t, document)
	loaded, err := resourceconfig.Load(path, true)
	if err != nil {
		t.Fatalf("three-tier configuration refused: %v", err)
	}
	if len(loaded.Coordination.Tiers) != 3 {
		t.Fatalf("tiers = %d, want exactly the three required tiers", len(loaded.Coordination.Tiers))
	}
	if _, declared := loaded.Coordination.Tiers[coordination.ResourceTierVeryHeavy.String()]; declared {
		t.Fatal("a configuration without very-heavy gained the optional tier")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if loaded.Hash != hex.EncodeToString(digest[:]) {
		t.Fatalf("config hash = %s, want the digest of the file bytes", loaded.Hash)
	}
}
