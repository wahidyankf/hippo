package bootstrap

import (
	"context"
	"errors"
	"testing"
)

func TestDefaultCollectorRootRequiresTestBuildAndAbsolutePath(t *testing.T) {
	for _, row := range []struct {
		name, version, root string
		enabled             bool
	}{
		{"development", "dev", "/fixture", false},
		{"release", "v0.9.0", "/fixture", false},
		{"relative test root", "v0.0.0-test", "fixture", false},
		{"empty test root", "v0.0.0-test", "", false},
		{"absolute test root", "v0.0.0-test", "/fixture", true},
	} {
		t.Run(row.name, func(t *testing.T) {
			collector := defaultCollector(row.version, []string{"HIPPO_TEST_LINUX_EVIDENCE_ROOT=" + row.root})
			if collector.ReadFile != nil || (collector.ReadFileContext != nil) != row.enabled {
				t.Fatalf("root gating: legacy=%t context=%t", collector.ReadFile != nil, collector.ReadFileContext != nil)
			}
			if collector.ReadFileContext != nil {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				data, err := collector.ReadFileContext(ctx, "/proc/meminfo")
				if data != nil || !errors.Is(err, context.Canceled) {
					t.Fatalf("root reader lost cancellation: %q, %v", data, err)
				}
			}
		})
	}
}
