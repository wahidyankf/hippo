//go:build darwin || linux

package host

import (
	"errors"
	"syscall"
	"testing"
)

func TestPollDescriptorClassifiesTheReadinessItReports(t *testing.T) {
	ready, err := pollDescriptor(func(int, int) (int16, error) { return testPollInput, nil }, 3, 0)
	if !ready || err != nil {
		t.Fatalf("readable descriptor reported ready=%t error=%v", ready, err)
	}

	ready, err = pollDescriptor(func(int, int) (int16, error) { return 0, syscall.EIO }, 3, 0)
	if ready || !errors.Is(err, syscall.EIO) {
		t.Fatalf("failed poll reported ready=%t error=%v", ready, err)
	}
}
