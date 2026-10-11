package support

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// Reuse the existing recording archive controls. These run the actual new
// preparation script, proving routing/refusal rather than official provenance.
func TestBDDGateV085PreparationChecksHostsAndIndependentProvenance(t *testing.T) {
	for _, row := range []struct {
		name, system, architecture, failure string
		want                                int
	}{
		{"Darwin arm64", "Darwin", "arm64", "", 0},
		{"Darwin amd64", "Darwin", "x86_64", "", 0},
		{"Linux arm64", "Linux", "aarch64", "", 0},
		{"Linux amd64", "Linux", "x86_64", "", 0},
		{"wrong archive hash", "Darwin", "arm64", "hash", 1},
		{"wrong archive members", "Darwin", "arm64", "members", 1},
		{"symlink executable", "Darwin", "arm64", "symlink", 1},
		{"original old receipt rejected", "Darwin", "arm64", "old-receipt", 1},
		{"stderr rejects identity", "Darwin", "arm64", "stderr", 1},
		{"parser failure retained", "Darwin", "arm64", "parser", 73},
		{"download failure retained", "Darwin", "arm64", "download", 73},
		{"version failure retained", "Darwin", "arm64", "version-exit", 73},
		{"invalid immutable pin", "Darwin", "arm64", "lock", 1},
		{"offline archive prepared", "Darwin", "arm64", "offline", 0},
		{"offline foreign archive", "Darwin", "arm64", "offline-hash", 1},
		{"offline wrong receipt", "Darwin", "arm64", "offline-identity", 1},
		{"offline symlink rejected", "Darwin", "arm64", "offline-symlink", 1},
	} {
		t.Run(row.name, func(t *testing.T) {
			code, output, directory := runBDDGateReleaseFixture(t, row.system, row.architecture, row.failure, true)
			if code != row.want {
				t.Fatalf("v0.8.5 preparation exit=%d want=%d output=%s", code, row.want, output)
			}
			if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("caller retained owned v0.8.5 artifacts: %v", err)
			}
			if row.want == 0 && (!strings.Contains(string(output), "BOUND_V085=") || !strings.Contains(string(output), `"version":"v0.8.5"`)) {
				t.Fatalf("v0.8.5 preparation accepted inherited artifact inputs: %s", output)
			}
		})
	}
}
