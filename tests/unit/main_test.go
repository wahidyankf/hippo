package unit_test

import (
	"os"
	"testing"

	"github.com/wahidyankf/hippo/tests/support"
)

// TestMain keeps inherited HIPPO_ variables and the shared evidence root away
// from the binary under test; see support.RunIsolated.
func TestMain(m *testing.M) { os.Exit(support.RunIsolated(m)) }
