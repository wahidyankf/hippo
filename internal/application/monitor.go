package application

import (
	"context"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// MonitorRequest describes the observation interval and resource-policy selection.
type MonitorRequest struct {
	ConfigPath, DiskPath string
	RequestedProfile     policy.ProfileName
	Interval             time.Duration
}

// MonitorTransition is a portable schema-1 resource transition.
type MonitorTransition struct {
	SchemaVersion int                `json:"schemaVersion"`
	MeasuredAt    string             `json:"measuredAt"`
	State         policy.State       `json:"state"`
	Reason        string             `json:"reason"`
	Profile       policy.ProfileName `json:"profile"`
	SwapState     string             `json:"swapState"`
}

// Monitor samples continuously and emits only changed resource transitions.
func (services ObservationServices) Monitor(ctx context.Context, request MonitorRequest, emit func(MonitorTransition) error) (int, error) {
	if request.Interval <= 0 {
		return 0, status.Fail(status.CodeArgsInvalid, "interval must be positive")
	}

	configuration, configError := services.Configuration.Load(request.ConfigPath, services.Environment)
	if configError != nil {
		return 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
	}

	var previous policy.CPUState

	samples := []policy.Sample{}
	prior := ""

	observe := func() error {
		reading, err := services.Collector.Collect(ctx, previous, request.DiskPath)
		if err != nil {
			return err
		}

		previous = reading.CPUState
		samples = append(samples, reading.Sample)
		if len(samples) > 17 {
			samples = samples[len(samples)-17:]
		}

		resolution, resolveError := configuration.Catalog.Resolve(request.RequestedProfile, policy.TaskEphemeral, reading.Sample)
		if resolveError != nil {
			return resolveError
		}

		assessment := policy.ResourceAssessment(samples, resolution.Policy)
		state := string(assessment.State) + ":" + assessment.Reason + ":" + string(resolution.ResolvedProfile)

		if state != prior {
			if err := emit(MonitorTransition{SchemaVersion: 1, MeasuredAt: reading.Sample.MeasuredAt, State: assessment.State, Reason: assessment.Reason, Profile: resolution.ResolvedProfile, SwapState: reading.Sample.SwapState}); err != nil {
				return err
			}

			prior = state
		}

		return nil
	}

	if err := observe(); err != nil {
		if stoppedByCaller(ctx, err) {
			return 0, nil
		}

		return 1, err
	}

	ticker := services.Clock.Ticker(request.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return 0, nil
		case <-ticker.Ticks:
			// Cancellation and a tick routinely become ready together, because the
			// interval is short and a loaded host leaves this loop unscheduled across
			// both. Go then selects uniformly, and servicing the tick collects through
			// an already-cancelled context, which turns a deliberate stop into a
			// monitoring failure. Stopping is what the caller asked for, so it wins.
			select {
			case <-ctx.Done():
				return 0, nil
			default:
			}

			if err := observe(); err != nil {
				// A stop can also land while a probe is running, and a probe
				// its cancelled context killed reports its own error.
				if stoppedByCaller(ctx, err) {
					return 0, nil
				}

				return 1, err
			}
		}
	}
}

// admissionWaitConflict refuses a wait that could only be ignored. The flag
// bounds the schema-2 FIFO queue. Schema 1 has no such queue — its exclusive
// lease waits as long as the resolved profile says — a tier carries its own
// queue deadline, and schema 3 always uses one, so a --wait-for-admission
// under any of them would silently wait for a deadline the caller did not ask
// for.
