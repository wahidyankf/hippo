package cli //nolint:testpackage // Application dependencies are injected at the CLI boundary.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/status"
)

func TestReleaseMonitorReportsMissingOrInvalidFlagsAsUsageErrors(t *testing.T) {
	// A flag the caller left out or gave an unusable value is a usage mistake,
	// not HIPPO failing to supervise anything: it exits 2 naming
	// hippo.args.invalid before any sample is taken.
	cases := []struct {
		name      string
		arguments []string
	}{
		{name: "missing health URL", arguments: []string{"--routed-origin", "https://example.test"}},
		{name: "missing routed origin", arguments: []string{"--health-url", "http://127.0.0.1:9/health"}},
		{name: "malformed health URL", arguments: []string{"--health-url", "ftp://example.test", "--routed-origin", "https://example.test"}},
		{name: "missing deployment root", arguments: []string{"--deployment-root", "", "--health-url", "http://127.0.0.1:9/health", "--routed-origin", "https://example.test"}},
		{name: "negative duration", arguments: []string{"--duration-ms", "-1", "--health-url", "http://127.0.0.1:9/health", "--routed-origin", "https://example.test"}},
		{name: "service port out of range", arguments: []string{"--service-port", "70000", "--health-url", "http://127.0.0.1:9/health", "--routed-origin", "https://example.test"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			stderr := &bytes.Buffer{}
			application := Application{
				Stdout: &bytes.Buffer{}, Stderr: stderr,
				Environment: []string{"HIPPO_ROOT=" + root},
				Collector:   stableCollector{},
			}
			arguments := append([]string{
				"release", "monitor", "--output", root + "/raw.jsonl", "--summary", root + "/summary.json",
				"--deployment-root", root,
			}, testCase.arguments...)
			code, _ := application.Run(context.Background(), arguments)
			if code != status.CallerError {
				t.Fatalf("exit %d, want %d: %q", code, status.CallerError, stderr.String())
			}
			if !strings.Contains(stderr.String(), "hippo: ["+string(status.CodeArgsInvalid)+"]") {
				t.Fatalf("the refusal does not name %s: %q", status.CodeArgsInvalid, stderr.String())
			}
		})
	}
}
