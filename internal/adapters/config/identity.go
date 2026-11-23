package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/identity"
)

// LoadIdentity reads optional identity input and applies validated overrides.
func LoadIdentity(path, sourceOverride string, tagOverrides []string) (identity.Value, error) {
	return loadIdentity(system.Production, path, sourceOverride, tagOverrides)
}

// loadIdentity reads optional identity input through the host's file port and applies validated overrides.
func loadIdentity(host system.System, path, sourceOverride string, tagOverrides []string) (identity.Value, error) {
	data, err := host.Files.ReadFile(filepath.Clean(path))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return identity.Value{}, err
	}
	return identity.Resolve(data, err == nil, sourceOverride, tagOverrides)
}

// IdentityPath resolves an explicit environment path, then discovers a worktree-local
// identity while walking upward. A machine default is the final fallback.
func IdentityPath(environment map[string]string, workingDirectory string) string {
	return identityPath(system.Production, environment, workingDirectory)
}

// identityPath applies IdentityPath to the host's working directory and file port.
func identityPath(host system.System, environment map[string]string, workingDirectory string) string {
	if path := environment["HIPPO_IDENTITY"]; path != "" {
		return path
	}
	if workingDirectory == "" {
		workingDirectory, _ = host.Environment.Getwd()
	}
	current, err := absolutePath(host, workingDirectory)
	if err == nil {
		for {
			candidate := filepath.Join(current, "hippo.identity.json")
			if _, statError := host.Files.Stat(candidate); statError == nil {
				return candidate
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	if path := environment["HIPPO_DEFAULT_IDENTITY"]; path != "" {
		return path
	}

	return filepath.Join(workingDirectory, "hippo.identity.json")
}
