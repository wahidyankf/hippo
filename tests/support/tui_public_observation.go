package support

import "github.com/wahidyankf/hippo/tests/contract"

// BindingsWithPublicTUIObservation retains the original neutral inventory.
// Replacement is passive, public-only and registered exactly once per original
// pattern; unimplemented families retain their original fail-closed handlers.
func (driver *Driver) BindingsWithPublicTUIObservation(factory func(string) any) []contract.StepBinding {
	bindings := driver.Bindings()
	if driver.mode != contract.E2E || factory == nil {
		return bindings
	}
	for index := range bindings {
		if handler := factory(bindings[index].Pattern); handler != nil {
			bindings[index].Handler = handler
		}
	}
	return bindings
}
