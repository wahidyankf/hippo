package config

import (
	"os"
	"path/filepath"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/identity"
)

// Provider implements validated application configuration and local path inputs.
type Provider struct{}

// Load resolves strict configuration precedence and decodes one catalog.
func (Provider) Load(path string, environment map[string]string) (application.Configuration, error) {
	resolved, explicit := Path(path, environment)
	return Load(resolved, explicit)
}

// Identity resolves a file then applies the operator's safe overrides.
func (Provider) Identity(environment map[string]string, workingDirectory, sourceOverride string, tags []string) (identity.Value, string, error) {
	path := IdentityPath(environment, workingDirectory)
	value, err := LoadIdentity(path, sourceOverride, tags)
	return value, path, err
}

// StateRoot resolves shared machine evidence storage.
func (Provider) StateRoot(environment map[string]string) string {
	return DefaultEvidenceRoot(environment)
}

// AbsolutePath normalizes a caller-provided working directory.
func (Provider) AbsolutePath(path string) (string, error) { return filepath.Abs(path) }

// IdentityPresent observes whether an identity file exists before optional-label resolution.
func (Provider) IdentityPresent(environment map[string]string, workingDirectory string) bool {
	_, err := os.Stat(IdentityPath(environment, workingDirectory))
	return err == nil
}

// DisplayPath formats a caller-facing identity path relative to the checkout.
func (Provider) DisplayPath(workingDirectory, path string) string {
	base, err := filepath.Abs(workingDirectory)
	if err != nil {
		return path
	}
	relative, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return relative
}

// PortLeaseRoot resolves private service-port identity storage from the invocation environment.
func PortLeaseRoot(environment map[string]string) string {
	temporary := environment["TMPDIR"]
	if temporary == "" {
		temporary = os.TempDir()
	}
	return filepath.Join(temporary, "hippo-port-leases")
}
