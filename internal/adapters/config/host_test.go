package config

import (
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestLoadReadsTheConfigurationThroughTheFileDouble(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["/cfg/hippo.local.json"] = []byte(`{"schemaVersion":1}`)

	loaded, err := load(host.Port(), "/cfg/hippo.local.json", true)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if loaded.Coordination.SchemaVersion != 1 || loaded.Source != "local" {
		t.Fatalf("load() = schema %d from %q, want schema 1 from local", loaded.Coordination.SchemaVersion, loaded.Source)
	}
}

func TestLoadRefusesAnUnknownSchemaFromTheFileDouble(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["/cfg/hippo.local.json"] = []byte(`{"schemaVersion":4}`)

	if _, err := load(host.Port(), "/cfg/hippo.local.json", true); err == nil {
		t.Fatal("load() accepted schema 4, want a refusal")
	}
}

func TestLoadRefusesAFileTheHostCannotReadEvenWhenOptional(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	port := host.Port()
	port.Files.ReadFile = func(string) ([]byte, error) { return nil, fs.ErrPermission }

	if _, err := load(port, "/cfg/hippo.local.json", false); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("load() error = %v, want the permission failure", err)
	}
}

func TestLoadIdentityRefusesAFileTheHostCannotRead(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	port := host.Port()
	port.Files.ReadFile = func(string) ([]byte, error) { return nil, fs.ErrPermission }

	if _, err := loadIdentity(port, "/work/project/hippo.identity.json", "", nil); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("loadIdentity() error = %v, want the permission failure", err)
	}
}

func TestIdentityPathFindsTheCheckoutFileThroughTheFileDouble(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["/work/project/hippo.identity.json"] = []byte("{}")

	if got := identityPath(host.Port(), map[string]string{}, "/work/project/cmd"); got != "/work/project/hippo.identity.json" {
		t.Fatalf("identityPath() = %q, want the checkout file", got)
	}
}

func TestIdentityPathFallsBackToTheMachineDefaultWhenNoCheckoutHoldsOne(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	environment := map[string]string{"HIPPO_DEFAULT_IDENTITY": "/machine/hippo.identity.json"}

	if got := identityPath(host.Port(), environment, "/work/project"); got != "/machine/hippo.identity.json" {
		t.Fatalf("identityPath() = %q, want the machine default", got)
	}
}

func TestIdentityPresentReadsTheFileDouble(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	if identityPresent(host.Port(), map[string]string{}, "/work/project") {
		t.Fatal("identityPresent() = true with no identity file, want false")
	}

	host.Files.Contents["/work/project/hippo.identity.json"] = []byte("{}")
	if !identityPresent(host.Port(), map[string]string{}, "/work/project") {
		t.Fatal("identityPresent() = false with an identity file, want true")
	}
}

func TestAbsolutePathJoinsARelativePathToTheHostWorkingDirectory(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Environment.Working = "/work"

	got, err := absolutePath(host.Port(), "project/../checkout")
	if err != nil || got != "/work/checkout" {
		t.Fatalf("absolutePath() = %q, %v, want /work/checkout", got, err)
	}
}

func TestDisplayPathReturnsThePathWhenItCannotBeMadeRelative(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	const path = "relative/hippo.identity.json"

	if got := displayPath(host.Port(), "/work/project", path); got != path {
		t.Fatalf("displayPath() = %q, want the path unchanged", got)
	}
}

func TestDisplayPathReturnsThePathWhenTheWorkingDirectoryIsUnknown(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	port := host.Port()
	port.Environment.Getwd = func() (string, error) { return "", errors.New("working directory unavailable") }
	const path = "/work/project/hippo.identity.json"

	if got := displayPath(port, "checkout", path); got != path {
		t.Fatalf("displayPath() = %q, want the path unchanged", got)
	}
}

func TestPortLeaseRootFallsBackToTheHostTemporaryDirectory(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Environment.Temp = "/host/tmp"

	if got := portLeaseRoot(host.Port(), map[string]string{}); got != "/host/tmp/hippo-port-leases" {
		t.Fatalf("portLeaseRoot() = %q, want the host temporary directory", got)
	}
}

func TestEvidenceRootIsEmptyWhenTheHostHasNoHomeDirectory(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})

	if got := evidenceRoot(host.Port(), map[string]string{}); got != "" {
		t.Fatalf("evidenceRoot() = %q, want empty without a home directory", got)
	}
}

func TestEvidenceRootUsesApplicationSupportOnDarwin(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Environment.Home = "/tmp/hippo-test-home"
	host.Environment.Platform = "darwin"

	want := "/tmp/hippo-test-home/Library/Application Support/hippo"
	if got := evidenceRoot(host.Port(), map[string]string{"XDG_STATE_HOME": "/state"}); got != want {
		t.Fatalf("evidenceRoot() = %q, want %q", got, want)
	}
}

func TestBuildCoordinationRefusesAnUnknownSchema(t *testing.T) {
	t.Parallel()

	if _, err := buildCoordination(file{SchemaVersion: 4}, policy.BuiltinCatalog()); err == nil {
		t.Fatal("buildCoordination() accepted schema 4, want a refusal")
	}
}

func TestBuildCoordinationRefusesAProfileWithNoOwnerShares(t *testing.T) {
	t.Parallel()

	catalog := policy.BuiltinCatalog()
	catalog.Profiles["unshared"] = policy.Profile{}

	if _, err := buildCoordination(file{SchemaVersion: 2}, catalog); err == nil {
		t.Fatal("buildCoordination() gave a profile with no owner shares, want a refusal")
	}
}

func TestBuildCoordinationRefusesAnOwnerShareInheritanceCycle(t *testing.T) {
	t.Parallel()

	catalog := policy.BuiltinCatalog()
	catalog.Profiles["alpha"] = policy.Profile{}
	catalog.Profiles["beta"] = policy.Profile{}
	decoded := file{SchemaVersion: 2, Profiles: map[policy.ProfileName]profileOverride{
		"alpha": {Extends: "beta"},
		"beta":  {Extends: "alpha"},
	}}

	if _, err := buildCoordination(decoded, catalog); err == nil {
		t.Fatal("buildCoordination() accepted an inheritance cycle, want a refusal")
	}
}
