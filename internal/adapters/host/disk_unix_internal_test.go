//go:build darwin || linux

package host

import (
	"math"
	"testing"
)

func TestBlockCapacityConvertsPlausibleCounts(t *testing.T) {
	free, total, err := blockCapacity(int64(4096), 3, 10)
	if err != nil || *free != 3*4096 || *total != 10*4096 {
		t.Fatalf("signed block size: free=%v total=%v error=%v", free, total, err)
	}
	free, total, err = blockCapacity(uint32(512), 1, 2)
	if err != nil || *free != 512 || *total != 1024 {
		t.Fatalf("unsigned block size: free=%v total=%v error=%v", free, total, err)
	}
}

func TestBlockCapacityRefusesInvalidAndOverflowingCounts(t *testing.T) {
	for _, test := range []struct {
		name      string
		blockSize int64
		available uint64
		blocks    uint64
	}{
		{"zero block size", 0, 1, 1},
		{"negative block size", -4096, 1, 1},
		{"available bytes overflow", 4096, math.MaxInt64, 1},
		{"total bytes overflow", 4096, 1, math.MaxInt64},
	} {
		if _, _, err := blockCapacity(test.blockSize, test.available, test.blocks); err == nil {
			t.Fatalf("%s was accepted", test.name)
		}
	}
}
