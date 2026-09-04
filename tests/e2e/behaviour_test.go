package e2e_test

import (
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

func TestE2EBehaviours(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.E2E)
	if err != nil {
		t.Fatal(err)
	}

	contract.Run(t, adapter, support.NewDriver(adapter))
}
