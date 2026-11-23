package config_test

import (
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestLoadReadsTheConfigurationThroughTheFileDouble(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["/cfg/hippo.local.json"] = []byte(`{"schemaVersion":1}`)

	loaded, err := config.LoadWithHost(host.Port(), "/cfg/hippo.local.json", true)
	if err != nil {
		t.Fatalf("LoadWithHost() error = %v", err)
	}
	if loaded.Coordination.SchemaVersion != 1 || loaded.Source != "local" {
		t.Fatalf("LoadWithHost() = schema %d from %q, want schema 1 from local", loaded.Coordination.SchemaVersion, loaded.Source)
	}
}

func TestLoadRefusesAnUnknownSchemaFromTheFileDouble(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["/cfg/hippo.local.json"] = []byte(`{"schemaVersion":4}`)

	if _, err := config.LoadWithHost(host.Port(), "/cfg/hippo.local.json", true); err == nil {
		t.Fatal("LoadWithHost() accepted schema 4, want a refusal")
	}
}

func TestLoadRefusesAFileTheHostCannotReadEvenWhenOptional(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	port := host.Port()
	port.Files.ReadFile = func(string) ([]byte, error) { return nil, fs.ErrPermission }

	if _, err := config.LoadWithHost(port, "/cfg/hippo.local.json", false); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("LoadWithHost() error = %v, want the permission failure", err)
	}
}

func TestLoadIdentityRefusesAFileTheHostCannotRead(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	port := host.Port()
	port.Files.ReadFile = func(string) ([]byte, error) { return nil, fs.ErrPermission }

	if _, err := config.LoadIdentityWithHost(port, "/work/project/hippo.identity.json", "", nil); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("LoadIdentityWithHost() error = %v, want the permission failure", err)
	}
}

func TestIdentityPathFindsTheCheckoutFileThroughTheFileDouble(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["/work/project/hippo.identity.json"] = []byte("{}")

	if got := config.IdentityPathWithHost(host.Port(), map[string]string{}, "/work/project/cmd"); got != "/work/project/hippo.identity.json" {
		t.Fatalf("IdentityPathWithHost() = %q, want the checkout file", got)
	}
}

func TestIdentityPathFallsBackToTheMachineDefaultWhenNoCheckoutHoldsOne(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	environment := map[string]string{"HIPPO_DEFAULT_IDENTITY": "/machine/hippo.identity.json"}

	if got := config.IdentityPathWithHost(host.Port(), environment, "/work/project"); got != "/machine/hippo.identity.json" {
		t.Fatalf("IdentityPathWithHost() = %q, want the machine default", got)
	}
}

func TestIdentityPresentReadsTheFileDouble(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	if config.IdentityPresentWithHost(host.Port(), map[string]string{}, "/work/project") {
		t.Fatal("IdentityPresentWithHost() = true with no identity file, want false")
	}

	host.Files.Contents["/work/project/hippo.identity.json"] = []byte("{}")
	if !config.IdentityPresentWithHost(host.Port(), map[string]string{}, "/work/project") {
		t.Fatal("IdentityPresentWithHost() = false with an identity file, want true")
	}
}

func TestAbsolutePathJoinsARelativePathToTheHostWorkingDirectory(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Environment.Working = "/work"

	got, err := config.AbsolutePathWithHost(host.Port(), "project/../checkout")
	if err != nil || got != "/work/checkout" {
		t.Fatalf("AbsolutePathWithHost() = %q, %v, want /work/checkout", got, err)
	}
}

func TestDisplayPathReturnsThePathWhenItCannotBeMadeRelative(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	const path = "relative/hippo.identity.json"

	if got := config.DisplayPathWithHost(host.Port(), "/work/project", path); got != path {
		t.Fatalf("DisplayPathWithHost() = %q, want the path unchanged", got)
	}
}

func TestDisplayPathReturnsThePathWhenTheWorkingDirectoryIsUnknown(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	port := host.Port()
	port.Environment.Getwd = func() (string, error) { return "", errors.New("working directory unavailable") }
	const path = "/work/project/hippo.identity.json"

	if got := config.DisplayPathWithHost(port, "checkout", path); got != path {
		t.Fatalf("DisplayPathWithHost() = %q, want the path unchanged", got)
	}
}

func TestPortLeaseRootFallsBackToTheHostTemporaryDirectory(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Environment.Temp = "/host/tmp"

	if got := config.PortLeaseRootWithHost(host.Port(), map[string]string{}); got != "/host/tmp/hippo-port-leases" {
		t.Fatalf("PortLeaseRootWithHost() = %q, want the host temporary directory", got)
	}
}

func TestEvidenceRootIsEmptyWhenTheHostHasNoHomeDirectory(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})

	if got := config.EvidenceRootWithHost(host.Port(), map[string]string{}); got != "" {
		t.Fatalf("EvidenceRootWithHost() = %q, want empty without a home directory", got)
	}
}

func TestEvidenceRootUsesApplicationSupportOnDarwin(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Environment.Home = "/tmp/hippo-test-home"
	host.Environment.Platform = "darwin"

	want := "/tmp/hippo-test-home/Library/Application Support/hippo"
	if got := config.EvidenceRootWithHost(host.Port(), map[string]string{"XDG_STATE_HOME": "/state"}); got != want {
		t.Fatalf("EvidenceRootWithHost() = %q, want %q", got, want)
	}
}

func TestBuildCoordinationRefusesAnUnknownSchema(t *testing.T) {
	t.Parallel()

	if _, err := config.BuildCoordination(config.File{SchemaVersion: 4}, policy.BuiltinCatalog()); err == nil {
		t.Fatal("BuildCoordination() accepted schema 4, want a refusal")
	}
}

func TestBuildCoordinationRefusesAProfileWithNoOwnerShares(t *testing.T) {
	t.Parallel()

	catalog := policy.BuiltinCatalog()
	catalog.Profiles["unshared"] = policy.Profile{}

	if _, err := config.BuildCoordination(config.File{SchemaVersion: 2}, catalog); err == nil {
		t.Fatal("BuildCoordination() gave a profile with no owner shares, want a refusal")
	}
}

func TestBuildCoordinationRefusesAnOwnerShareInheritanceCycle(t *testing.T) {
	t.Parallel()

	catalog := policy.BuiltinCatalog()
	catalog.Profiles["alpha"] = policy.Profile{}
	catalog.Profiles["beta"] = policy.Profile{}
	decoded := config.File{SchemaVersion: 2, Profiles: map[policy.ProfileName]config.ProfileOverride{
		"alpha": {Extends: "beta"},
		"beta":  {Extends: "alpha"},
	}}

	if _, err := config.BuildCoordination(decoded, catalog); err == nil {
		t.Fatal("BuildCoordination() accepted an inheritance cycle, want a refusal")
	}
}
