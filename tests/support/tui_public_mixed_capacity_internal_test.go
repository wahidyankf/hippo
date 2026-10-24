package support

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	configadapter "github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

// releasedMixedCapacity is the CPU capacity the released v0.8.4 and v0.8.5
// builds offer: they subtract one CPU from the host before applying maxCpu.
func releasedMixedCapacity(logicalCPUs, maxCPU int) int {
	return min(maxCPU, max(1, logicalCPUs-1))
}

// TestMixedPublicOwnerBudgetAcceptsEachBuildsCapacityOnEveryAdmittedHost reproduces
// the CI failure: one shared ledger is written by the released binary and then
// validated against the current formula, which must agree with it on every host
// that can admit the two declared owners.
func TestMixedPublicOwnerBudgetAcceptsEachBuildsCapacityOnEveryAdmittedHost(t *testing.T) {
	for _, variant := range []string{publicTUIMixedVariant, publicTUIMixedV085Variant, publicTUIMixedV084Variant} {
		t.Run(variant, func(t *testing.T) {
			configured := loadMixedPublicConfiguration(t, variant)
			validate := mixedPublicBudgetValidator(variant)
			for logicalCPUs := 3; logicalCPUs <= 12; logicalCPUs++ {
				t.Run(fmt.Sprintf("%d-logical-CPUs", logicalCPUs), func(t *testing.T) {
					current := plannedMixedCapacity(t, configured, logicalCPUs)
					released := releasedMixedCapacity(logicalCPUs, configured.Coordination.MaxCPU)
					if released != current {
						t.Fatalf("released build offers %d CPUs and current build offers %d on the same host and configuration", released, current)
					}
					for _, cpu := range []int{released, current} {
						actual := coordination.ReservationTotals{
							SchemaVersion: 5, Mode: reservationMode, ActiveOwners: 2,
							Capacity:  coordination.ReservationVector{CPU: cpu, MemoryBytes: mixedPublicMemoryCap(variant)},
							Allocated: coordination.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB},
						}
						if err := validate(actual, logicalCPUs); err != nil {
							t.Fatalf("real %d-CPU capacity from a build on this host rejected: %v", cpu, err)
						}
					}
				})
			}
		})
	}
}

// TestMixedPublicOwnerBudgetRefusesHostsThatCannotOfferTwoOwners checks the explicit
// host precondition: with fewer than three logical CPUs the released build offers
// one CPU, so two one-CPU owners cannot both be active.
func TestMixedPublicOwnerBudgetRefusesHostsThatCannotOfferTwoOwners(t *testing.T) {
	for _, variant := range []string{publicTUIMixedVariant, publicTUIMixedV085Variant, publicTUIMixedV084Variant} {
		t.Run(variant, func(t *testing.T) {
			configured := loadMixedPublicConfiguration(t, variant)
			validate := mixedPublicBudgetValidator(variant)
			for logicalCPUs := range 3 {
				t.Run(fmt.Sprintf("%d-logical-CPUs", logicalCPUs), func(t *testing.T) {
					for _, cpu := range []int{releasedMixedCapacity(logicalCPUs, configured.Coordination.MaxCPU), max(1, logicalCPUs), 2} {
						actual := coordination.ReservationTotals{
							SchemaVersion: 5, Mode: reservationMode, ActiveOwners: 2,
							Capacity:  coordination.ReservationVector{CPU: cpu, MemoryBytes: mixedPublicMemoryCap(variant)},
							Allocated: coordination.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB},
						}
						if err := validate(actual, logicalCPUs); err == nil {
							t.Fatalf("host with %d logical CPUs accepted as offering two one-CPU owners at capacity %d", logicalCPUs, cpu)
						}
					}
				})
			}
		})
	}
}

func mixedPublicMemoryCap(variant string) int64 {
	if variant == publicTUIMixedVariant {
		return 8 * policy.GiB
	}
	return 3 * policy.GiB
}

func loadMixedPublicConfiguration(t *testing.T, variant string) configadapter.Result {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(directory, "public-state")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := &PublicTUILedger{directory: directory, root: root, configuration: filepath.Join(directory, "config.json")}
	if err := fixture.initializeObservation(variant); err != nil {
		t.Fatal(err)
	}
	configured, err := configadapter.Load(fixture.configuration, true)
	if err != nil {
		t.Fatal(err)
	}
	return configured
}

// plannedMixedCapacity asks the current planner, with the fixture's real loaded
// configuration and the balanced safety reserve, what CPU capacity it offers.
func plannedMixedCapacity(t *testing.T, configured configadapter.Result, logicalCPUs int) int {
	t.Helper()
	memory := 32 * policy.GiB
	disk := 64 * policy.GiB
	sample := policy.Sample{PhysicalMemoryBytes: memory, EffectiveMemoryLimitBytes: memory, AvailableMemoryBytes: &memory, AvailableParallelism: logicalCPUs, DiskFreeBytes: &disk, DiskTotalBytes: &disk, SwapState: "available"}
	resolution, err := configured.Catalog.Resolve("balanced", policy.TaskEphemeral, sample)
	if err != nil {
		t.Fatal(err)
	}
	settings := coordination.ReservationPolicy{Enabled: true, MaxCPU: configured.Coordination.MaxCPU, MaxMemoryBytes: configured.Coordination.MaxMemoryBytes, MaxActiveOwners: configured.Coordination.MaxActiveOwners, OwnerShares: configured.Coordination.OwnerShares}
	plan, err := coordination.PlanReservation(sample, resolution, settings, 1, policy.GiB)
	if err != nil {
		t.Fatal(err)
	}
	return plan.Capacity.CPU
}
