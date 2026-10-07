package cli //nolint:testpackage // The default collector is resolved inside the CLI boundary.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/host"
)

// The literals below are the contract: the identity the repository's own
// test builds stamp, and the variable the end-to-end fixtures set. Only a
// build carrying that identity may read Linux evidence from anywhere but the
// live host.
func TestOnlyATestBuildReadsFixedLinuxEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proc"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "proc", "stat"), []byte("fixed-evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, version, root string
		redirected          bool
	}{
		{name: "test build", version: "v0.0.0-test", root: root, redirected: true},
		{name: "release build", version: "v0.8.2", root: root},
		{name: "plain build", version: "dev", root: root},
		{name: "relative root", version: "v0.0.0-test", root: "relative/evidence"},
		{name: "empty root", version: "v0.0.0-test"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			environment := []string{"PATH=/usr/bin:/bin", "HIPPO_TEST_LINUX_EVIDENCE_ROOT=" + testCase.root}
			application := wiredApplication(Application{Version: testCase.version, Environment: environment}).defaults()
			collector, isSystem := application.Collector.(host.SystemCollector)
			if !isSystem {
				t.Fatalf("default collector is %T, want host.SystemCollector", application.Collector)
			}
			if !testCase.redirected {
				if collector.ReadFile != nil {
					t.Fatal("a build that is not the test build read Linux evidence from a fixture root")
				}

				return
			}
			if collector.ReadFile == nil {
				t.Fatal("the test build ignored its Linux evidence root")
			}
			if data, err := collector.ReadFile("/proc/stat"); err != nil || string(data) != "fixed-evidence" {
				t.Fatalf("test build read %q error=%v, want the fixture evidence", data, err)
			}
		})
	}
}

// defaultCollector samples the live host, except in a test build given an
// absolute Linux evidence root.
func defaultCollector(version string, environment []string) host.SystemCollector {
	root := environmentMap(environment)[linuxEvidenceRootEnvironment]
	if version != testBuildVersion || !filepath.IsAbs(root) {
		return host.SystemCollector{}
	}

	return host.SystemCollector{ReadFile: host.RootedFileReader(root)}
}
