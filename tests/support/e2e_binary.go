package support

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// testBuildVersion is the version every harness-built binary carries.
// tests/e2e/run.sh and RunCompiled stamp the same value, with an all-zero
// commit, and the scenario "Version identifies the exact build" requires it.
const testBuildVersion = "v0.0.0-test"

// StampedBuildLinkerFlags returns the -ldflags value that makes a build of
// ./cmd/hippo a harness test build. Only such a build honours the fixed host
// evidence QuietHostEnvironment publishes; a release or plain build samples the
// live host, whose load is the runner's and not the test's.
func StampedBuildLinkerFlags() string {
	return "-X github.com/wahidyankf/hippo/internal/adapters/cli.Version=" + testBuildVersion +
		" -X github.com/wahidyankf/hippo/internal/adapters/cli.Commit=" + strings.Repeat("0", 40)
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
	repository, rootError := toolRoot()
	if rootError != nil {
		return "", func() {}, rootError
	}
	parent := os.Getenv("HIPPO_E2E_TEMP_PARENT")
	directory, err := os.MkdirTemp(parent, "hippo-e2e-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) } //nolint:gosec // The directory is the one MkdirTemp just created for this build.
	// Register the acquired path first: failed resolution still owns cleanup.
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	if !filepath.IsAbs(canonical) || filepath.Clean(canonical) != canonical {
		cleanup()
		return "", func() {}, fmt.Errorf("candidate build directory is not a clean absolute path: %s", canonical)
	}
	if err := validateOwnedCandidateDirectory(canonical); err != nil {
		cleanup()
		return "", func() {}, err
	}
	directory = canonical
	binary := filepath.Join(directory, "hippo")
	goBinary := os.Getenv("HIPPO_GO_BINARY")
	if goBinary == "" {
		goBinary = "go"
	}
	// The same flags and bounded concurrency as tests/e2e/run.sh.
	build := exec.Command(goBinary, "build", "-p=1", "-trimpath", "-ldflags", //nolint:gosec // HIPPO_GO_BINARY is the harness input naming the Go toolchain, as tests/e2e/run.sh reads it.
		StampedBuildLinkerFlags(), "-o", binary, "./cmd/hippo")
	build.Dir = repository
	build.Env = append(os.Environ(), "GOMAXPROCS=2")
	if output, buildError := build.CombinedOutput(); buildError != nil {
		cleanup()

		return "", func() {}, fmt.Errorf("build the working tree: %w: %s", buildError, output)
	}

	return binary, cleanup, nil
}

func validateOwnedCandidateDirectory(canonical string) error {
	resolved, err := filepath.EvalSymlinks(canonical)
	if err != nil {
		return err
	}
	if resolved != canonical {
		return errors.New("candidate build directory contains a symlink")
	}
	root, err := os.OpenRoot(canonical)
	if err != nil {
		return err
	}
	info, statError := root.Lstat(".")
	if err := errors.Join(statError, root.Close()); err != nil {
		return err
	}
	if info == nil {
		return errors.New("candidate build directory inspection returned no identity")
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return errors.New("candidate build directory is not private and owned")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int64(stat.Uid) != int64(os.Getuid()) {
		return errors.New("candidate build directory has a foreign owner")
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
