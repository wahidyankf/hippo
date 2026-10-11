package tui

import (
	"context"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/tests/contract"
)

// NewBDDTestDriver exposes only the test driver, never production model or framework types.
func NewBDDTestDriver(adapter contract.Adapter) *BDDTestDriver {
	return &BDDTestDriver{adapter: adapter}
}

// SetGuardedJobFactory connects the separate support component without making
// the production TUI import its bootstrap-based test support.
func (driver *BDDTestDriver) SetGuardedJobFactory(factory func(context.Context) (func() error, func() error, error)) {
	driver.guardFactory = factory
}

func (driver *BDDTestDriver) SetObservationFactory(factory func(context.Context, string) (application.ObservationSnapshot, error)) {
	driver.observationFactory = factory
}

// SetNativeStepFactory replaces handlers within the original single inventory.
// Unit keeps its exact handlers. Constructing a factory must not start a child;
// each native preparation step owns its own runtime and bounded cleanup.
func (driver *BDDTestDriver) SetNativeStepFactory(factory func(string) any, closeSession func()) {
	driver.nativeStepFactory, driver.nativeClose = factory, closeSession
}
