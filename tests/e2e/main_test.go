package e2e_test

import (
	"os"
	"testing"

	"github.com/wahidyankf/hippo/tests/support"
)

// TestMain chooses a test-stamped binary of this working tree, then keeps
// inherited HIPPO_ variables and the shared evidence root away from it; see
// support.RunCompiled and support.RunIsolated.
func TestMain(m *testing.M) { os.Exit(support.RunCompiled(m)) }
