package application

import "io"

// DisplayAdvisories retains only invocation-local annotation operation flags.
// Native adapters supply errors for classification and the current writer
// separately; the policy retains neither an error nor a caller stream.
type DisplayAdvisories struct {
	publicationPending  bool
	publicationReported bool
	removalPending      bool
	removalReported     bool
}

// RecordPublication classifies an attempted annotation publication failure.
func (advisories *DisplayAdvisories) RecordPublication(err error) {
	if err != nil {
		advisories.publicationPending = true
	}
}

// RecordRemoval classifies an attempted owned annotation removal failure.
func (advisories *DisplayAdvisories) RecordRemoval(err error) {
	if err != nil {
		advisories.removalPending = true
	}
}

// Report attempts each pending operation once using only fixed, safe text.
// The writer is supplied for this call and its failures do not affect the run.
func (advisories *DisplayAdvisories) Report(stderr io.Writer) {
	if advisories.publicationPending && !advisories.publicationReported {
		advisories.publicationReported = true
		if stderr != nil {
			_, _ = io.WriteString(stderr, "HIPPO advisory: display label publication failed; monitoring may show an unavailable label.\n")
		}
	}
	if advisories.removalPending && !advisories.removalReported {
		advisories.removalReported = true
		if stderr != nil {
			_, _ = io.WriteString(stderr, "HIPPO advisory: display label cleanup failed; later reconciliation may be needed.\n")
		}
	}
}
