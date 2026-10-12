//go:build darwin

package integration_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type hostReaderIdentity int

type readerProcessMetadata struct {
	parent     int
	executable string
}

func contextReaderProcesses(t *testing.T) map[int]readerProcessMetadata {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	data, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,ppid=,comm=").Output()
	if err != nil {
		t.Fatalf("owned reader metadata query: %v", err)
	}
	processes := map[int]readerProcessMetadata{}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, pidError := strconv.Atoi(fields[0])
		parent, parentError := strconv.Atoi(fields[1])
		if pidError != nil || parentError != nil {
			t.Fatal("invalid process metadata identifiers")
		}
		processes[pid] = readerProcessMetadata{parent: parent, executable: strings.Join(fields[2:], " ")}
	}
	return processes
}

func contextReaderBaseline(t *testing.T) map[int]bool {
	t.Helper()
	baseline := map[int]bool{}
	for pid, process := range contextReaderProcesses(t) {
		if process.parent == os.Getpid() && filepath.Base(process.executable) == "cat" {
			baseline[pid] = true
		}
	}
	if len(baseline) != 0 {
		t.Fatal("existing reader child makes isolated fixture ambiguous")
	}
	return baseline
}

func waitHostReaderAcquisition(t *testing.T, _ string, finished chan hostReadResult, baseline map[int]bool) hostReaderIdentity {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		candidates := []int{}
		for pid, process := range contextReaderProcesses(t) {
			if process.parent == os.Getpid() && filepath.Base(process.executable) == "cat" && !baseline[pid] {
				candidates = append(candidates, pid)
			}
		}
		if len(candidates) > 1 {
			t.Fatal("multiple owned reader children make acknowledgement ambiguous")
		}
		if len(candidates) == 1 {
			return hostReaderIdentity(candidates[0])
		}
		select {
		case result := <-finished:
			finished <- result
			t.Fatalf("writerless reader ended before child-start acknowledgement: %q/%v", result.data, result.err)
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("owned Darwin reader child never acknowledged")
		}
	}
}

func assertHostReaderRetired(t *testing.T, identity hostReaderIdentity) {
	t.Helper()
	if _, exists := contextReaderProcesses(t)[int(identity)]; exists {
		t.Error("owned reader PID remains after joined factory return")
	}
}
