package integration_test

import (
	"fmt"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

func TestAnnotationNativeFaultsKeepChildStreamsStatusAllocationAndRelease(t *testing.T) {
	for _, operation := range []string{"write", "removal"} {
		for _, reservation := range []bool{false, true} {
			for _, childExit := range []int{0, 37} {
				for _, jsonMode := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s-reservation%t-exit%d-json%t", operation, reservation, childExit, jsonMode), func(t *testing.T) {
						if err := support.CheckAnnotationNativeRun(t.Context(), contract.Adapter{Name: contract.Integration}, support.AnnotationNativeCase{Operation: operation, Reservation: reservation, ChildExit: childExit, JSON: jsonMode}); err != nil {
							t.Fatal(err)
						}
					})
				}
			}
		}
	}
}

func TestAnnotationNativeHealthyMissingAndBothOperations(t *testing.T) {
	for _, operation := range []string{"healthy", "missing", "both"} {
		for _, reservation := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-reservation%t", operation, reservation), func(t *testing.T) {
				if err := support.CheckAnnotationNativeRun(t.Context(), contract.Adapter{Name: contract.Integration}, support.AnnotationNativeCase{Operation: operation, Reservation: reservation, ChildExit: 37, JSON: true}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestAnnotationNativeFailedDiagnosticWriterKeepsActualChildOutcome(t *testing.T) {
	for _, operation := range []string{"write", "removal"} {
		for _, reservation := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-reservation%t", operation, reservation), func(t *testing.T) {
				if err := support.CheckAnnotationNativeRun(t.Context(), contract.Adapter{Name: contract.Integration}, support.AnnotationNativeCase{Operation: operation, Reservation: reservation, ChildExit: 37, FailDiagnosticWriter: true}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
