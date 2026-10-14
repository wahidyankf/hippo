package support

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"
	"testing"
)

const (
	witnessExitGroupAMD64 = 231
	witnessExitGroupARM64 = 94
	witnessFutexAMD64     = 202
	witnessExitAMD64      = 60
)

func witnessEntry(number, argument uint64) linuxSyscallInfo {
	return linuxSyscallInfo{Operation: 1, Architecture: linuxAuditAMD64, Data: [8]uint64{number, argument}}
}

func TestLinuxWitnessGroupExitRequest(t *testing.T) {
	exit := linuxSyscallInfo{Operation: 2, Architecture: linuxAuditAMD64, Data: [8]uint64{witnessExitGroupAMD64, 0}}
	for _, test := range []struct {
		name  string
		info  linuxSyscallInfo
		group uint64
		want  linuxWitnessGroupExit
	}{
		{"amd64 exit_group zero is a clean group exit", witnessEntry(witnessExitGroupAMD64, 0), witnessExitGroupAMD64, linuxWitnessGroupExitClean},
		{"arm64 exit_group zero is a clean group exit", linuxSyscallInfo{Operation: 1, Architecture: linuxAuditARM64, Data: [8]uint64{witnessExitGroupARM64, 0}}, witnessExitGroupARM64, linuxWitnessGroupExitClean},
		{"exit_group status is masked like the kernel", witnessEntry(witnessExitGroupAMD64, 256), witnessExitGroupAMD64, linuxWitnessGroupExitClean},
		{"exit_group nonzero is a failed group exit", witnessEntry(witnessExitGroupAMD64, 1), witnessExitGroupAMD64, linuxWitnessGroupExitFailed},
		{"exit_group high nonzero byte is a failed group exit", witnessEntry(witnessExitGroupAMD64, 0xff), witnessExitGroupAMD64, linuxWitnessGroupExitFailed},
		{"syscall exit stop is not a request", exit, witnessExitGroupAMD64, linuxWitnessGroupExitNone},
		{"single thread exit is not a group exit", witnessEntry(witnessExitAMD64, 0), witnessExitGroupAMD64, linuxWitnessGroupExitNone},
		{"unrelated syscall is not a request", witnessEntry(witnessFutexAMD64, 0), witnessExitGroupAMD64, linuxWitnessGroupExitNone},
		{"another architecture number is not a request", witnessEntry(witnessExitGroupARM64, 0), witnessExitGroupAMD64, linuxWitnessGroupExitNone},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := linuxWitnessGroupExitRequest(test.info, test.group); got != test.want {
				t.Fatalf("group exit request=%v, want %v", got, test.want)
			}
		})
	}
}

func TestLinuxWitnessGroupExitObserveIsSticky(t *testing.T) {
	const (
		none   = linuxWitnessGroupExitNone
		clean  = linuxWitnessGroupExitClean
		failed = linuxWitnessGroupExitFailed
	)
	for _, test := range []struct {
		name     string
		previous linuxWitnessGroupExit
		request  linuxWitnessGroupExit
		want     linuxWitnessGroupExit
	}{
		{"nothing observed stays nothing", none, none, none},
		{"first clean request is kept", none, clean, clean},
		{"later unrelated syscall keeps the clean request", clean, none, clean},
		{"a failed request after a clean one is failed", clean, failed, failed},
		{"a clean request after a failed one stays failed", failed, clean, failed},
		{"failed stays failed", failed, none, failed},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.previous.observe(test.request); got != test.want {
				t.Fatalf("observed group exit=%v, want %v", got, test.want)
			}
		})
	}
}

func TestLinuxWitnessGroupExitPresentation(t *testing.T) {
	for exit, want := range map[linuxWitnessGroupExit]string{
		linuxWitnessGroupExitNone:   "none",
		linuxWitnessGroupExitClean:  "clean",
		linuxWitnessGroupExitFailed: "failed",
		linuxWitnessGroupExit(200):  "unknown",
	} {
		if got := exit.String(); got != want {
			t.Fatalf("group exit text=%q, want %q", got, want)
		}
	}
	metadata := linuxWitnessFailureMetadata{Leader: 11, TID: 22, GroupExit: linuxWitnessGroupExitClean}
	if text := fmt.Sprintf("%+v", metadata); !strings.Contains(text, "GroupExit:clean") {
		t.Fatalf("failure metadata hides the observed group exit: %s", text)
	}
}

func TestLinuxWitnessGroupExitVanished(t *testing.T) {
	tagged := linuxWitnessTagOperation(linuxWitnessSyscallInfoKernel, 59230, syscall.ESRCH)
	for _, test := range []struct {
		name     string
		original error
		exit     linuxWitnessGroupExit
		vanished bool
	}{
		{"tagged ESRCH after clean group exit is explained", tagged, linuxWitnessGroupExitClean, true},
		{"bare ESRCH after clean group exit is explained", syscall.ESRCH, linuxWitnessGroupExitClean, true},
		{"joined ESRCH after clean group exit is explained", errors.Join(io.EOF, syscall.ESRCH), linuxWitnessGroupExitClean, true},
		{"ESRCH without an observed group exit stays fail-closed", tagged, linuxWitnessGroupExitNone, false},
		{"ESRCH after a failed group exit stays fail-closed", tagged, linuxWitnessGroupExitFailed, false},
		{"nil error is never a vanished thread", nil, linuxWitnessGroupExitClean, false},
		{"permission failure stays fail-closed", linuxWitnessTagOperation(linuxWitnessSyscallInfoKernel, 59230, syscall.EPERM), linuxWitnessGroupExitClean, false},
		{"I/O failure stays fail-closed", syscall.EIO, linuxWitnessGroupExitClean, false},
		{"wait failure stays fail-closed", syscall.ECHILD, linuxWitnessGroupExitClean, false},
		{"validation failure stays fail-closed", linuxWitnessTagOperation(linuxWitnessSyscallInfoValidation, 59230, errors.New("truncated")), linuxWitnessGroupExitClean, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := linuxWitnessGroupExitVanished(test.original, test.exit); got != test.vanished {
				t.Fatalf("group exit vanished=%v, want %v", got, test.vanished)
			}
		})
	}
}

func witnessStatus(state, threads string) []byte {
	return []byte("Name:\thippo\nUmask:\t0022\nState:\t" + state + "\nTgid:\t59230\nPid:\t59230\nPPid:\t59229\nThreads:\t" + threads + "\nSigQ:\t0/62\n")
}

func TestLinuxWitnessTaskSnapshotOf(t *testing.T) {
	for _, test := range []struct {
		name   string
		status []byte
		want   linuxWitnessTaskSnapshot
	}{
		{"lone zombie leader", witnessStatus("Z (zombie)", "1"), linuxWitnessTaskSnapshot{linuxWitnessTaskZombie, 1}},
		{"zombie leader with live siblings", witnessStatus("Z (zombie)", "7"), linuxWitnessTaskSnapshot{linuxWitnessTaskZombie, 7}},
		{"tracing stop", witnessStatus("t (tracing stop)", "3"), linuxWitnessTaskSnapshot{linuxWitnessTaskStopped, 3}},
		{"signal stop", witnessStatus("T (stopped)", "3"), linuxWitnessTaskSnapshot{linuxWitnessTaskStopped, 3}},
		{"running", witnessStatus("R (running)", "2"), linuxWitnessTaskSnapshot{linuxWitnessTaskLive, 2}},
		{"sleeping", witnessStatus("S (sleeping)", "2"), linuxWitnessTaskSnapshot{linuxWitnessTaskLive, 2}},
		{"disk sleep", witnessStatus("D (disk sleep)", "2"), linuxWitnessTaskSnapshot{linuxWitnessTaskLive, 2}},
		{"dead is not accepted as a zombie", witnessStatus("X (dead)", "1"), linuxWitnessTaskSnapshot{linuxWitnessTaskUnknown, 1}},
		{"lowercase dead is unknown", witnessStatus("x (dead)", "1"), linuxWitnessTaskSnapshot{linuxWitnessTaskUnknown, 1}},
		{"unlisted state letter is unknown", witnessStatus("Q (mystery)", "1"), linuxWitnessTaskSnapshot{linuxWitnessTaskUnknown, 1}},
		{"empty state value is unknown", witnessStatus("", "1"), linuxWitnessTaskSnapshot{linuxWitnessTaskUnknown, 1}},
		{"missing thread count is zero", []byte("Name:\thippo\nState:\tZ (zombie)\n"), linuxWitnessTaskSnapshot{linuxWitnessTaskZombie, 0}},
		{"garbled thread count is zero", witnessStatus("Z (zombie)", "many"), linuxWitnessTaskSnapshot{linuxWitnessTaskZombie, 0}},
		{"negative thread count is zero", witnessStatus("Z (zombie)", "-1"), linuxWitnessTaskSnapshot{linuxWitnessTaskZombie, 0}},
		{"zero thread count is zero", witnessStatus("Z (zombie)", "0"), linuxWitnessTaskSnapshot{linuxWitnessTaskZombie, 0}},
		{"missing state is unknown", []byte("Name:\thippo\nThreads:\t1\n"), linuxWitnessTaskSnapshot{linuxWitnessTaskUnknown, 1}},
		{"empty status is unknown", nil, linuxWitnessTaskSnapshot{}},
		{"state text inside the name never counts", []byte("Name:\tx State:\tZ (zombie)\nState:\tS (sleeping)\nThreads:\t2\n"), linuxWitnessTaskSnapshot{linuxWitnessTaskLive, 2}},
		{"first state line wins", []byte("State:\tS (sleeping)\nState:\tZ (zombie)\nThreads:\t1\n"), linuxWitnessTaskSnapshot{linuxWitnessTaskLive, 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := linuxWitnessTaskSnapshotOf(test.status); got != test.want {
				t.Fatalf("snapshot=%+v, want %+v", got, test.want)
			}
		})
	}
}

func TestLinuxWitnessLeaderHarvestAction(t *testing.T) {
	const (
		retire = linuxWitnessLeaderRetire
		idle   = linuxWitnessLeaderIdle
		wait   = linuxWitnessLeaderWait
	)
	zombie := func(threads int) linuxWitnessTaskSnapshot {
		return linuxWitnessTaskSnapshot{state: linuxWitnessTaskZombie, threads: threads}
	}
	live := linuxWitnessTaskSnapshot{state: linuxWitnessTaskLive, threads: 2}
	stopped := linuxWitnessTaskSnapshot{state: linuxWitnessTaskStopped, threads: 2}
	unknown := linuxWitnessTaskSnapshot{state: linuxWitnessTaskUnknown, threads: 1}
	for _, test := range []struct {
		name     string
		exit     linuxWitnessGroupExit
		snapshot linuxWitnessTaskSnapshot
		want     linuxWitnessLeaderAction
	}{
		{"clean group exit then lone zombie leader retires unreaped", linuxWitnessGroupExitClean, zombie(1), retire},
		{"clean group exit then zombie leader with siblings is left alone", linuxWitnessGroupExitClean, zombie(2), idle},
		{"clean group exit then zombie with unknown thread count stays fail-closed", linuxWitnessGroupExitClean, zombie(0), wait},
		{"clean group exit then a running leader is left alone", linuxWitnessGroupExitClean, live, idle},
		{"clean group exit then a stop is taken normally", linuxWitnessGroupExitClean, stopped, wait},
		{"clean group exit then an unknown state stays fail-closed", linuxWitnessGroupExitClean, unknown, wait},
		{"clean group exit then an out-of-range state stays fail-closed", linuxWitnessGroupExitClean, linuxWitnessTaskSnapshot{state: 200, threads: 1}, wait},
		{"unexplained lone zombie leader stays fail-closed", linuxWitnessGroupExitNone, zombie(1), wait},
		{"lone zombie leader after a failed group exit stays fail-closed", linuxWitnessGroupExitFailed, zombie(1), wait},
		{"out-of-range group exit never retires a zombie leader", linuxWitnessGroupExit(200), zombie(1), wait},
		{"unexplained running leader keeps the normal wait", linuxWitnessGroupExitNone, live, wait},
		{"failed group exit keeps the normal wait", linuxWitnessGroupExitFailed, live, wait},
		{"unexplained stop keeps the normal wait", linuxWitnessGroupExitNone, stopped, wait},
		{"unexplained unknown state keeps the normal wait", linuxWitnessGroupExitNone, unknown, wait},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := linuxWitnessLeaderHarvestAction(test.exit, test.snapshot); got != test.want {
				t.Fatalf("leader harvest action=%v, want %v", got, test.want)
			}
		})
	}
}
