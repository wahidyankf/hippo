package host

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultEvidenceRoot resolves private evidence storage from environment or user home.
func DefaultEvidenceRoot(environment map[string]string) string {
	if root := environment["RESOURCE_GUARD_ROOT"]; root != "" {
		return root
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "resource-guard")
	}

	if state := environment["XDG_STATE_HOME"]; state != "" {
		return filepath.Join(state, "resource-guard")
	}

	return filepath.Join(home, ".local", "state", "resource-guard")
}
