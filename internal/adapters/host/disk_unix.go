//go:build darwin || linux

package host

import (
	"errors"
	"math"

	"golang.org/x/sys/unix"
)

func diskCapacity(path string) (*int64, *int64, error) {
	var value unix.Statfs_t
	if err := unix.Statfs(path, &value); err != nil {
		return nil, nil, err
	}

	return blockCapacity(value.Bsize, value.Bavail, value.Blocks)
}

// blockCapacity converts statfs block counts into byte capacities. A block size that is not positive is invalid, and
// a product that would overflow int64 is refused rather than wrapped. The block size type differs by platform.
func blockCapacity[Size int32 | int64 | uint32](blockSize Size, available, blocks uint64) (*int64, *int64, error) {
	if blockSize <= 0 {
		return nil, nil, errors.New("disk block size is invalid")
	}

	size := uint64(blockSize)
	if available > math.MaxInt64/size || blocks > math.MaxInt64/size {
		return nil, nil, errors.New("disk capacity exceeds supported range")
	}

	free := int64(available * size) //nolint:gosec // Multiplication is bounded above by math.MaxInt64.
	total := int64(blocks * size)   //nolint:gosec // Multiplication is bounded above by math.MaxInt64.

	return &free, &total, nil
}
