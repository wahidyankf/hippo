package support

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
)

// testBuildVersion is the version every harness-built binary carries.
// tests/e2e/run.sh and the quick gate stamp the same value, with an all-zero
// commit, and the scenario "Version identifies the exact build" requires it.
const testBuildVersion = "v0.0.0-test"

// e2eBinarySelection is what one binary-selection scenario staged and chose.
type e2eBinarySelection struct {
	inherited, chosen string
}

// RunCompiled is the end-to-end package's TestMain body. It chooses the binary
// the adapter runs, hands it on as HIPPO_BIN, then isolates the package as
// RunIsolated does. A helper the package re-executes keeps its parent's choice.
//
//	func TestMain(m *testing.M) { os.Exit(support.RunCompiled(m)) }
func RunCompiled(m *testing.M) int {
	if os.Getenv(isolatedRootVariable) != "" {
		return m.Run()
	}
	binary, cleanup, err := e2eBinary(os.Getenv("HIPPO_BIN"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "choose the end-to-end binary: %v\n", err)

		return 1
	}
	defer cleanup()
	if err = os.Setenv("HIPPO_BIN", binary); err != nil {
		fmt.Fprintf(os.Stderr, "choose the end-to-end binary: %v\n", err)

		return 1
	}

	return RunIsolated(m)
}

// e2eBinary returns the binary the end-to-end adapter runs, and a cleanup for
// anything it created. It keeps an inherited HIPPO_BIN only when that binary
// carries the harness's stamp, as the one tests/e2e/run.sh builds does. Any
// other value is not evidence about this working tree: a guarded shell
// exports the guard's own binary under the same name, and a bare `go test
// ./...` exports nothing. Then it builds the working tree with the stamp.
func e2eBinary(inherited string) (string, func(), error) {
	if inherited != "" && requireTestStamp(inherited) == nil {
		return inherited, func() {}, nil
	}
	parent := os.Getenv("HIPPO_E2E_TEMP_PARENT")
	directory, err := os.MkdirTemp(parent, "hippo-e2e-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) } //nolint:gosec // The directory is the one MkdirTemp just created for this build.
	binary := filepath.Join(directory, "hippo")
	goBinary := os.Getenv("HIPPO_GO_BINARY")
	if goBinary == "" {
		goBinary = "go"
	}
	// The same flags and bounded concurrency as tests/e2e/run.sh.
	build := exec.Command(goBinary, "build", "-p=1", "-trimpath", "-ldflags", //nolint:gosec // HIPPO_GO_BINARY is the harness input naming the Go toolchain, as tests/e2e/run.sh reads it.
		"-X github.com/wahidyankf/hippo/internal/cli.Version="+testBuildVersion+
			" -X github.com/wahidyankf/hippo/internal/cli.Commit="+strings.Repeat("0", 40),
		"-o", binary, "./cmd/hippo")
	build.Dir = toolRoot()
	build.Env = append(os.Environ(), "GOMAXPROCS=2")
	if output, buildError := build.CombinedOutput(); buildError != nil {
		cleanup()

		return "", func() {}, fmt.Errorf("build the working tree: %w: %s", buildError, output)
	}

	return binary, cleanup, nil
}

func (driver *Driver) e2eBinaryBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(
			`^(no binary|an unstamped build|a build stamped v0\.0\.0-test) as the inherited HIPPO_BIN$`,
			driver.inheritedE2EBinary,
		),
		step(`^the end-to-end adapter chooses the binary it runs$`, driver.chooseE2EBinary),
		step(`^it runs a build of the working tree stamped v0\.0\.0-test$`, driver.requireBuiltE2EBinary),
		step(`^it runs that build unchanged$`, driver.requireInheritedE2EBinary),
	}
}

// inheritedE2EBinary stages what an invoking shell may have exported. An
// unstamped build stands in for the one an enclosing guard exports as its own
// HIPPO_BIN; both fixtures answer only the version query the choice asks.
func (driver *Driver) inheritedE2EBinary(kind string) error {
	if kind == "no binary" {
		return nil
	}
	directory, err := os.MkdirTemp("", "hippo-inherited-")
	if err != nil {
		return err
	}
	driver.temporaryPaths = append(driver.temporaryPaths, directory)
	version, commit := "dev", "unknown"
	if kind == "a build stamped v0.0.0-test" {
		version, commit = testBuildVersion, strings.Repeat("0", 40)
	}
	driver.e2eSelection.inherited = filepath.Join(directory, "hippo")
	program := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' '{\"schemaVersion\":1,\"version\":%q,\"commit\":%q}'\n", version, commit)

	return os.WriteFile(driver.e2eSelection.inherited, []byte(program), 0o700)
}

func (driver *Driver) chooseE2EBinary() error {
	chosen, _, err := e2eBinary(driver.e2eSelection.inherited)
	if err != nil {
		return err
	}
	driver.e2eSelection.chosen = chosen
	// A build lives in a directory of its own, removed with the scenario's
	// other temporary paths once the outcome has queried it.
	if chosen != driver.e2eSelection.inherited {
		driver.temporaryPaths = append(driver.temporaryPaths, filepath.Dir(chosen))
	}

	return nil
}

func (driver *Driver) requireBuiltE2EBinary() error {
	if driver.e2eSelection.chosen == driver.e2eSelection.inherited {
		return fmt.Errorf("the adapter kept the inherited binary %q", driver.e2eSelection.inherited)
	}

	return requireTestStamp(driver.e2eSelection.chosen)
}

func (driver *Driver) requireInheritedE2EBinary() error {
	if driver.e2eSelection.chosen != driver.e2eSelection.inherited {
		return fmt.Errorf("the adapter replaced the stamped binary it was handed with %q", driver.e2eSelection.chosen)
	}

	return nil
}

// requireTestStamp fails unless binary reports the identity the harness stamps.
func requireTestStamp(binary string) error {
	output, err := exec.Command(binary, versionCommandName, jsonFlag).Output() //nolint:gosec // The binary is the harness input HIPPO_BIN or a fixture, asked only for its version.
	if err != nil {
		return fmt.Errorf("query %s for its version: %w", binary, err)
	}
	var identity struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err = json.Unmarshal(output, &identity); err != nil {
		return fmt.Errorf("read the version of %s: %w", binary, err)
	}
	if identity.Version != testBuildVersion || identity.Commit != strings.Repeat("0", 40) {
		return fmt.Errorf("%s reports %s (%s), want %s", binary, identity.Version, identity.Commit, testBuildVersion)
	}

	return nil
}
