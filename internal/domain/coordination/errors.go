package coordination

import "errors"

// ErrCoordinationDeferred indicates bounded lock contention during admission.
var (
	ErrCoordinationDeferred = errors.New("shared coordination deferred admission")
	// ErrCoordinationCleanupDeferred indicates a reconcilable deferred release.
	ErrCoordinationCleanupDeferred = errors.New("shared coordination deferred ownership cleanup")
)
