package support

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
)

func TestAnnotationUnitCanonicalBindingsReachActualRunAndSharedPolicy(t *testing.T) {
	requireAnnotationCanonicalFeature(t)
	for _, operation := range []string{"write", "removal"} {
		t.Run(operation, func(t *testing.T) { checkAnnotationCanonicalRow(t, operation) })
	}
}

func requireAnnotationCanonicalFeature(t *testing.T) {
	t.Helper()
	root, err := toolRoot()
	if err != nil {
		t.Fatal(err)
	}
	feature, err := os.ReadFile(filepath.Join(root, "specs", "behaviours", "execution.feature"))
	if err != nil {
		t.Fatal(err)
	}
	canonical := []string{
		"Given a labeled guarded run whose annotation <operation> fails",
		"When that guarded run completes",
		"Then its admission resources child status and release outcome remain unchanged",
		"And the annotation failure is reported without private paths",
	}
	for _, line := range canonical {
		if !strings.Contains(string(feature), line) {
			t.Fatalf("canonical annotation assertion changed or absent: %s", line)
		}
	}
}

func checkAnnotationCanonicalRow(t *testing.T, operation string) {
	t.Helper()
	driver := NewDriver(contract.Adapter{Name: contract.Unit})
	defer driver.Close()
	driver.Reset()
	texts := []string{"a labeled guarded run whose annotation " + operation + " fails", "that guarded run completes", "its admission resources child status and release outcome remain unchanged", "the annotation failure is reported without private paths"}
	selected := selectAnnotationCanonicalBindings(t, driver, texts)
	runAnnotationCanonicalSteps(t, driver, selected, operation)
}

func selectAnnotationCanonicalBindings(t *testing.T, driver *Driver, texts []string) []contract.StepBinding {
	t.Helper()
	selected := make([]contract.StepBinding, 0, 4)
	for _, text := range texts {
		matches := 0
		for _, binding := range driver.Bindings() {
			pattern, err := regexp.Compile(binding.Pattern)
			if err != nil {
				t.Fatal(err)
			}
			if pattern.MatchString(text) {
				matches++
				selected = append(selected, binding)
			}
		}
		if matches != 1 {
			t.Fatalf("canonical step %q resolves to %d handlers", text, matches)
		}
	}
	if findings := contract.ValidateBindings(selected, texts); len(findings) != 0 {
		t.Fatal(findings)
	}
	return selected
}

func runAnnotationCanonicalSteps(t *testing.T, driver *Driver, selected []contract.StepBinding, operation string) {
	t.Helper()
	given, ok := selected[0].Handler.(func(string) error)
	if !ok {
		t.Fatal("canonical annotation Given has no real operation handler")
	}
	when, ok := selected[1].Handler.(func(context.Context) error)
	if !ok {
		t.Fatal("canonical annotation When has no caller-context handler")
	}
	if err := given(operation); err != nil {
		t.Fatal(err)
	}
	if err := when(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, binding := range selected[2:] {
		assertion, ok := binding.Handler.(func() error)
		if !ok {
			t.Fatal("canonical annotation Then has no actual assertion")
		}
		if err := assertion(); err != nil {
			t.Fatal(err)
		}
	}
	if driver.annotationRun.memory == nil || driver.annotationRun.native != nil {
		t.Fatal("canonical Unit did not use only the in-memory application subject")
	}
}

type annotationMemoryRefusedWriter struct{ calls int }

func (writer *annotationMemoryRefusedWriter) Write([]byte) (int, error) {
	writer.calls++
	return 0, errors.New("/private/caller-write-error")
}

func TestAnnotationMemoryRunHealthyInheritedMissingAndFailedWriterControls(t *testing.T) {
	for _, reservation := range []bool{false, true} {
		for _, control := range []string{"healthy", "inherited", "missing", "failed-writer"} {
			t.Run(control+"-"+map[bool]string{false: "exclusive", true: "reservation"}[reservation], func(t *testing.T) {
				checkAnnotationMemoryControl(t, reservation, control)
			})
		}
	}
}

func TestAnnotationMemoryRunMatchesHealthyAdmissionAndLifetime(t *testing.T) {
	for _, reservation := range []bool{false, true} {
		healthy := newAnnotationMemoryRun("healthy")
		healthy.run(t.Context(), reservation)
		if err := requireAnnotationMemoryOutcome(healthy, reservation); err != nil {
			t.Fatal(err)
		}
		for _, operation := range []string{"write", "removal"} {
			fixture := newAnnotationMemoryRun(operation)
			fixture.run(t.Context(), reservation)
			if err := requireAnnotationMemoryOutcome(fixture, reservation); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(fixture.started.Resolution, healthy.started.Resolution) || strings.Join(fixture.started.Environment, "\n") != strings.Join(healthy.started.Environment, "\n") || fixture.lease.Allocation != healthy.lease.Allocation || fixture.outcome != healthy.outcome || fixture.exit != healthy.exit {
				t.Fatal("annotation changed actual application allocation, child environment or lifetime outcome")
			}
			if err := requireAnnotationDiagnostics("", fixture.stderr.String(), operation); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func checkAnnotationMemoryControl(t *testing.T, reservation bool, control string) {
	t.Helper()
	fixture, writer := prepareAnnotationMemoryControl(control)
	fixture.run(t.Context(), reservation)
	if err := requireAnnotationMemoryOutcome(fixture, reservation); err != nil {
		t.Fatal(err)
	}
	if fixture.stderr.Len() != 0 {
		t.Fatalf("silent control emitted diagnostic %q", fixture.stderr.String())
	}
	if control == "inherited" && (fixture.writes != 0 || fixture.removals != 0 || fixture.failures != 0) {
		t.Fatal("inherited run performed annotation effects")
	}
	if control == "failed-writer" {
		requireAnnotationRefusedWriter(t, fixture, writer)
	} else if fixture.failures != 0 {
		t.Fatalf("healthy control invented %d failures", fixture.failures)
	}
}

func prepareAnnotationMemoryControl(control string) (*annotationMemoryRun, *annotationMemoryRefusedWriter) {
	operation := "healthy"
	if control == "failed-writer" || control == "inherited" {
		operation = "removal"
	}
	fixture := newAnnotationMemoryRun(operation)
	fixture.inherited = control == "inherited"
	fixture.missingRemoval = control == "missing"
	writer := &annotationMemoryRefusedWriter{}
	if control == "failed-writer" {
		fixture.writer = writer
	}
	return fixture, writer
}

func requireAnnotationRefusedWriter(t *testing.T, fixture *annotationMemoryRun, writer *annotationMemoryRefusedWriter) {
	t.Helper()
	if writer.calls != 1 || fixture.failures != 1 {
		t.Fatalf("actual failure report not attempted once: calls=%d failures=%d", writer.calls, fixture.failures)
	}
	fixture.advisories.Report(writer)
	var later bytes.Buffer
	fixture.advisories.Report(&later)
	if writer.calls != 1 || later.Len() != 0 {
		t.Fatal("failed diagnostic escaped into a later invocation")
	}
}
