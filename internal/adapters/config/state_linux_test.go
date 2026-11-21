//go:build linux

package config_test

import (
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/config"
)

// The default state root on Linux is XDG_STATE_HOME, else the local state
// directory under HOME, and HIPPO_ROOT overrides both. The corpus cannot bind
// the Linux rule on a macOS host, whose default is its Application Support
// directory, so this Linux-only test is the proof for HI-ENV-002.
func TestDefaultEvidenceRootFollowsLinuxStateHome(t *testing.T) {
	t.Setenv("HOME", "/tmp/hippo-test-home")
	tests := []struct {
		name        string
		environment map[string]string
		want        string
	}{
		{
			name:        "XDG_STATE_HOME names the root",
			environment: map[string]string{"XDG_STATE_HOME": "/state/xdg"},
			want:        "/state/xdg/hippo",
		},
		{
			name:        "HOME names the default when XDG_STATE_HOME is unset",
			environment: map[string]string{},
			want:        "/tmp/hippo-test-home/.local/state/hippo",
		},
		{
			name:        "HIPPO_ROOT overrides XDG_STATE_HOME",
			environment: map[string]string{"HIPPO_ROOT": "/root/hippo", "XDG_STATE_HOME": "/state/xdg"},
			want:        "/root/hippo",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := config.DefaultEvidenceRoot(test.environment); got != test.want {
				t.Fatalf("DefaultEvidenceRoot = %q, want %q", got, test.want)
			}
		})
	}
}
