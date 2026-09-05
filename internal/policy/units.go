package policy

import (
	"errors"
	"math"
)

// MiBToBytes converts an untrusted MiB count without allowing signed overflow.
func MiBToBytes(value int64) (int64, error) {
	if value < 0 || value > math.MaxInt64/MiB {
		return 0, errors.New("MiB value exceeds representable bytes")
	}

	return value * MiB, nil
}
