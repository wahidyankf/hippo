package application

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/wahidyankf/hippo/internal/status"
)

// Watch observes status and emits only changed presentation projections.
func (services ObservationServices) Watch(ctx context.Context, request StatusRequest, interval time.Duration, project func(StatusView) ([]byte, error), emit func([]byte) error) (int, error) {
	if interval <= 0 {
		return 0, status.Fail(status.CodeArgsInvalid, "interval must be positive")
	}
	var prior []byte
	for {
		view, code, err := services.Observe(ctx, request)
		if stoppedByCaller(ctx, err) {
			return 0, nil
		}
		if err != nil || code != 0 {
			return code, err
		}
		projected, err := project(view)
		if err != nil {
			return 1, err
		}
		if !bytes.Equal(projected, prior) {
			if err := emit(projected); err != nil {
				return 1, err
			}
			prior = projected
		}
		if err := services.Clock.Wait(ctx, interval); err != nil {
			if stoppedByCaller(ctx, err) {
				return 0, nil
			}
			return 1, err
		}
	}
}

// stoppedByCaller reports whether an observer loop failed only because its
// caller cancelled it. watch and monitor run until they are stopped, so a
// stop is their end wherever it lands: while waiting, or while a sample is
// being collected. The entry point turns a signal-caused stop into 128+N; a
// failure hippo classified while stopping is still its own answer.
func stoppedByCaller(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() == nil {
		return false
	}
	_, classified := errors.AsType[status.Failure](err)

	return !classified
}
