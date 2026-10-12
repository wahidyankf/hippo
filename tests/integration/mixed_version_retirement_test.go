//go:build mixedversion

package integration_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMixedVersionOwnerRetirementIdentity(t *testing.T) {
	mixedV085OwnerRetirementIdentity(t)
}

func mixedV085OwnerRetirementIdentity(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	zero := exec.CommandContext(ctx, "/bin/sh", "-c", "exit 0")
	if err := zero.Run(); err != nil {
		t.Fatal(err)
	}
	nonzero := exec.CommandContext(ctx, "/bin/sh", "-c", "exit 130")
	waitError := nonzero.Run()
	if waitError == nil || nonzero.ProcessState == nil || nonzero.ProcessState.ExitCode() != 130 {
		t.Fatalf("nonzero control lacks its actual joined exit-130 receipt: %v", waitError)
	}
	queryError := errors.New("probe failed")
	for _, test := range []struct {
		name       string
		pid        int
		queryError error
		queries    int
		message    string
		cause      error
		command    *exec.Cmd
		waitError  error
	}{
		{name: "zero identity unknown", queries: 0, message: "mixed payload retirement UNKNOWN: payload PID unobserved", command: zero},
		{name: "negative identity unknown", pid: -42, queries: 0, message: "mixed payload retirement UNKNOWN: payload PID unobserved", command: zero},
		{name: "identified absent", pid: 42, queryError: syscall.ESRCH, queries: 1, command: zero},
		{name: "identified live", pid: 42, queries: 1, message: "mixed payload survived actual joined Wait", command: zero},
		{name: "permission unknown", pid: 42, queryError: syscall.EPERM, queries: 1, message: "mixed payload retirement UNKNOWN: inspect PID 42", cause: syscall.EPERM, command: zero},
		{name: "probe error unknown", pid: 42, queryError: queryError, queries: 1, message: "mixed payload retirement UNKNOWN: inspect PID 42", cause: queryError, command: zero},
		{name: "nonzero owner absent payload", pid: 42, queryError: syscall.ESRCH, queries: 1, message: "mixed owner did not naturally exit zero: exit status 130", command: nonzero, waitError: waitError},
		{name: "nonzero owner unobserved payload", queries: 0, message: "mixed owner did not naturally exit zero: exit status 130", command: nonzero, waitError: waitError},
	} {
		t.Run(test.name, func(t *testing.T) {
			queries := 0
			job := &mixedV085Owner{command: test.command, payloadPID: test.pid, waitError: test.waitError}
			err := mixedV085OwnerRetirement(job, func(pid int) error {
				queries++
				if pid != test.pid || pid <= 0 {
					t.Errorf("signal query used an unidentified payload PID: %d", pid)
				}
				return test.queryError
			})
			if queries != test.queries {
				t.Errorf("signal queries=%d, want %d", queries, test.queries)
			}
			mixedV085RequireRetirementDiagnostic(t, err, test.message, test.cause, test.pid <= 0 || test.cause != nil)
		})
	}
}

func mixedV085RequireRetirementDiagnostic(t *testing.T, err error, message string, cause error, unknown bool) {
	t.Helper()
	if message == "" {
		if err != nil {
			t.Fatalf("identified absence after natural zero exit rejected: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), message) {
		t.Fatalf("retirement=%v, want %q", err, message)
	}
	if cause != nil && !errors.Is(err, cause) {
		t.Errorf("query cause was lost: %v", err)
	}
	if unknown && strings.Contains(err.Error(), "mixed payload survived") {
		t.Errorf("unproven payload identity reported as survival: %v", err)
	}
}
