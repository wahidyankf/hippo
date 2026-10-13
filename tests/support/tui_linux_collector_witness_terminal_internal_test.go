package support

import (
	"syscall"
	"testing"
)

func TestLinuxWitnessTerminalRetire(t *testing.T) {
	for _, test := range []struct {
		name   string
		leader bool
		status syscall.WaitStatus
		retire bool
	}{
		{"clean clone exit retires detached", false, syscall.WaitStatus(0), true},
		{"leader exit stays fail-closed", true, syscall.WaitStatus(0), false},
		{"failed clone exit stays fail-closed", false, syscall.WaitStatus(1 << 8), false},
		{"killed clone exit stays fail-closed", false, syscall.WaitStatus(syscall.SIGKILL), false},
		{"killed leader exit stays fail-closed", true, syscall.WaitStatus(syscall.SIGKILL), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := linuxWitnessTerminalRetire(test.leader, test.status); got != test.retire {
				t.Fatalf("terminal retire=%v, want %v", got, test.retire)
			}
		})
	}
}
