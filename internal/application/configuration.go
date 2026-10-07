package application

import (
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
)

// Configuration is a normalized validated catalog and coordination policy.
type Configuration struct {
	Catalog      policy.Catalog
	Coordination coordination.Configuration
	Hash         string
	Source       string
}

// ConfigurationProvider loads strict configuration and resolves safe identity at its IO boundary.
type ConfigurationProvider interface {
	Load(path string, environment map[string]string) (Configuration, error)
	Identity(environment map[string]string, workingDirectory, sourceOverride string, tags []string) (identity.Value, string, error)
	StateRoot(environment map[string]string) string
	AbsolutePath(path string) (string, error)
}
