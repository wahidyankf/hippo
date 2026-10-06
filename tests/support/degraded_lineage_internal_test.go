package support

import (
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
)

// TestTheGuardReadsDegradedAdmissionFromTheLineageAlone holds the guard to the
// lineage a resolution carries, so a hand-built resolution cannot disagree with
// itself: Resolution.DegradedAdmission is what status publishes, and a stale or
// invented value of it neither grants nor takes away the admission.
func TestTheGuardReadsDegradedAdmissionFromTheLineageAlone(t *testing.T) {
	for _, row := range []struct {
		name      string
		base      string
		published bool
		admitted  bool
	}{
		{"a balanced lineage whose published flag is cleared", profileBalanced, false, true},
		{"a constrained lineage whose published flag is set", profileConstrained, true, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			driver := &Driver{}
			defer driver.Close()
			driver.stableDarwinWarning()
			if err := driver.derivedProfileConfiguration(row.base); err != nil {
				t.Fatal(err)
			}
			resolution, err := driver.resolveConfigured(policy.TaskEphemeral, driver.samples[len(driver.samples)-1])
			if err != nil {
				t.Fatal(err)
			}
			resolution.DegradedAdmission = row.published

			if err = driver.guardUnder(resolution); err != nil {
				t.Fatal(err)
			}
			if admitted := strings.Contains(driver.errorOutput, degradedAdmissionLine); admitted != row.admitted {
				t.Errorf("lineage %d with published flag %v: admitted degraded = %v, want %v (exit %d, stderr %q)",
					resolution.Lineage, row.published, admitted, row.admitted, driver.exitCode, driver.errorOutput)
			}
		})
	}
}

// TestDegradedAdmissionIgnoresARunnerStall drives both scenarios' own step
// bindings with a runner that takes ten real milliseconds over every reading.
// The admission window counts the guard's readings, so a slow runner admits
// balanced's lineage and defers every other one, as a fast runner does.
func TestDegradedAdmissionIgnoresARunnerStall(t *testing.T) {
	for _, row := range []struct {
		base    string
		require func(*Driver) error
	}{
		{profileBalanced, (*Driver).requireConfiguredDegradedAdmission},
		{profileConstrained, (*Driver).requireConfiguredDeferral},
	} {
		t.Run(row.base, func(t *testing.T) {
			driver := &Driver{readingStall: 10 * time.Millisecond}
			defer driver.Close()
			driver.stableDarwinWarning()
			if err := driver.derivedProfileConfiguration(row.base); err != nil {
				t.Fatal(err)
			}
			if err := driver.guardUnderConfiguration(); err != nil {
				t.Fatal(err)
			}
			if err := row.require(driver); err != nil {
				t.Error(err)
			}
		})
	}
}
