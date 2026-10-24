package unit_test

import (
	"strings"
	"testing"
	"time"

	resourceconfig "github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

// TestSwapAllowanceEntersThePlannedPoolOnly proves the pool grows by the
// allowance, and that a tier which needs the allowance replans without it.
func TestSwapAllowanceEntersThePlannedPoolOnly(t *testing.T) {
	host := swapHost(7618*policy.MiB, 4*policy.GiB, 8*policy.GiB, 0)
	host.AvailableParallelism = 9
	settings := reservationPolicy()
	settings.MaxMemoryBytes = 16 * policy.GiB
	settings.Tiers = map[string]coordination.ResourceTierPolicy{
		"heavy": {
			Minimum:       coordination.ReservationVector{CPU: 2, MemoryBytes: 8 * policy.GiB},
			Maximum:       coordination.ReservationVector{CPU: 2, MemoryBytes: 10 * policy.GiB},
			QueueDeadline: time.Hour,
		},
	}
	resolution := reservationResolution("balanced")

	withoutCounting, err := coordination.PlanReservation(host, resolution, settings, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	counted := policy.WithSwapAllowance(host, true)
	withCounting, err := coordination.PlanReservation(counted, resolution, settings, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantPool := 7618*policy.MiB + 8*policy.GiB - 4*policy.GiB
	if withCounting.Capacity.MemoryBytes != wantPool {
		t.Fatalf("counted pool = %d, want %d", withCounting.Capacity.MemoryBytes, wantPool)
	}
	if withoutCounting.Capacity.MemoryBytes != 7618*policy.MiB-4*policy.GiB {
		t.Fatalf("pool without counting = %d", withoutCounting.Capacity.MemoryBytes)
	}

	if _, _, err := coordination.PlanTierReservation(counted, resolution, settings, "heavy", 0, 0); err != nil {
		t.Fatalf("heavy tier refused with counting on: %v", err)
	}
	if _, _, err := coordination.PlanTierReservation(host, resolution, settings, "heavy", 0, 0); err == nil {
		t.Fatal("heavy tier admitted without the swap allowance")
	}
	capped := settings
	capped.MaxMemoryBytes = 9 * policy.GiB
	cappedPlan, err := coordination.PlanReservation(counted, resolution, capped, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cappedPlan.Capacity.MemoryBytes != 9*policy.GiB {
		t.Fatalf("maxMemoryMiB did not cap the pool: %d", cappedPlan.Capacity.MemoryBytes)
	}
}

// TestSwapCountingConfigurationIsSchemaThreeOnly proves countSwap is a boolean
// that only a schema-3 file may carry.
func TestSwapCountingConfigurationIsSchemaThreeOnly(t *testing.T) {
	loadWith := func(countSwap string) (resourceconfig.Result, error) {
		document := strings.Replace(veryHeavyPool, `"coordination":{`, `"coordination":{"countSwap":`+countSwap+`,`, 1) + "}}}"

		return resourceconfig.Load(writeConfig(t, document), true)
	}

	loaded, err := loadWith("true")
	if err != nil || !loaded.Coordination.CountSwap {
		t.Fatalf("schema three countSwap true: CountSwap=%v err=%v", loaded.Coordination.CountSwap, err)
	}
	loaded, err = loadWith("false")
	if err != nil || loaded.Coordination.CountSwap {
		t.Fatalf("schema three countSwap false: CountSwap=%v err=%v", loaded.Coordination.CountSwap, err)
	}
	if _, err := loadWith(`"yes"`); err == nil {
		t.Fatal("a non-boolean countSwap was accepted")
	}

	plain, err := resourceconfig.Load(writeConfig(t, veryHeavyPool+"}}}"), true)
	if err != nil || plain.Coordination.CountSwap {
		t.Fatalf("file without countSwap: CountSwap=%v err=%v", plain.Coordination.CountSwap, err)
	}

	schemaTwo := `{"schemaVersion":2,"coordination":{"countSwap":true}}`
	if _, err := resourceconfig.Load(writeConfig(t, schemaTwo), true); err == nil ||
		!strings.Contains(err.Error(), "countSwap requires schema 3") {
		t.Fatalf("schema two countSwap error = %v, want the schema-three refusal", err)
	}
	schemaTwoOff := `{"schemaVersion":2,"coordination":{"countSwap":false}}`
	if _, err := resourceconfig.Load(writeConfig(t, schemaTwoOff), true); err == nil {
		t.Fatal("schema two accepted countSwap false")
	}
}
