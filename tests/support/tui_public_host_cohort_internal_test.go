package support

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func publicHostNextRequest(index, pid, parent int) publicHostRequest {
	request := publicHostProtocolRequest()
	request.PID, request.ParentPID = pid, parent
	request.Name = publicHostDarwinCommands[index][0]
	request.Arguments = publicHostDarwinCommands[index][1:]
	return request
}

func TestPublicHostCohortRetirementPreservesHistoryAndAllowsNextOwner(t *testing.T) {
	probe := publicHostAcknowledgedPrefix(t)
	probe.platform = darwinPlatform
	second := publicHostNextRequest(1, 29, 23)
	if _, _, err := probe.acceptDarwinLocked(second); err != nil {
		t.Fatal(err)
	}
	probe.completeDarwin(1, second, "output-ack")
	if _, _, err := probe.acceptDarwinLocked(publicHostNextRequest(0, 31, 37)); err == nil {
		t.Fatal("genuine attempted overlap was accepted")
	}
	before := probe.Trace()
	var inspected []int
	if err := probe.retireDarwinReading(t.Context(), 1, 23, func(pid int) error {
		inspected = append(inspected, pid)
		return syscall.ESRCH
	}); err != nil {
		t.Fatal("absent incomplete cohort refused", err)
	}
	after := probe.Trace()
	if !reflect.DeepEqual(inspected, []int{23, 17, 29}) || after.Active != 0 || after.Readings != 2 || after.MaximumActive != 2 ||
		!reflect.DeepEqual(before.Events, after.Events[:len(before.Events)]) || len(after.Events) != len(before.Events)+1 ||
		after.Events[len(before.Events)].Phase != "retired-incomplete" || after.Events[len(before.Events)].ParentPID != 23 ||
		after.Events[len(before.Events)].Reading != 1 || probe.commandIndex != 0 {
		t.Fatalf("retirement erased history or omitted retained identities: %+v inspected=%v", after, inspected)
	}
	if reading, _, err := probe.acceptDarwinLocked(publicHostNextRequest(0, 31, 37)); err != nil || reading != 3 {
		t.Fatalf("next owned reading refused after retirement: %d %v", reading, err)
	}
	if probe.trace.MaximumActive != 2 {
		t.Fatal("next owner erased original attempted overlap")
	}
}

func TestPublicHostCohortRetirementRefusesUnprovenStateBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*PublicTUIHostProbe, *int, *int)
		status  func(int) error
		expired bool
	}{
		{"wrong-reading", func(_ *PublicTUIHostProbe, reading, _ *int) { *reading = 2 }, nil, false},
		{"wrong-parent", func(_ *PublicTUIHostProbe, _, parent *int) { *parent = 31 }, nil, false},
		{"non-Darwin", func(probe *PublicTUIHostProbe, _, _ *int) { probe.platform = linuxPlatform }, nil, false},
		{"closed", func(probe *PublicTUIHostProbe, _, _ *int) { probe.closing = true }, nil, false},
		{"completed", func(probe *PublicTUIHostProbe, _, _ *int) { probe.trace.Active = 0 }, nil, false},
		{"pending-request", func(probe *PublicTUIHostProbe, _, _ *int) {
			request := publicHostNextRequest(1, 29, 23)
			probe.darwinPending = &request
		}, nil, false},
		{"pending-handler", func(probe *PublicTUIHostProbe, _, _ *int) { probe.connections = map[net.Conn]int{nil: 17} }, nil, false},
		{"live-parent", nil, func(pid int) error {
			if pid == 23 {
				return nil
			}
			return syscall.ESRCH
		}, false},
		{"live-helper-or-reused-PID", nil, func(pid int) error {
			if pid == 17 {
				return nil
			}
			return syscall.ESRCH
		}, false},
		{"permission-error", nil, func(int) error { return syscall.EPERM }, false},
		{"unknown-error", nil, func(int) error { return errors.New("unknown process state") }, false},
		{"expired", nil, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := publicHostAcknowledgedPrefix(t)
			probe.platform = darwinPlatform
			reading, parent := 1, 23
			if test.change != nil {
				test.change(probe, &reading, &parent)
			}
			before := probe.Trace()
			ctx := t.Context()
			if test.expired {
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer stop()
			}
			status := test.status
			if status == nil {
				status = func(int) error { return syscall.ESRCH }
			}
			if err := probe.retireDarwinReading(ctx, reading, parent, status); err == nil {
				t.Fatal("unproven cohort was retired")
			}
			if !reflect.DeepEqual(before, probe.Trace()) || probe.commandIndex != 1 || probe.reading != 1 {
				t.Fatal("refused retirement changed counters or original prefix")
			}
		})
	}
}

func publicHostAcknowledgedPrefix(t *testing.T) *PublicTUIHostProbe {
	t.Helper()
	probe := publicHostProtocolFixture()
	request := publicHostProtocolRequest()
	reading, _, err := probe.acceptDarwinLocked(request)
	if err != nil {
		t.Fatal(err)
	}
	probe.completeDarwin(reading, request, "output-ack")
	return probe
}

func TestPublicHostCohortRefusesForeignContinuationBeforeMutation(t *testing.T) {
	probe := publicHostAcknowledgedPrefix(t)
	before := probe.Trace()
	if _, _, err := probe.acceptDarwinLocked(publicHostNextRequest(1, 29, 31)); err == nil {
		t.Fatal("foreign parent continued the original acknowledged reading")
	}
	if !reflect.DeepEqual(before, probe.Trace()) || probe.commandIndex != 1 || probe.reading != 1 {
		t.Fatal("foreign continuation mutated the original reading")
	}
}

func TestPublicHostCohortRefusesForeignCompletionBeforeMutation(t *testing.T) {
	for _, change := range []struct {
		name   string
		mutate func(*publicHostRequest)
	}{
		{"parent", func(request *publicHostRequest) { request.ParentPID++ }},
		{"helper", func(request *publicHostRequest) { request.PID++ }},
		{"nonce", func(request *publicHostRequest) { request.Nonce = "foreign" }},
		{"argv", func(request *publicHostRequest) { request.Arguments = []string{"foreign"} }},
	} {
		t.Run(change.name, func(t *testing.T) {
			probe := publicHostProtocolFixture()
			request := publicHostProtocolRequest()
			reading, _, err := probe.acceptDarwinLocked(request)
			if err != nil {
				t.Fatal(err)
			}
			before := probe.Trace()
			change.mutate(&request)
			probe.completeDarwin(reading, request, "output-ack")
			if !reflect.DeepEqual(before, probe.Trace()) || probe.commandIndex != 0 || probe.trace.Active != 1 {
				t.Fatal("foreign completion mutated an accepted reading")
			}
		})
	}
}

// This integration control runs the actual wrapper/helper wire, qualifies the
// exact shell parent's Wait, and then asks the peer to verify kernel absence.
func TestPublicHostCohortRealParentWaitAndHelperAbsence(t *testing.T) {
	if runtime.GOOS != darwinPlatform {
		return
	} // The repaired cohort is Darwin-only; Linux retains its FIFO controls.
	helper := PublicTUIHostHelper{Executable: os.Getenv("HIPPO_PUBLIC_HOST_HELPER"), SHA256: os.Getenv("HIPPO_PUBLIC_HOST_HELPER_SHA256")}
	probe := newPublicHostProbeControl(t, helper)
	ctx, stop := context.WithTimeout(t.Context(), 3*time.Second)
	defer stop()
	command := exec.CommandContext(ctx, "/bin/sh", "-c", "\"$1\" -A -o %cpu=; exit 125", "owned-cohort-parent", filepath.Join(probe.wrapperDirectory, "ps"))
	command.Env = probe.Environment([]string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "GOMAXPROCS=2"})
	var output, diagnostic bytes.Buffer
	command.Stdout, command.Stderr = &output, &diagnostic
	command.WaitDelay = 500 * time.Millisecond
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	parent := command.Process.Pid
	waitError := command.Wait()
	if command.ProcessState == nil || !command.ProcessState.Exited() || command.ProcessState.ExitCode() != 125 || waitError == nil {
		t.Fatalf("exact parent real Wait unavailable: %v %v", command.ProcessState, waitError)
	}
	before := probe.Trace()
	if len(before.Events) != 2 || before.Events[0].ParentPID != parent || before.Events[1].Phase != "output-ack" ||
		before.Active != 1 || before.Readings != 1 || before.MaximumActive != 1 || output.Len() == 0 || diagnostic.Len() != 0 {
		t.Fatalf("real acknowledged incomplete prefix absent: %+v output=%q diagnostic=%q", before, output.String(), diagnostic.String())
	}
	if err := probe.RetireDarwinReading(ctx, 1, parent); err != nil {
		t.Fatal("peer could not prove actual parent/helper absence after Wait", err)
	}
	after := probe.Trace()
	if after.Active != 0 || after.Readings != 1 || after.MaximumActive != 1 || len(after.Events) != 3 ||
		!reflect.DeepEqual(before.Events, after.Events[:2]) || after.Events[2].Phase != "retired-incomplete" {
		t.Fatalf("real retirement did not preserve the original prefix: %+v", after)
	}
	request := publicHostNextRequest(0, 17, 23)
	request.Nonce, request.Executable = probe.nonce, probe.executable
	probe.mu.Lock()
	reading, _, nextError := probe.acceptDarwinLocked(request)
	probe.mu.Unlock()
	if nextError != nil || reading != 2 || probe.Trace().MaximumActive != 1 {
		t.Fatal("retired actual cohort blocked next owner", reading, nextError)
	}
	probe.completeDarwin(reading, request, "canceled-eof")
	t.Logf("actual shell Wait exit=125 parent=%d helper=%d; peer observed both ESRCH; retained readings=1 maximum=1", parent, before.Events[0].PID)
}
