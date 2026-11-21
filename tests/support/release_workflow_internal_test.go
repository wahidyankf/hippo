package support

import "testing"

// Each test below is the unit form of a release workflow scenario removed from
// specs/behaviours/artifacts.feature. It runs the check that scenario's step ran, so a changed line in
// .github/workflows/release.yml (or the CI release job in pr-quality-gate.yml) fails with that check's message.

// TestReleaseWorkflowPeelsAnnotatedAndLightweightTags covers the removed scenario "Release workflow peels annotated and
// lightweight tags".
func TestReleaseWorkflowPeelsAnnotatedAndLightweightTags(t *testing.T) {
	if err := requireV04ReleaseTagPeeling(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

// TestReleaseWorkflowRequiresPeeledTagToEqualEventCommit covers the removed scenario "Release workflow requires the
// peeled tag to equal the event commit".
func TestReleaseWorkflowRequiresPeeledTagToEqualEventCommit(t *testing.T) {
	if err := requireV04ReleaseEventCommitMismatch(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

// TestReleaseWorkflowRequiresOriginMainAncestry covers the removed scenario "Release workflow requires origin main
// ancestry".
func TestReleaseWorkflowRequiresOriginMainAncestry(t *testing.T) {
	if err := requireV04ReleaseMainReachability(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

// TestReleaseWorkflowAddsNoActionsStorage covers the removed scenario "Release workflow adds no Actions storage".
func TestReleaseWorkflowAddsNoActionsStorage(t *testing.T) {
	if err := requireV04ReleaseStorageNeutrality(""); err != nil {
		t.Fatal(err)
	}
}

// TestReleaseWorkflowsDisableImplicitGoModuleCaching covers the removed scenario "Release workflows disable implicit Go
// module caching".
func TestReleaseWorkflowsDisableImplicitGoModuleCaching(t *testing.T) {
	if err := requireV04ReleaseCacheDisabled(""); err != nil {
		t.Fatal(err)
	}
}

// TestCIValidatesAssetsAgainstExactBuildCommit covers the removed scenario "CI validates assets against the exact build
// commit".
func TestCIValidatesAssetsAgainstExactBuildCommit(t *testing.T) {
	if err := requireV04CIValidatorCommit(""); err != nil {
		t.Fatal(err)
	}
}

// TestReleaseWorkflowRequiresPassingPostMergeRun covers the removed scenario "Release workflow requires a passing
// post-merge run for the tagged commit".
func TestReleaseWorkflowRequiresPassingPostMergeRun(t *testing.T) {
	if err := requireV04ReleasePostMergeTestGate(""); err != nil {
		t.Fatal(err)
	}
}
