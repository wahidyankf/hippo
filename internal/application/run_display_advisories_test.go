package application_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
)

const (
	publicationAdvisory = "HIPPO advisory: display label publication failed; monitoring may show an unavailable label.\n"
	removalAdvisory     = "HIPPO advisory: display label cleanup failed; later reconciliation may be needed.\n"
)

type opaqueDisplayError struct{}

func (opaqueDisplayError) Error() string { panic("annotation errors must never be rendered") }

type refusedAdvisoryWriter struct{ calls int }

func (writer *refusedAdvisoryWriter) Write([]byte) (int, error) {
	writer.calls++
	return 0, errors.New("/private/expired-caller\x1b[31m")
}

func TestDisplayAdvisoriesHealthyAndZeroValueAreSilent(t *testing.T) {
	var advisories application.DisplayAdvisories
	var output bytes.Buffer
	advisories.Report(&output)
	advisories.RecordPublication(nil)
	advisories.RecordRemoval(nil)
	advisories.Report(&output)
	if output.Len() != 0 {
		t.Fatalf("healthy/no-operation annotation emitted %q", output.String())
	}
}

func TestDisplayAdvisoriesClassifyAndReportEachOperationOnce(t *testing.T) {
	for _, operation := range []string{"publication", "removal", "both"} {
		t.Run(operation, func(t *testing.T) {
			var advisories application.DisplayAdvisories
			var output bytes.Buffer
			want := ""
			if operation != "removal" {
				advisories.RecordPublication(opaqueDisplayError{})
				want += publicationAdvisory
			}
			if operation != "publication" {
				advisories.RecordRemoval(opaqueDisplayError{})
				want += removalAdvisory
			}
			advisories.Report(&output)
			advisories.RecordPublication(nil)
			advisories.RecordRemoval(nil)
			advisories.Report(&output)
			if operation != "removal" {
				advisories.RecordPublication(opaqueDisplayError{})
			}
			if operation != "publication" {
				advisories.RecordRemoval(opaqueDisplayError{})
			}
			advisories.Report(&output)
			if output.String() != want {
				t.Fatalf("operation %s output=%q want fixed, once-only %q", operation, output.String(), want)
			}
		})
	}
}

func TestDisplayAdvisoriesLaterOperationRemainsReportable(t *testing.T) {
	var advisories application.DisplayAdvisories
	var output bytes.Buffer
	advisories.RecordPublication(opaqueDisplayError{})
	advisories.Report(&output)
	if output.String() != publicationAdvisory {
		t.Fatalf("publication report=%q", output.String())
	}
	advisories.RecordRemoval(opaqueDisplayError{})
	advisories.Report(&output)
	advisories.Report(&output)
	if output.String() != publicationAdvisory+removalAdvisory {
		t.Fatalf("later independent operation lost or duplicated: %q", output.String())
	}
}

func TestDisplayAdvisoriesFailedWriterIsBestEffortAndNeverRetried(t *testing.T) {
	var advisories application.DisplayAdvisories
	writer := &refusedAdvisoryWriter{}
	advisories.RecordPublication(opaqueDisplayError{})
	advisories.RecordRemoval(opaqueDisplayError{})
	advisories.Report(writer)
	if writer.calls != 2 {
		t.Fatalf("failed writer got %d calls, want one attempt for each pending operation", writer.calls)
	}
	advisories.Report(writer)
	var later bytes.Buffer
	advisories.Report(&later)
	if writer.calls != 2 || later.Len() != 0 {
		t.Fatalf("failed diagnostics retried or escaped to later caller: calls=%d later=%q", writer.calls, later.String())
	}
}

func TestDisplayAdvisoriesRetainOnlyPrivateBooleanOperationState(t *testing.T) {
	var advisories application.DisplayAdvisories
	advisories.RecordPublication(errors.New("/private/root owner-label secret-argument\x1b[31m"))
	advisories.RecordRemoval(errors.New("/private/raw-removal-error"))
	shape := reflect.TypeOf(advisories)
	if shape.Kind() != reflect.Struct || shape.NumField() != 4 {
		t.Fatalf("policy must contain only the four invocation operation flags: %v", shape)
	}
	for field := range shape.Fields() {
		if field.IsExported() || field.Type.Kind() != reflect.Bool {
			t.Fatalf("policy retains non-private/non-boolean state: %v", field)
		}
	}
	var output bytes.Buffer
	advisories.Report(&output)
	if output.String() != publicationAdvisory+removalAdvisory || strings.Contains(output.String(), "private") {
		t.Fatalf("policy diagnostic leaked input or changed framing: %q", output.String())
	}
}
