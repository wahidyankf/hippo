package support

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wahidyankf/hippo/tests/contract"
)

// bootstrapCacheVariable names the directory the ./hippo bootstrap caches its binary in.
const bootstrapCacheVariable = "HIPPO_BUILD_CACHE"

// bootstrapRuns is what the bootstrap scenario records across its two runs: the
// cache it uses, the directory holding its go shim, the shim's build log, and,
// per run, the exit status, the cumulative build count, and the output.
type bootstrapRuns struct {
	cache    string
	shim     string
	buildLog string
	exits    []int
	compiles []int
	outputs  []string
}

func (driver *Driver) bootstrapAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^an empty build cache named by HIPPO_BUILD_CACHE$`, driver.emptyBuildCache),
		step(`^the bootstrap is run twice in a row$`, driver.runBootstrapTwice),
		step(`^both runs exit 0 and only the first run compiles the binary$`, driver.requireOnlyFirstBootstrapCompiles),
	}
}

// emptyBuildCache makes an empty cache and a go command on the PATH that logs
// each build the bootstrap starts and then runs the real toolchain, so the
// scenario can count the compiles the bootstrap really performs.
func (driver *Driver) emptyBuildCache() error {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return err
	}
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	runs := bootstrapRuns{
		cache:    filepath.Join(root, "cache"),
		shim:     filepath.Join(root, "shim"),
		buildLog: filepath.Join(root, "builds.log"),
	}
	if err = os.MkdirAll(runs.cache, 0o700); err != nil {
		return err
	}
	if err = os.MkdirAll(runs.shim, 0o700); err != nil {
		return err
	}
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = build ]; then printf 'build\\n' >> '%s'; fi\nexec '%s' \"$@\"\n", runs.buildLog, goPath)
	if err = os.WriteFile(filepath.Join(runs.shim, "go"), []byte(script), 0o700); err != nil {
		return err
	}
	driver.addCLI.bootstrap = runs

	return nil
}

// runBootstrapTwice runs the repository's ./hippo bootstrap twice against the
// same cache, with the logging go command first on the PATH.
func (driver *Driver) runBootstrapTwice() error {
	runs := &driver.addCLI.bootstrap
	if runs.cache == "" {
		return errors.New("no build cache was prepared")
	}
	repository, err := toolRoot()
	if err != nil {
		return err
	}
	values := map[string]string{
		bootstrapCacheVariable: runs.cache,
		"PATH":                 runs.shim + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	for range 2 {
		command := exec.Command(filepath.Join(repository, "hippo"), "version")
		command.Env = environmentWith(values)
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		command.Stdout, command.Stderr = stdout, stderr
		code := 0
		if runError := command.Run(); runError != nil {
			exitError, failed := errors.AsType[*exec.ExitError](runError)
			if !failed {
				return runError
			}
			code = exitError.ExitCode()
		}
		builds, err := countBuildLines(runs.buildLog)
		if err != nil {
			return err
		}
		runs.exits = append(runs.exits, code)
		runs.compiles = append(runs.compiles, builds)
		runs.outputs = append(runs.outputs, stdout.String()+stderr.String())
	}

	return nil
}

// countBuildLines counts the builds the logging go command has started; a missing log is none.
func countBuildLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return strings.Count(string(data), "build\n"), nil
}

// requireOnlyFirstBootstrapCompiles holds the two runs to exit 0, to one compile
// by the first run, and to no further compile by the second, which reuses the
// cached binary and so prints what the first run printed.
func (driver *Driver) requireOnlyFirstBootstrapCompiles() error {
	runs := driver.addCLI.bootstrap
	if len(runs.exits) != 2 || runs.exits[0] != 0 || runs.exits[1] != 0 {
		return fmt.Errorf("bootstrap exits %v, want 0 and 0", runs.exits)
	}
	if len(runs.compiles) != 2 || runs.compiles[0] != 1 || runs.compiles[1] != 1 {
		return fmt.Errorf("go build ran %v times across the runs, want 1 after the first and still 1 after the second", runs.compiles)
	}
	if runs.outputs[0] == "" || runs.outputs[0] != runs.outputs[1] {
		return fmt.Errorf("the runs printed %q and %q, want the same version output", runs.outputs[0], runs.outputs[1])
	}

	return nil
}
