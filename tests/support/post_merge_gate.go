package support

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// postMergeWiring holds the workflow texts that decide where the full gate
// runs. The pull-request gate must stay light; the main test workflow must run
// the full gate after every merge; the release workflow must wait for it.
type postMergeWiring struct {
	pullRequest string
	mainTest    string
	mainTrigger string
}

// inspectPostMergeGate reads the three workflows once. A missing workflow fails
// the step, so a scenario cannot pass against a file that was never written.
func (driver *Driver) inspectPostMergeGate() error {
	root, err := toolRoot()
	if err != nil {
		return err
	}
	read := func(name string) (string, error) {
		data, readError := os.ReadFile(filepath.Join(root, ".github", "workflows", name))

		return string(data), readError
	}
	pullRequest, err := read("pr-quality-gate.yml")
	if err != nil {
		return err
	}
	mainTest, err := read("main-test.yml")
	if err != nil {
		return err
	}
	driver.postMerge = postMergeWiring{
		pullRequest: pullRequest,
		mainTest:    mainTest,
		mainTrigger: workflowTriggers(mainTest),
	}

	return nil
}

// requireNoFullGateOnPullRequests refuses the full test script and its matrix
// job in the pull-request gate. The gate must mirror the hooks and nothing
// heavier, so a full run there would slow every pull request again.
func (driver *Driver) requireNoFullGateOnPullRequests() error {
	pullRequest := driver.postMerge.pullRequest
	if strings.Contains(pullRequest, "./scripts/test.sh") || strings.Contains(pullRequest, "\n  test:") ||
		strings.Contains(pullRequest, "matrix:") {
		return errors.New("the pull request gate still runs the full gate matrix")
	}

	return nil
}

// requirePostMergeFullGate requires the post-merge workflow to run the full
// gate on push to main and on manual dispatch, across both operating systems
// the test job covered, with the same timeout and read-only permissions.
func (driver *Driver) requirePostMergeFullGate() error {
	wiring := driver.postMerge
	if wiring.mainTrigger != "push,workflow_dispatch" ||
		!containsAll(wiring.mainTest, "branches: [main]", "./scripts/test.sh", "os: [ubuntu-24.04, macos-15]",
			"fail-fast: false", "timeout-minutes: 45", "permissions:\n  contents: read") {
		return errors.New("the post-merge workflow does not run the full gate on push to main across both operating systems")
	}

	return nil
}

// requirePostMergeNotCancelled refuses a concurrency setting that could cancel
// the run for one main commit when a later commit lands, so every main commit
// keeps its own result.
func (driver *Driver) requirePostMergeNotCancelled() error {
	mainTest := driver.postMerge.mainTest
	if !strings.Contains(mainTest, "cancel-in-progress: false") || strings.Contains(mainTest, "cancel-in-progress: true") {
		return errors.New("the post-merge workflow can cancel a main commit's run for a later commit")
	}

	return nil
}

// requireReproducibilityAfterMerge moves the macOS reproducibility job out of
// the pull-request gate and into the post-merge workflow, where it runs with
// the full gate.
func (driver *Driver) requireReproducibilityAfterMerge() error {
	if strings.Contains(driver.postMerge.pullRequest, "\n  release-reproducibility:") ||
		!strings.Contains(driver.postMerge.mainTest, "\n  release-reproducibility:") {
		return errors.New("the macOS release reproducibility check is not scheduled after merge alone")
	}

	return nil
}

// requireV04ReleasePostMergeTestGate requires the release workflow to ask
// GitHub for a successful main test run on the tagged commit, with read access
// to Actions, and to make that check before it publishes.
func requireV04ReleasePostMergeTestGate(string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	workflow, err := os.ReadFile(filepath.Join(moduleRoot, ".github", "workflows", "release.yml"))
	if err != nil {
		return err
	}
	text := string(workflow)
	check := strings.Index(text, "main-test.yml")
	publish := strings.Index(text, "Publish immutable GitHub Release")
	if check < 0 || publish < 0 || check > publish ||
		!containsAll(text, "head_sha=$GITHUB_SHA", "status=success", "actions: read") {
		return errors.New("release workflow does not require a passing post-merge test run before publishing")
	}

	return nil
}
