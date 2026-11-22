package config

import (
	"path/filepath"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

// DefaultEvidenceRoot resolves private evidence storage from environment or user home.
func DefaultEvidenceRoot(environment map[string]string) string {
	return evidenceRoot(system.Production, environment)
}

// evidenceRoot applies DefaultEvidenceRoot to the host's home directory and platform.
func evidenceRoot(host system.System, environment map[string]string) string {
	if root := environment["HIPPO_ROOT"]; root != "" {
		return root
	}

	home, err := host.Environment.UserHomeDir()
	if err != nil {
		return ""
	}

	if host.Environment.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "hippo")
	}

	if state := environment["XDG_STATE_HOME"]; state != "" {
		return filepath.Join(state, "hippo")
	}

	return filepath.Join(home, ".local", "state", "hippo")
}
