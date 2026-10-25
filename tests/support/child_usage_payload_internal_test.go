package support

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// helperPollLimit bounds the wait for the payload's helper to appear. Reaching
// it fails the test with a diagnostic instead of hanging.
const helperPollLimit = 10 * time.Second

// processRow is the part of one process-table row this test reads.
type processRow struct {
	pid, parent, group int
	name               string
}

// processRows lists every process in the table.
func processRows(t *testing.T) []processRow {
	t.Helper()
	output, err := exec.Command("/bin/ps", "-A", "-o", "pid=", "-o", "ppid=", "-o", "pgid=", "-o", "comm=").Output()
	if err != nil {
		t.Fatalf("list processes: %v", err)
	}
	var rows []processRow
	for line := range strings.SplitSeq(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, pidError := strconv.Atoi(fields[0])
		parent, parentError := strconv.Atoi(fields[1])
		group, groupError := strconv.Atoi(fields[2])
		if pidError != nil || parentError != nil || groupError != nil {
			continue
		}
		rows = append(rows, processRow{pid: pid, parent: parent, group: group, name: filepath.Base(fields[3])})
	}

	return rows
}

// The shed scenario promises a lower bound: its helper's CPU is never in the
// run's recorded usage, because the guard waits for the shell and never for
// the helper. A helper the shell itself started would break that promise
// whenever the helper died while the shell was still alive, since the shell
// would reap it and its CPU would travel into the shell's own usage. That
// happens when a group signal reaches the helper before the shell, which a
// loaded host makes common. So the helper must never be the shell's child.
func TestUnreapedRoundHelperIsNotTheShellsChild(t *testing.T) {
	shell := exec.Command(shellPath, "-c", unreapedRound)
	shell.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := shell.Start(); err != nil {
		t.Fatal(err)
	}
	group := shell.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-group, syscall.SIGKILL)
		_ = shell.Wait()
	})

	deadline := time.Now().Add(helperPollLimit)
	for time.Now().Before(deadline) {
		for _, row := range processRows(t) {
			if row.group == group && row.name == filepath.Base(cpuBusyHelper) {
				if row.parent == group {
					t.Fatalf("the helper %+v is the shell's child, so the shell reaps its CPU when it dies first", row)
				}

				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no helper appeared in the payload's process group %d within %s", group, helperPollLimit)
}
