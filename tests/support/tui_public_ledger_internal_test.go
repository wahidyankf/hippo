package support

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	configadapter "github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

func TestPublicTUILedgerRejectsForeignRootsBeforeEffects(t *testing.T) {
	directory := filepath.Join(string(filepath.Separator), "owned-public-ledger-passive")
	root := filepath.Join(directory, "public-state")
	for _, test := range []struct {
		name            string
		driver          *Driver
		root, directory string
	}{
		{"nil-driver", nil, root, directory},
		{"unit", &Driver{mode: contract.Unit, temporaryPaths: []string{directory}}, root, directory},
		{"integration", &Driver{mode: contract.Integration, temporaryPaths: []string{directory}}, root, directory},
		{"unregistered-directory", &Driver{mode: contract.E2E}, root, directory},
		{"foreign-root", &Driver{mode: contract.E2E, temporaryPaths: []string{directory}}, filepath.Join(directory, "other"), directory},
		{"relative-directory", &Driver{mode: contract.E2E, temporaryPaths: []string{"relative"}}, "relative/public-state", "relative"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, err := test.driver.NewPublicTUILedger(t.Context(), test.root, test.directory)
			if err == nil || fixture != nil {
				t.Fatalf("foreign ledger preparation accepted: %v/%v", fixture, err)
			}
			if test.driver != nil && len(test.driver.stops) != 0 {
				t.Fatal("rejected ledger registered cleanup/effects")
			}
		})
	}
	driver := &Driver{mode: contract.E2E, temporaryPaths: []string{directory}}
	fixture, err := driver.NewPublicTUILedger(t.Context(), root, directory)
	if err != nil || fixture == nil || fixture.prepared || len(fixture.owners) != 0 || len(fixture.waiters) != 0 || len(driver.stops) != 1 {
		t.Fatalf("passive owned constructor changed acquisition state: %+v/%v", fixture, err)
	}
}

func TestPublicObservationFactoryIsPassiveAndModeScoped(t *testing.T) {
	for _, mode := range []string{contract.Unit, contract.Integration, contract.E2E} {
		t.Run(mode, func(t *testing.T) {
			driver := &Driver{mode: mode}
			before := driver.Bindings()
			calls, executions := 0, 0
			var calledPatterns []string
			replacement := func() error { executions++; return errors.New("external public handler marker") }
			selected := map[string]bool{
				`^two running jobs and three registered FIFO waiters with known reservation vectors$`: true,
				`^one TUI observation completes$`: true,
				`^both running identities and all three original FIFO positions retain their resource values$`: true,
			}
			after := driver.BindingsWithPublicTUIObservation(func(pattern string) any {
				calls++
				calledPatterns = append(calledPatterns, pattern)
				if selected[pattern] {
					return replacement
				}
				return nil
			})
			wantCalls, wantChanged := 0, 0
			if mode == contract.E2E {
				wantCalls, wantChanged = len(before), 3
			}
			if calls != wantCalls || executions != 0 || len(after) != len(before) || len(driver.stops) != 0 || len(driver.temporaryPaths) != 0 {
				t.Fatalf("passive replacement crossed boundary: calls=%d executions=%d bindings=%d/%d", calls, executions, len(after), len(before))
			}
			requirePublicObservationFactoryRegistry(t, mode, before, after, calledPatterns, selected, wantChanged)
		})
	}
}

func requirePublicObservationFactoryRegistry(t *testing.T, mode string, before, after []contract.StepBinding, calledPatterns []string, selected map[string]bool, wantChanged int) {
	t.Helper()
	var expectedPatterns []string
	if mode == contract.E2E {
		for _, binding := range before {
			expectedPatterns = append(expectedPatterns, binding.Pattern)
		}
	}
	if !reflect.DeepEqual(calledPatterns, expectedPatterns) {
		t.Fatal("factory did not inspect each original pattern once in registry order")
	}
	changed := 0
	for index := range before {
		if before[index].Pattern != after[index].Pattern {
			t.Fatal("factory changed original pattern/order")
		}
		replaced := reflect.ValueOf(before[index].Handler).Pointer() != reflect.ValueOf(after[index].Handler).Pointer()
		if replaced != (mode == contract.E2E && selected[before[index].Pattern]) {
			t.Fatalf("factory changed handler identity outside its selected E2E patterns: %s", before[index].Pattern)
		}
		if replaced {
			changed++
		}
	}
	if changed != wantChanged {
		t.Fatalf("factory replaced %d patterns, want %d", changed, wantChanged)
	}
}

func TestPublicTUILedgerRejectsWrongPublicIdentityFIFOAndVectors(t *testing.T) {
	vector := coordination.ReservationVector{CPU: 2, MemoryBytes: policy.GiB}
	expected := coordination.ReservationTotals{SchemaVersion: 5, Mode: reservationMode, ActiveOwners: 2, WaitingOwners: 3, Ephemeral: 2, Capacity: coordination.ReservationVector{CPU: 8, MemoryBytes: 8 * policy.GiB}, Allocated: coordination.ReservationVector{CPU: 4, MemoryBytes: 2 * policy.GiB}, Waiting: coordination.ReservationVector{CPU: 6, MemoryBytes: 3 * policy.GiB}}
	for index := range 2 {
		expected.Owners = append(expected.Owners, coordination.ReservationEntry{RunID: fmt.Sprintf("%032x", index+1), State: "active", Class: policy.TaskEphemeral, Profile: profileBalanced, Source: "owner", Tags: map[string]string{"role": "owner"}, Requested: vector, Allocated: vector})
	}
	for index := range 3 {
		expected.Waiters = append(expected.Waiters, coordination.ReservationEntry{RunID: fmt.Sprintf("%032x", index+10), State: "waiting", Position: index + 1, Class: policy.TaskEphemeral, Profile: profileBalanced, Source: "other", Tags: map[string]string{"role": "other"}, Requested: vector})
	}
	fixture := &PublicTUILedger{expected: expected}
	if err := fixture.RequirePublicTotals(fixture.Expected()); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*coordination.ReservationTotals)
	}{
		{"different-full-owner-ID", func(v *coordination.ReservationTotals) { v.Owners[0].RunID = fmt.Sprintf("%032x", 99) }},
		{"different-full-waiter-ID", func(v *coordination.ReservationTotals) { v.Waiters[2].RunID = fmt.Sprintf("%032x", 98) }},
		{"wrong-FIFO-position", func(v *coordination.ReservationTotals) { v.Waiters[2].Position = 1 }},
		{"reordered-waiters", func(v *coordination.ReservationTotals) { v.Waiters[0], v.Waiters[1] = v.Waiters[1], v.Waiters[0] }},
		{"wrong-owner-allocation", func(v *coordination.ReservationTotals) { v.Owners[0].Allocated.CPU = 1 }},
		{"wrong-waiter-demand", func(v *coordination.ReservationTotals) { v.Waiters[1].Requested.MemoryBytes = 2 * policy.GiB }},
		{"filtered-global-capacity", func(v *coordination.ReservationTotals) { v.Capacity.CPU = 2 }},
		{"filtered-global-allocation", func(v *coordination.ReservationTotals) { v.Allocated.CPU = 0 }},
		{"wrong-waiting-total", func(v *coordination.ReservationTotals) { v.Waiting.CPU = 2 }},
		{"wrong-source", func(v *coordination.ReservationTotals) { v.Owners[0].Source = "other" }},
		{"wrong-tags", func(v *coordination.ReservationTotals) { v.Waiters[0].Tags["role"] = "owner" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			actual := fixture.Expected()
			test.change(&actual)
			if err := fixture.RequirePublicTotals(actual); err == nil {
				t.Fatal("incorrect public observation accepted")
			}
		})
	}
	if !reflect.DeepEqual(fixture.Expected(), expected) {
		t.Fatal("caller mutation changed immutable expected fixture inputs")
	}
}

func TestMixedPublicOwnerBudgetUsesDeclaredHostParallelism(t *testing.T) {
	for _, host := range []struct {
		name        string
		parallelism int
		cpuCapacity int
	}{
		{"five-CPU-host-fits-both-declared-owners", 5, 4},
		{"eight-CPU-host-reserves-one", 8, 7},
		{"nine-CPU-host-reaches-configured-cap", 9, 8},
		{"sixteen-CPU-host-keeps-configured-cap", 16, 8},
	} {
		t.Run(host.name, func(t *testing.T) {
			declared := coordination.ReservationTotals{
				SchemaVersion: 5, Mode: reservationMode, ActiveOwners: 2,
				Capacity:  coordination.ReservationVector{CPU: host.cpuCapacity, MemoryBytes: 8 * policy.GiB},
				Allocated: coordination.ReservationVector{CPU: 4, MemoryBytes: 2 * policy.GiB},
			}
			if err := validateMixedPublicOwnerBudget(declared, host.parallelism); err != nil {
				t.Fatalf("declared host/configuration budget rejected: %v", err)
			}
			mixedPublicOwnerBudgetFaultControls(t, declared, host.parallelism, validateMixedPublicOwnerBudget)
		})
	}
}

func TestMixedPublicOwnerBudgetRejectsHostsBelowDeclaredOwnerCapacity(t *testing.T) {
	for _, host := range []struct {
		parallelism int
		safeCPU     int
	}{
		{0, 1}, {1, 1}, {2, 1}, {3, 2}, {4, 3},
	} {
		t.Run(fmt.Sprintf("%d-logical-CPUs", host.parallelism), func(t *testing.T) {
			for _, cpuCapacity := range []int{host.safeCPU, 8} {
				t.Run(fmt.Sprintf("%d-declared-capacity", cpuCapacity), func(t *testing.T) {
					actual := coordination.ReservationTotals{
						SchemaVersion: 5, Mode: reservationMode, ActiveOwners: 2,
						Capacity:  coordination.ReservationVector{CPU: cpuCapacity, MemoryBytes: 8 * policy.GiB},
						Allocated: coordination.ReservationVector{CPU: 4, MemoryBytes: 2 * policy.GiB},
					}
					if err := validateMixedPublicOwnerBudget(actual, host.parallelism); err == nil {
						t.Fatal("host unable to admit two declared two-CPU owners accepted, including manufactured capacity")
					}
				})
			}
		})
	}
}

func TestMixedV085PublicOwnerBudgetUsesDeclaredHostParallelism(t *testing.T) {
	for _, host := range []struct {
		parallelism int
		cpuCapacity int
	}{{3, 2}, {4, 3}, {8, 7}, {9, 8}, {16, 8}} {
		t.Run(fmt.Sprintf("%d-logical-CPUs", host.parallelism), func(t *testing.T) {
			declared := coordination.ReservationTotals{
				SchemaVersion: 5, Mode: reservationMode, ActiveOwners: 2,
				Capacity:  coordination.ReservationVector{CPU: host.cpuCapacity, MemoryBytes: 3 * policy.GiB},
				Allocated: coordination.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB},
			}
			if err := validateMixedV085PublicOwnerBudget(declared, host.parallelism); err != nil {
				t.Fatalf("declared one-CPU compatibility owner budget rejected: %v", err)
			}
			mixedPublicOwnerBudgetFaultControls(t, declared, host.parallelism, validateMixedV085PublicOwnerBudget)
		})
	}
}

func mixedPublicOwnerBudgetFaultControls(t *testing.T, declared coordination.ReservationTotals, logicalCPUs int, validate func(coordination.ReservationTotals, int) error) {
	t.Helper()
	for _, fault := range []struct {
		name   string
		change func(*coordination.ReservationTotals)
	}{
		{"schema", func(actual *coordination.ReservationTotals) { actual.SchemaVersion = 4 }},
		{"mode", func(actual *coordination.ReservationTotals) { actual.Mode = exclusiveMode }},
		{"owner-count", func(actual *coordination.ReservationTotals) { actual.ActiveOwners = 3 }},
		{"waiter-count", func(actual *coordination.ReservationTotals) { actual.WaitingOwners = 1 }},
		{"CPU-capacity-too-high", func(actual *coordination.ReservationTotals) { actual.Capacity.CPU++ }},
		{"CPU-capacity-too-low", func(actual *coordination.ReservationTotals) { actual.Capacity.CPU-- }},
		{"memory-capacity", func(actual *coordination.ReservationTotals) { actual.Capacity.MemoryBytes++ }},
		{"CPU-allocation", func(actual *coordination.ReservationTotals) { actual.Allocated.CPU++ }},
		{"memory-allocation", func(actual *coordination.ReservationTotals) { actual.Allocated.MemoryBytes++ }},
	} {
		t.Run(fault.name, func(t *testing.T) {
			actual := declared
			fault.change(&actual)
			if err := validate(actual, logicalCPUs); err == nil {
				t.Fatal("altered real-owner budget/count/schema/vector accepted")
			}
		})
	}
}

func TestMixedV085PublicOwnerBudgetRejectsImpossibleHostsAndNativeAllocations(t *testing.T) {
	for _, logicalCPUs := range []int{0, 1, 2, 3, 4, 8} {
		t.Run(fmt.Sprintf("%d-logical-CPUs", logicalCPUs), func(t *testing.T) {
			for _, allocatedCPU := range []int{2, 4} {
				if logicalCPUs >= 3 && allocatedCPU == 2 {
					continue
				}
				for _, cpuCapacity := range []int{max(1, logicalCPUs-1), 8} {
					actual := coordination.ReservationTotals{
						SchemaVersion: 5, Mode: reservationMode, ActiveOwners: 2,
						Capacity:  coordination.ReservationVector{CPU: cpuCapacity, MemoryBytes: 3 * policy.GiB},
						Allocated: coordination.ReservationVector{CPU: allocatedCPU, MemoryBytes: 2 * policy.GiB},
					}
					if err := validateMixedV085PublicOwnerBudget(actual, logicalCPUs); err == nil {
						t.Fatal("impossible host, manufactured capacity or native allocation accepted as compatibility budget")
					}
				}
			}
		})
	}
}

func TestMixedV085PublicOwnerBudgetRejectsNativeMemoryCap(t *testing.T) {
	actual := coordination.ReservationTotals{
		SchemaVersion: 5, Mode: reservationMode, ActiveOwners: 2,
		Capacity:  coordination.ReservationVector{CPU: 2, MemoryBytes: 8 * policy.GiB},
		Allocated: coordination.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB},
	}
	if err := validateMixedV085PublicOwnerBudget(actual, 3); err == nil {
		t.Fatal("native eight-GiB cap accepted as compatibility three-GiB budget")
	}
}

// These are modeled host inputs, not a claim of execution on a hosted runner.
// Use the real configuration loader, catalog safety reserve and admission planner.
func TestMixedV085PublicOwnerBudgetDerivesStandardRAMCapacity(t *testing.T) {
	for _, variant := range []string{publicTUIMixedVariant, publicTUIMixedV085Variant} {
		t.Run(variant, func(t *testing.T) {
			directory := t.TempDir()
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
			capBytes := 8 * policy.GiB
			if variant == publicTUIMixedV085Variant {
				capBytes = 3 * policy.GiB
			}
			if configured.Coordination.MaxMemoryBytes != capBytes || configured.Coordination.MaxCPU != 8 || configured.Coordination.MaxActiveOwners != 2 {
				t.Fatalf("scoped mixed configuration changed exact limits: %+v", configured.Coordination)
			}
			if variant == publicTUIMixedVariant {
				return
			}
			mixedV085StandardRAMControls(t, configured)
		})
	}
}

func mixedV085StandardRAMControls(t *testing.T, configured configadapter.Result) {
	t.Helper()
	capBytes := 3 * policy.GiB
	for _, memory := range []int64{7_000_000_000, 7 * policy.GiB, 16 * policy.GiB} {
		for _, swap := range []string{"available", "unavailable"} {
			t.Run(fmt.Sprintf("%d-bytes-%s-swap", memory, swap), func(t *testing.T) {
				disk := 64 * policy.GiB
				sample := policy.Sample{PhysicalMemoryBytes: memory, EffectiveMemoryLimitBytes: memory, AvailableMemoryBytes: &memory, AvailableParallelism: 3, DiskFreeBytes: &disk, DiskTotalBytes: &disk, SwapState: swap}
				resolution, err := configured.Catalog.Resolve("balanced", policy.TaskEphemeral, sample)
				if err != nil {
					t.Fatal(err)
				}
				if resolution.Decision != policy.DecisionRun || resolution.ResolvedProfile != "balanced" || resolution.MemoryReserve <= 0 || memory-resolution.MemoryReserve < capBytes {
					t.Fatalf("real balanced safety reserve cannot admit scoped cap: %+v", resolution)
				}
				settings := coordination.ReservationPolicy{Enabled: true, MaxCPU: configured.Coordination.MaxCPU, MaxMemoryBytes: configured.Coordination.MaxMemoryBytes, MaxActiveOwners: configured.Coordination.MaxActiveOwners, OwnerShares: configured.Coordination.OwnerShares}
				plan, err := coordination.PlanReservation(sample, resolution, settings, 1, policy.GiB)
				if err != nil || plan.Capacity != (coordination.ReservationVector{CPU: 2, MemoryBytes: 3 * policy.GiB}) || plan.Requested != (coordination.ReservationVector{CPU: 1, MemoryBytes: policy.GiB}) || plan.Allocated != plan.Requested {
					t.Fatalf("standard RAM exact safe plan differs: %+v error=%v", plan, err)
				}
			})
		}
	}
}
