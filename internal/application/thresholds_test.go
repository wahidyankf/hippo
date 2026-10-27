package application_test

import (
	"context"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/policy"
)

// TestObservationAppliesCoordinationThresholds proves the status path resolves
// the profile and then applies the coordination knobs: a floor above the
// available memory warns where the legacy bound stays normal, and the resolved
// policy carries both knobs.
func TestObservationAppliesCoordinationThresholds(t *testing.T) {
	fixture := newObservationFixture()
	fixture.memory = policy.MemoryPolicy{WarningPercent: 50, WarningFloorBytes: 25 * policy.GiB}
	fixture.swapMultiple = 2
	view, code, err := observationServices(fixture).Observe(context.Background(), application.StatusRequest{DiskPath: "disk", Source: "hippo"})
	if err != nil || code != 0 {
		t.Fatalf("code=%d error=%v", code, err)
	}
	if view.Resource.State != policy.StateWarning || view.Resource.Reason != "memory-warning" {
		t.Fatalf("state %q reason %q, want warning memory-warning", view.Resource.State, view.Resource.Reason)
	}
	if view.Profile.Policy.WarningMemoryPercent != 50 ||
		view.Profile.Policy.WarningMemoryFloorBytes != 25*policy.GiB ||
		view.Profile.Policy.SwapUsedMaxMultiple != 2 {
		t.Fatalf("resolved policy lost the coordination knobs: %+v", view.Profile.Policy)
	}
}

// TestObservationKeepsLegacyThresholdsWithoutKnobs proves a coordination
// without knobs observes the legacy bounds on the same sample.
func TestObservationKeepsLegacyThresholdsWithoutKnobs(t *testing.T) {
	fixture := newObservationFixture()
	view, code, err := observationServices(fixture).Observe(context.Background(), application.StatusRequest{DiskPath: "disk", Source: "hippo"})
	if err != nil || code != 0 {
		t.Fatalf("code=%d error=%v", code, err)
	}
	if view.Resource.State != policy.StateNormal || view.Resource.Reason != "normal" {
		t.Fatalf("state %q reason %q, want a normal admission", view.Resource.State, view.Resource.Reason)
	}
}
