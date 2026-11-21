package support

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/tests/contract"
)

// stateHomeVariable is the variable the Linux default state root reads first.
const (
	stateHomeVariable = "XDG_STATE_HOME"
	namedStateHome    = "the XDG_STATE_HOME variable names a state directory"
	unsetStateHome    = "the XDG_STATE_HOME variable is unset"
)

func (driver *Driver) stateRootAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^(the XDG_STATE_HOME variable names a state directory|the XDG_STATE_HOME variable is unset)$`, driver.stageStateHome),
		step(`^the default state root is resolved with HIPPO_ROOT unset$`, driver.resolveDefaultStateRoot),
		step(`^the root is the hippo directory the host platform names for that state$`, driver.requireDefaultStateRoot),
	}
}

// stageStateHome records the environment the default root is resolved from. It
// never names HIPPO_ROOT, so the override cannot decide the answer.
func (driver *Driver) stageStateHome(condition string) error {
	environment := map[string]string{}
	if condition == namedStateHome {
		base, err := driver.temporaryRoot()
		if err != nil {
			return err
		}
		environment[stateHomeVariable] = filepath.Join(base, "state")
	}
	driver.addCLI.stateEnvironment = environment

	return nil
}

func (driver *Driver) resolveDefaultStateRoot() error {
	if driver.addCLI.stateEnvironment == nil {
		return fmt.Errorf("no state environment was staged")
	}
	driver.addCLI.stateRoot = config.DefaultEvidenceRoot(driver.addCLI.stateEnvironment)

	return nil
}

// requireDefaultStateRoot holds the resolved root to the location the host platform
// names. macOS keeps its evidence under Library, and every other platform uses
// XDG_STATE_HOME when it is set, else the local state directory under HOME.
func (driver *Driver) requireDefaultStateRoot() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	var want string
	switch {
	case runtime.GOOS == "darwin":
		want = filepath.Join(home, "Library", "Application Support", "hippo")
	case driver.addCLI.stateEnvironment[stateHomeVariable] != "":
		want = filepath.Join(driver.addCLI.stateEnvironment[stateHomeVariable], "hippo")
	default:
		want = filepath.Join(home, ".local", "state", "hippo")
	}
	if driver.addCLI.stateRoot != want {
		return fmt.Errorf("default state root is %q, want %q", driver.addCLI.stateRoot, want)
	}

	return nil
}
