package unit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	resourceconfig "github.com/wahidyankf/hippo/internal/config"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/policy"
)

func adaptiveSample(memory, available, diskFree, diskTotal int64, swapState string) policy.Sample {
	level, cpu := 1, 20.0

	return policy.Sample{
		SchemaVersion:             3,
		MeasuredAt:                time.Unix(0, 0).UTC().Format(time.RFC3339Nano),
		Platform:                  "linux",
		Capabilities:              []string{"cgroup-v2", "memory-psi"},
		EffectiveMemoryLimitBytes: memory,
		PhysicalMemoryBytes:       memory,
		AvailableMemoryBytes:      &available,
		MemoryPressureLevel:       &level,
		AvailableParallelism:      4,
		CPUUtilizationPercent:     &cpu,
		DiskFreeBytes:             &diskFree,
		DiskTotalBytes:            &diskTotal,
		SwapState:                 swapState,
	}
}

func TestAdaptiveProfileSelectionAndThresholds(t *testing.T) { //nolint:cyclop // One table-like test proves every profile transition and terminal decision.
	catalog := policy.BuiltinCatalog()
	balanced, err := catalog.Resolve("balanced", "ephemeral", adaptiveSample(32*policy.GiB, 20*policy.GiB, 100*policy.GiB, 512*policy.GiB, "active"))
	if err != nil || balanced.ResolvedProfile != "balanced" || balanced.MemoryReserve != 4*policy.GiB || balanced.Concurrency != 3 {
		t.Fatalf("unexpected balanced resolution %+v error=%v", balanced, err)
	}

	if balanced.Policy.WarningAdmissionMemoryBytes != 8*policy.GiB ||
		balanced.Policy.SwapOutWarningBytes != 128*policy.MiB ||
		balanced.Policy.SwapOutCriticalBytes != 512*policy.MiB ||
		balanced.Policy.CompressorWarningPayloadBytes != 12*policy.GiB ||
		balanced.Policy.CompressorCriticalPayloadBytes != 16*policy.GiB {
		t.Fatalf("unexpected dynamic signal thresholds %+v", balanced.Policy)
	}

	constrained, err := catalog.Resolve("balanced", "ephemeral", adaptiveSample(5*policy.GiB, 800*policy.MiB, 12*policy.GiB, 14*policy.GiB, "unavailable"))
	if err != nil || constrained.ResolvedProfile != "constrained" || constrained.Concurrency != 2 {
		t.Fatalf("unexpected constrained resolution %+v error=%v", constrained, err)
	}

	minimal, err := catalog.Resolve("balanced", "ephemeral", adaptiveSample(policy.GiB, 200*policy.MiB, policy.GiB, 4*policy.GiB, "unavailable"))
	if err != nil || minimal.ResolvedProfile != "minimal" || minimal.Concurrency != 1 || minimal.ExitCode != 0 {
		t.Fatalf("unexpected minimal resolution %+v error=%v", minimal, err)
	}

	strict, err := catalog.Resolve("balanced", "transactional", adaptiveSample(5*policy.GiB, 700*policy.MiB, 12*policy.GiB, 14*policy.GiB, "unavailable"))
	if err != nil || strict.ExitCode != policy.ReplanRequiredExitCode || strict.Decision != "replan" {
		t.Fatalf("unexpected strict resolution %+v error=%v", strict, err)
	}

	cleanup, err := catalog.Resolve("balanced", "ephemeral", adaptiveSample(policy.GiB, 800*policy.MiB, 200*policy.MiB, policy.GiB, "unavailable"))
	if err != nil || cleanup.ExitCode != guard.StorageBlockedExitCode || cleanup.Decision != "cleanup" {
		t.Fatalf("unexpected cleanup resolution %+v error=%v", cleanup, err)
	}
	if _, err := catalog.Resolve("missing", "ephemeral", adaptiveSample(policy.GiB, policy.GiB, policy.GiB, policy.GiB, "idle")); err == nil {
		t.Fatal("unknown profile was accepted")
	}

	legacySample := adaptiveSample(32*policy.GiB, 20*policy.GiB, 100*policy.GiB, 512*policy.GiB, "active")
	legacySample.EffectiveMemoryLimitBytes = 0
	legacySample.DiskTotalBytes = nil
	defaultless := catalog
	defaultless.DefaultProfile = ""
	if resolved, err := defaultless.Resolve("", "ephemeral", legacySample); err != nil || resolved.ResolvedProfile != "balanced" {
		t.Fatalf("legacy capacity fallback failed: %+v %v", resolved, err)
	}

	terminal := catalog
	terminalProfile := terminal.Profiles["balanced"]
	terminalProfile.Fallback = ""
	terminal.Profiles["balanced"] = terminalProfile
	if _, err := terminal.Resolve("balanced", "ephemeral", adaptiveSample(policy.GiB, 1, policy.GiB, policy.GiB, "idle")); err == nil {
		t.Fatal("profile without a usable fallback was accepted")
	}

	terminalProfile.Fallback = "constrained"
	catalog.Profiles["balanced"] = terminalProfile
	cycle := catalog
	profile := cycle.Profiles["constrained"]
	profile.Fallback = "balanced"
	cycle.Profiles["constrained"] = profile
	busy := adaptiveSample(policy.GiB, 1, policy.GiB, policy.GiB, "idle")
	busy.CPUUtilizationPercent = new(100.0)

	if _, err := cycle.Resolve("balanced", "ephemeral", busy); err == nil {
		t.Fatal("fallback cycle was accepted")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "hippo.local.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestStrictLocalConfiguration(t *testing.T) { //nolint:cyclop,gocyclo // The strict table intentionally keeps all schema and precedence cases together.
	valid := writeConfig(t, `{"schemaVersion":1,"defaultProfile":"local","profiles":{"local":{"extends":"constrained","fallback":"minimal","strict":true,"maxConcurrency":1,"maxCpuUtilizationPercent":90}}}`)
	loaded, err := resourceconfig.Load(valid, true)
	if err != nil ||
		loaded.Hash == "" ||
		loaded.Source != "local" ||
		loaded.Catalog.DefaultProfile != "local" ||
		!loaded.Catalog.Profiles["local"].Strict {
		t.Fatalf("unexpected config result %+v error=%v", loaded, err)
	}

	defaults, err := resourceconfig.Load(writeConfig(t, `{"schemaVersion":1}`), true)
	if err != nil || defaults.Catalog.DefaultProfile != "balanced" || defaults.Coordination.Mode != "exclusive" {
		t.Fatalf("default local config failed: %+v %v", defaults, err)
	}
	reservations, err := resourceconfig.Load(writeConfig(t, `{"schemaVersion":2,"coordination":{"maxCpu":6,"maxMemoryMiB":4096,"maxActiveOwners":8,"automaticOwnerShares":{"balanced":4,"constrained":2,"minimal":1}}}`), true)
	if err != nil || reservations.Coordination.Mode != "reservation" || reservations.Coordination.MaxCPU != 6 ||
		reservations.Coordination.MaxMemoryBytes != 4*policy.GiB || reservations.Coordination.MaxActiveOwners != 8 {
		t.Fatalf("schema 2 reservation config failed: %+v %v", reservations, err)
	}
	reservationDefaults, err := resourceconfig.Load(writeConfig(t, `{"schemaVersion":2}`), true)
	if err != nil || reservationDefaults.Coordination.Mode != "reservation" || reservationDefaults.Coordination.MaxActiveOwners != 20 {
		t.Fatalf("schema 2 reservation defaults failed: %+v %v", reservationDefaults, err)
	}

	sharedParent := writeConfig(t, `{"schemaVersion":1,"profiles":{"one":{"extends":"constrained"},"two":{"extends":"constrained"}}}`)
	if _, err := resourceconfig.Load(sharedParent, true); err != nil {
		t.Fatalf("shared profile inheritance failed: %v", err)
	}
	if empty, err := resourceconfig.Load("", false); err != nil || empty.Catalog.DefaultProfile != "balanced" {
		t.Fatalf("empty config path failed: %+v %v", empty, err)
	}
	if fallback, err := resourceconfig.Load(filepath.Join(t.TempDir(), "missing.json"), false); err != nil || fallback.Catalog.DefaultProfile != "balanced" {
		t.Fatalf("optional config did not fall back: %+v %v", fallback, err)
	}
	if _, err := resourceconfig.Load(filepath.Join(t.TempDir(), "missing.json"), true); err == nil {
		t.Fatal("missing explicit config was accepted")
	}

	invalid := []string{
		``,
		`{"schemaVersion":`,
		`[`,
		`{"schemaVersion":1,]}`,
		`{"schemaVersion":1,"unknown":[{"value":]}`,
		`{"schemaVersion":3}`,
		`{"schemaVersion":1,"coordination":{"mode":"reservation"}}`,
		`{"schemaVersion":2,"coordination":{"mode":"exclusive"}}`,
		`{"schemaVersion":2,"coordination":{"maxCpu":-1}}`,
		`{"schemaVersion":2,"coordination":{"maxMemoryMiB":-1}}`,
		`{"schemaVersion":2,"coordination":{"maxActiveOwners":-1}}`,
		`{"schemaVersion":2,"coordination":{"maxMemoryMiB":255}}`,
		`{"schemaVersion":2,"coordination":{"maxActiveOwners":21}}`,
		`{"schemaVersion":2,"coordination":{"automaticOwnerShares":{"unknown":2}}}`,
		`{"schemaVersion":2,"coordination":{"automaticOwnerShares":{"balanced":0}}}`,
		`{"schemaVersion":2,"coordination":{"automaticOwnerShares":{"minimal":21}}}`,
		`{"schemaVersion":1,"schemaVersion":1}`,
		`{"schemaVersion":1,"unknown":true}`,
		`{"schemaVersion":1} {}`,
		`{"schemaVersion":1,"defaultProfile":"missing"}`,
		`{"schemaVersion":1,"profiles":{"local":{"maxConcurrency":1}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"memoryReserveMinMiB":64}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"memoryReserveMinMiB":9223372036854775807}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"memoryReserveMaxMiB":9223372036854775807}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"noSwapMemoryReserveMinMiB":9223372036854775807}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"noSwapMemoryReserveMaxMiB":9223372036854775807}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"diskReserveMinMiB":9223372036854775807}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"diskReserveMaxMiB":9223372036854775807}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"memoryReserveMinMiB":2048,"memoryReserveMaxMiB":1024}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"maxCpuUtilizationPercent":99}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"memoryReservePercent":0}}}`,
		`{"schemaVersion":1,"profiles":{"one":{"extends":"two"},"two":{"extends":"one"}}}`,
		`{"schemaVersion":1,"profiles":{"one":{"extends":"missing"}}}`,
		`{"schemaVersion":1,"profiles":{"balanced":{"fallback":"missing"}}}`,
		`{"schemaVersion":1,"profiles":{"constrained":{"fallback":"balanced"}}}`,
	}
	for _, content := range invalid {
		if _, err := resourceconfig.Load(writeConfig(t, content), true); err == nil {
			t.Fatalf("invalid config accepted: %s", content)
		}
	}

	if path, explicit := resourceconfig.Path("cli.json", map[string]string{"HIPPO_CONFIG": "env.json"}); path != "cli.json" || !explicit {
		t.Fatalf("CLI precedence failed: %q %v", path, explicit)
	}
	if path, explicit := resourceconfig.Path("", map[string]string{"HIPPO_CONFIG": "env.json"}); path != "env.json" || !explicit {
		t.Fatalf("environment precedence failed: %q %v", path, explicit)
	}
	if path, explicit := resourceconfig.Path("", map[string]string{"HIPPO_DEFAULT_CONFIG": "default.json"}); path != "default.json" || explicit {
		t.Fatalf("default path failed: %q %v", path, explicit)
	}
	if path, explicit := resourceconfig.Path("", map[string]string{}); path != "hippo.local.json" || explicit {
		t.Fatalf("repository-local path failed: %q %v", path, explicit)
	}
}

func TestLinuxParsers(t *testing.T) { //nolint:cyclop,gocognit,gocyclo // Colocated parser cases share canonical Linux fixture vocabulary.
	memory, err := host.ParseMemInfo("ignored\nMemTotal: 16777216 kB\nMemAvailable: 8388608 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n")
	if err != nil || memory.Total != 16*policy.GiB || host.EffectiveMemoryLimit(memory.Total, 4*policy.GiB, 6*policy.GiB) != 4*policy.GiB {
		t.Fatalf("unexpected Linux memory %+v error=%v", memory, err)
	}

	for _, invalid := range []string{"", "MemTotal: bad kB\nMemAvailable: 1 kB\n", "MemTotal: 0 kB\nMemAvailable: 0 kB\n", "MemTotal: 1 kB\n"} {
		if _, err := host.ParseMemInfo(invalid); err == nil {
			t.Fatalf("invalid meminfo accepted: %q", invalid)
		}
	}

	state, err := host.ParseProcStat("intr 1\ncpu 10 20 30 40 50\ncpu0 1 2 3 4\n")
	if err != nil || len(state) != 5 || state[3] != 40 {
		t.Fatalf("unexpected CPU state %v error=%v", state, err)
	}

	for _, invalid := range []string{"", "cpu a 2 3 4"} {
		if _, err := host.ParseProcStat(invalid); err == nil {
			t.Fatalf("invalid proc stat accepted: %q", invalid)
		}
	}

	psi, err := host.ParsePSI("some avg10=10.00 avg60=1.00 total=1\nfull avg10=5.00 avg60=1.00 total=1\n")
	if err != nil || psi.SomeAvg10 != 10 || psi.FullAvg10 != 5 {
		t.Fatalf("unexpected PSI %+v error=%v", psi, err)
	}

	for _, invalid := range []string{"", "some total=1", "some avg10=bad", "some avg10=101"} {
		if _, err := host.ParsePSI(invalid); err == nil {
			t.Fatalf("invalid PSI accepted: %q", invalid)
		}
	}
	if psi, err := host.ParsePSI("some avg10=1 total=1\n"); err != nil || psi.FullAvg10 != 0 {
		t.Fatalf("optional PSI full line failed: %+v %v", psi, err)
	}
	if _, err := host.ParsePSI("some avg10=1\nfull avg10=bad\n"); err == nil {
		t.Fatal("invalid full PSI was accepted")
	}

	events := host.ParseMemoryEvents("low 1\noom 2\nbad\noom_kill nope\n")
	if events["oom"] != 2 || len(events) != 2 {
		t.Fatalf("unexpected events %+v", events)
	}

	if units, finite := host.ParseCPUMax("150000 100000"); !finite || units != 1 {
		t.Fatalf("unexpected CPU quota %d %v", units, finite)
	}
	for _, unlimited := range []string{"max 100000", "bad", "0 100000"} {
		if _, finite := host.ParseCPUMax(unlimited); finite {
			t.Fatalf("invalid CPU quota accepted: %q", unlimited)
		}
	}

	if value, finite := host.ParseCgroupLimit([]byte("4096\n")); !finite || value != 4096 {
		t.Fatalf("finite cgroup limit failed: %d %v", value, finite)
	}
	for _, unlimited := range [][]byte{nil, []byte("max"), []byte("-1"), []byte("bad")} {
		if _, finite := host.ParseCgroupLimit(unlimited); finite {
			t.Fatalf("invalid cgroup limit accepted: %q", unlimited)
		}
	}

	if root := host.CgroupRoot("0::/actions/job\n"); root != "/sys/fs/cgroup/actions/job" {
		t.Fatalf("unexpected cgroup root %q", root)
	}
	if root := host.CgroupRoot("1:name:/legacy\n"); root != "/sys/fs/cgroup" {
		t.Fatalf("unexpected default cgroup root %q", root)
	}
	if swapIn, swapOut := host.ParseLinuxVMStat("ignored\npswpin 3\npswpout 4\npswpout bad\n"); swapIn != 3 || swapOut != 4 {
		t.Fatalf("unexpected vmstat %d %d", swapIn, swapOut)
	}
}

func TestNoSwapPSIAndOOMAssessment(t *testing.T) {
	base := adaptiveSample(4*policy.GiB, 3*policy.GiB, 10*policy.GiB, 20*policy.GiB, "unavailable")
	resolution, err := policy.BuiltinCatalog().Resolve("balanced", "ephemeral", base)
	if err != nil {
		t.Fatal(err)
	}

	if assessment := policy.ResourceAssessment([]policy.Sample{base}, resolution.Policy); assessment.State != "normal" ||
		assessment.SwapState != "unavailable" {
		t.Fatalf("no-swap host rejected: %+v", assessment)
	}
	psi := base
	psi.MemoryPSISomeAvg10 = new(10.0)

	if assessment := policy.ResourceAssessment([]policy.Sample{psi}, resolution.Policy); assessment.State != "warning" ||
		assessment.Reason != "memory-psi" {
		t.Fatalf("PSI warning missed: %+v", assessment)
	}
	first, second := base, base
	first.OOMEvents, second.OOMEvents = new(int64(1)), new(int64(2))

	if assessment := policy.ResourceAssessment([]policy.Sample{first, second}, resolution.Policy); assessment.State != "critical" ||
		assessment.Reason != "memory-oom" {
		t.Fatalf("OOM delta missed: %+v", assessment)
	}
	if strings.TrimSpace(resolution.ResolvedProfile) == "" {
		t.Fatal("resolution profile is empty")
	}
}
