package bdd_test

import (
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
)

// @scenario specs/behaviours/admission.feature: *
// @scenario specs/behaviours/artifacts.feature: *
// @scenario specs/behaviours/conformance.feature: *
// @scenario specs/behaviours/evidence.feature: *
// @scenario specs/behaviours/execution.feature: *
// @scenario specs/behaviours/portability.feature: *
// @scenario specs/behaviours/public-cli.feature: *
// @scenario specs/behaviours/release.feature: *
// @scenario specs/behaviours/reservations.feature: *
// @scenario specs/behaviours/resource-tiers.feature: *
// @scenario specs/behaviours/terminal.feature: *

func TestUnitBehaviours(t *testing.T) { runBehaviours(t, contract.Unit) }
