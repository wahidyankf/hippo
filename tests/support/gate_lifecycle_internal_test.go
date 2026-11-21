package support

import "testing"

// wiringCheck is one assertion over the gate wiring that a removed scenario made.
type wiringCheck struct {
	name string
	run  func() error
}

func runWiringChecks(t *testing.T, checks []wiringCheck) {
	t.Helper()
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestFullGateRunsAfterMergeNotOnPullRequests covers the removed scenario "Full gate runs after merge on main, not on
// pull requests". The main workflow runs the full gate after every merge without cancelling any commit's run, and the
// macOS reproducibility job runs there; the pull-request gate runs no full matrix.
func TestFullGateRunsAfterMergeNotOnPullRequests(t *testing.T) {
	driver := &Driver{}
	if err := driver.inspectPostMergeGate(); err != nil {
		t.Fatal(err)
	}
	runWiringChecks(t, []wiringCheck{
		{"the pull request gate runs no full gate matrix", driver.requireNoFullGateOnPullRequests},
		{"the post-merge workflow runs the full gate on push to main on both operating systems", driver.requirePostMergeFullGate},
		{"the post-merge workflow is never cancelled by a later commit", driver.requirePostMergeNotCancelled},
		{"the macOS release reproducibility check runs after merge only", driver.requireReproducibilityAfterMerge},
	})
}

// TestStableLifecycleWiringIsComplete covers the removed scenario "Stable lifecycle wiring is complete". The hooks and
// the pull-request caller dispatch only closed v0.4 surfaces, the declared runners keep every owned policy validator,
// and the harness declaration owns three non-overlapping projections.
func TestStableLifecycleWiringIsComplete(t *testing.T) {
	driver := &Driver{}
	if err := driver.inspectDocumentationGate(); err != nil {
		t.Fatal(err)
	}
	runWiringChecks(t, []wiringCheck{
		{"the hooks and pull request caller dispatch only closed v0.4 surfaces", driver.requireSharedDocumentationCheck},
		{"the declared main and pull request runners retain every owned policy validator", driver.requirePinnedValidatorsRun},
		{"the harness declaration owns non-overlapping Claude Codex and OpenCode projections", driver.requireThreeHarnessProfiles},
	})
}
