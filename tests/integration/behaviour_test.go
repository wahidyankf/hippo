package integration_test

import (
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

func TestIntegrationBehaviours(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.Integration)
	if err != nil {
		t.Fatal(err)
	}

	contract.Run(t, adapter, support.NewDriver(adapter))
}
