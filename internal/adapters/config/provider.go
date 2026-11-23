package config

import (
	"path/filepath"

	"github.com/wahidyankf/hippo/internal/adapters/system"
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
func (Provider) AbsolutePath(path string) (string, error) {
	return absolutePath(system.Production, path)
}

// IdentityPresent observes whether an identity file exists before optional-label resolution.
func (Provider) IdentityPresent(environment map[string]string, workingDirectory string) bool {
	return identityPresent(system.Production, environment, workingDirectory)
}

// identityPresent applies IdentityPresent through the host's file port.
func identityPresent(host system.System, environment map[string]string, workingDirectory string) bool {
	_, err := host.Files.Stat(identityPath(host, environment, workingDirectory))
	return err == nil
}

// DisplayPath formats a caller-facing identity path relative to the checkout.
func (Provider) DisplayPath(workingDirectory, path string) string {
	return displayPath(system.Production, workingDirectory, path)
}

// displayPath applies DisplayPath against the host's working directory.
func displayPath(host system.System, workingDirectory, path string) string {
	base, err := absolutePath(host, workingDirectory)
	if err != nil {
		return path
	}
	relative, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return relative
}

// absolutePath resolves a path against the host's working directory, as filepath.Abs does.
func absolutePath(host system.System, path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	working, err := host.Environment.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(working, path), nil
}

// PortLeaseRoot resolves private service-port identity storage from the invocation environment.
func PortLeaseRoot(environment map[string]string) string {
	return portLeaseRoot(system.Production, environment)
}

// portLeaseRoot applies PortLeaseRoot with the host's temporary directory as the fallback.
func portLeaseRoot(host system.System, environment map[string]string) string {
	temporary := environment["TMPDIR"]
	if temporary == "" {
		temporary = host.Environment.TempDir()
	}
	return filepath.Join(temporary, "hippo-port-leases")
}
