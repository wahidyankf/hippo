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

func TestLinuxWitnessVanishedDecision(t *testing.T) {
	for _, test := range []struct {
		name   string
		wait   error
		wpid   int
		status syscall.WaitStatus
		leader bool
		retire bool
	}{
		{"reaped clean clone exit retires detached", nil, 58740, syscall.WaitStatus(0), false, true},
		{"wait failure stays fail-closed", syscall.ECHILD, 0, syscall.WaitStatus(0), false, false},
		{"unreaped thread stays fail-closed", nil, 0, syscall.WaitStatus(0), false, false},
		{"reaped leader exit stays fail-closed", nil, 58740, syscall.WaitStatus(0), true, false},
		{"reaped failed exit stays fail-closed", nil, 58740, syscall.WaitStatus(1 << 8), false, false},
		{"reaped signaled thread stays fail-closed", nil, 58740, syscall.WaitStatus(syscall.SIGKILL), false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := linuxWitnessVanishedDecision(test.wait, test.wpid, test.status, test.leader); got != test.retire {
				t.Fatalf("vanished decision=%v, want %v", got, test.retire)
			}
		})
	}
}
