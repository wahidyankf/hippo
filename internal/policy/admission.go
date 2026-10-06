package policy

import "errors"

// AdmissionPath is the one way host evidence admits, defers, or refuses work.
// run, status, and the behaviour driver each act on the path DecideAdmission
// returns, so none keeps a rule of its own. The zero value, AdmissionUnset, is
// the path of no decision; DecideAdmission returns it only with an error.
type AdmissionPath uint8

const (
	// AdmissionUnset is the zero value: no path was decided.
	AdmissionUnset AdmissionPath = iota
	// AdmissionNormal admits the work at the resolved concurrency.
	AdmissionNormal
	// AdmissionDegraded admits ephemeral work at concurrency one under a stable
	// macOS warning, which only a profile of the balanced lineage may do.
	AdmissionDegraded
	// AdmissionWait keeps sampling until the admission window closes, and defers
	// the work if it does.
	AdmissionWait
	// AdmissionCleanup stops the work until storage is cleaned up.
	AdmissionCleanup
	// AdmissionReplan stops work whose resolved profile cannot be made to fit,
	// until a different capacity envelope is requested.
	AdmissionReplan
)

// EvidenceWindow is what the samples handed to DecideAdmission are: the same
// pressure reads differently from the two samples status takes than from the
// run of samples a guarded run collects. The zero value, WindowUnset, is no
// window, which DecideAdmission refuses.
type EvidenceWindow uint8

const (
	// WindowUnset is the zero value: the samples are not described.
	WindowUnset EvidenceWindow = iota
	// WindowSnapshot is the two samples status takes, which cannot fill the
	// window a stable warning needs: a normal state admits, anything else waits.
	WindowSnapshot
	// WindowSampling is the samples a guarded run collects while it waits to be
	// admitted: consecutive safe samples admit, and a stable warning may admit
	// degraded.
	WindowSampling
)

// AdmissionInput is everything DecideAdmission reads. Every field is required,
// and the repository's lint configuration refuses a literal that omits one.
//
// Policy is the resource policy the samples are assessed and admitted against.
// It is carried apart from Resolution because callers set the two
// independently: run admits against the policy its caller configures, which
// the behaviour driver sets beside a separate or zero Resolution, while status
// and the driver's assessment admit against Resolution.Policy. DecideAdmission
// reads Policy alone, never Resolution.Policy.
type AdmissionInput struct {
	Resolution Resolution
	TaskClass  TaskClass
	Samples    []Sample
	Policy     Policy
	Window     EvidenceWindow
}

// DecideAdmission decides the admission path for one input, in order:
//
//  1. a resolution already at replan or cleanup decides its own path;
//  2. a storage-blocked assessment is cleanup;
//  3. a snapshot of a normal state is normal, and of anything else, wait;
//  4. a sampling window that is ready is normal, a stable warning that the
//     task's lineage may be spared is degraded, and anything else is wait;
//  5. an unset or unknown window is an error, and the path is AdmissionUnset.
func DecideAdmission(input AdmissionInput) (AdmissionPath, error) {
	switch input.Resolution.Decision {
	case DecisionReplan:
		return AdmissionReplan, nil
	case DecisionCleanup:
		return AdmissionCleanup, nil
	case DecisionRun, DecisionWait:
	}

	assessment := ResourceAssessment(input.Samples, input.Policy)
	if assessment.StorageBlocked {
		return AdmissionCleanup, nil
	}

	switch input.Window {
	case WindowSnapshot:
		if assessment.State == StateNormal {
			return AdmissionNormal, nil
		}

		return AdmissionWait, nil
	case WindowSampling:
		if AdmissionReady(input.Samples, input.Policy) {
			return AdmissionNormal, nil
		}
		if SparesStableWarning(input.TaskClass, input.Resolution, input.Samples, input.Policy) {
			return AdmissionDegraded, nil
		}

		return AdmissionWait, nil
	case WindowUnset:
	}

	return AdmissionUnset, errors.New("admission window is not a snapshot or a sampling window")
}

// SparesStableWarning reports whether a stable macOS warning never counts
// toward the grace of a running child of the class and resolution, however that
// child was admitted: the same warning would admit it degraded now. It is the
// one eligibility rule, so DecideAdmission's degraded path is this function
// holding for a host that is not already normal. It takes the policy the
// samples are assessed against, for the same reason AdmissionInput does.
func SparesStableWarning(class TaskClass, resolution Resolution, samples []Sample, resourcePolicy Policy) bool {
	return class == TaskEphemeral &&
		resolution.Lineage.DegradedAdmission() &&
		WarningAdmissionReady(samples, resourcePolicy)
}
