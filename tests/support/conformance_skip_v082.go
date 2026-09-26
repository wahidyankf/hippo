package support

import (
	"errors"
	"strings"
)

// requireConformanceCapacitySkip binds the one outcome an allow-capacity-skip
// check may pass with: exit 124 naming hippo.limit.capacity-deferred, beside a
// never-started receipt the check wrote. The compiled runner must skip it, and
// the classifier must accept the exact bytes a current guard writes for it.
func requireConformanceCapacitySkip(string) error {
	return errors.Join(
		runGoRegressionV10("./tests/integration", "TestCompiledConformanceCapacitySkipRequiresNewNeverStartedReceipt/verified"),
		runGoRegressionV10("./internal/conformance", "TestCleanCapacitySkipMatchesWhatHippoEmits"),
	)
}

// requireConformancePressureShedFatal binds a pressure shed to staying fatal.
// It shares exit 124 with a capacity deferral, but its child started, so it
// is never a skip. With a never-started receipt beside it, only the reason can
// refuse it, which is what makes that example the one that proves the reason
// is read.
func requireConformancePressureShedFatal(receipt string) error {
	subtest := "without_receipt"
	if strings.HasPrefix(receipt, "beside") {
		subtest = "with_receipt"
	}

	return runGoRegressionV10("./tests/integration", "TestCompiledConformancePressureShedIsNotCapacitySkip/"+subtest)
}
