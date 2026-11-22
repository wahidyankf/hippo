package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestFloorStartSupervisedLifetimeReportsSecondPipeFailure(t *testing.T) {
	_, port := floorDoublesSystem()
	calls := 0
	port.Files.Pipe = func() (*os.File, *os.File, error) {
		calls++
		if calls > 1 {
			return nil, nil, errors.New("floor: second pipe refused")
		}
		return os.Pipe()
	}
	_, err := startSupervisedLifetimeWithLauncher(
		port, context.Background(), RunConfig{}, "exe", nil, "launcher", time.Second,
	)
	if err == nil {
		t.Fatal("startSupervisedLifetimeWithLauncher with a failing second pipe = nil, want error")
	}
}

// floorLaunchChild re-executes the test binary as the private lifetime
// launcher. The descriptor layout a launcher needs cannot be built inside a
// live Go process without clobbering its own netpoller, so the child owns
// descriptors 3 and 4 through ExtraFiles while the parent observes the report
// pipe and the exit status.
// readReport is nil when the child owns no readable report end.
const floorLauncherCapability = "0123456789abcdef0123456789abcdef"

func floorLaunchChild(
	t *testing.T, reportChild, capabilityChild, readReport *os.File, executable string,
) (string, int) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = devNull.Close() }()
	command := exec.Command(self, lifetimeLauncherArgument, t.TempDir(), executable)
	command.Env = append(
		os.Environ(), lifetimeLauncherEnvironment+"=1", lifetimeCapabilityEnvironment+"="+floorLauncherCapability,
	)
	command.ExtraFiles = []*os.File{reportChild, capabilityChild}
	command.Stdin, command.Stdout, command.Stderr = devNull, devNull, devNull
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	_ = reportChild.Close()
	_ = capabilityChild.Close()
	report := ""
	if readReport != nil {
		data, err := io.ReadAll(readReport)
		if err != nil {
			t.Fatal(err)
		}
		_ = readReport.Close()
		report = string(data)
	}
	waitErr := command.Wait()
	code := 0
	if exitError, ok := errors.AsType[*exec.ExitError](waitErr); ok {
		code = exitError.ExitCode()
	} else if waitErr != nil {
		t.Fatalf("launcher wait: %v", waitErr)
	}

	return report, code
}

func TestFloorRunLifetimeLauncherMatrix(t *testing.T) { //nolint:gocognit // One re-exec matrix covers every launcher exit through the same pipe harness.
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("true executable unavailable: %v", err)
	}
	t.Run("success", func(t *testing.T) {
		reportReader, reportWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		capabilityReader, capabilityWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := capabilityWriter.WriteString(floorLauncherCapability); err != nil {
			t.Fatal(err)
		}
		_ = capabilityWriter.Close()
		report, code := floorLaunchChild(t, reportWriter, capabilityReader, reportReader, truePath)
		if code != 0 {
			t.Fatalf("launcher(true) = %d, want 0 (report %q)", code, report)
		}
		var handshake lifetimeHandshake
		if err := json.Unmarshal([]byte(report), &handshake); err != nil {
			t.Fatalf("launcher report %q: %v", report, err)
		}
		if handshake.ProcessGroup <= 0 {
			t.Fatalf("launcher handshake = %+v, want a payload process group", handshake)
		}
	})
	t.Run("capability mismatch", func(t *testing.T) {
		reportReader, reportWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		capabilityReader, capabilityWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := capabilityWriter.WriteString("ffffffffffffffffffffffffffffffff"); err != nil {
			t.Fatal(err)
		}
		_ = capabilityWriter.Close()
		report, code := floorLaunchChild(t, reportWriter, capabilityReader, reportReader, truePath)
		if code != 1 {
			t.Fatalf("launcher with a wrong capability = %d, want 1 (report %q)", code, report)
		}
		if !strings.Contains(report, "launcher capability failed") {
			t.Fatalf("launcher report %q, want the capability failure", report)
		}
	})
	t.Run("payload start failure", func(t *testing.T) {
		reportReader, reportWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		capabilityReader, capabilityWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := capabilityWriter.WriteString(floorLauncherCapability); err != nil {
			t.Fatal(err)
		}
		_ = capabilityWriter.Close()
		report, code := floorLaunchChild(
			t, reportWriter, capabilityReader, reportReader, "nonexistent-floor-binary",
		)
		if code != 1 {
			t.Fatalf("launcher with a missing payload = %d, want 1 (report %q)", code, report)
		}
	})
	t.Run("report write failure", func(t *testing.T) {
		reportReader, reportWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		capabilityReader, capabilityWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := capabilityWriter.WriteString(floorLauncherCapability); err != nil {
			t.Fatal(err)
		}
		_ = capabilityWriter.Close()
		_ = reportWriter.Close()
		report, code := floorLaunchChild(t, reportReader, capabilityReader, nil, truePath)
		if code != 1 {
			t.Fatalf("launcher with an unwritable report = %d, want 1 (report %q)", code, report)
		}
	})
}

// floorLauncherFiles runs the launcher body in-process: the descriptor
// binding is covered by the re-exec matrix above, while the files variant
// exercises the supervision logic under the unit profile.
func floorLauncherFiles(
	t *testing.T, report, capability *os.File, executable string,
) int {
	t.Helper()
	t.Setenv(lifetimeCapabilityEnvironment, floorLauncherCapability)
	oldStdin := os.Stdin
	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = devNull
	defer func() {
		os.Stdin = oldStdin
		_ = devNull.Close()
	}()
	return runLifetimeLauncherFiles(report, capability, t.TempDir(), executable, nil)
}

func floorLauncherReport(t *testing.T, reader *os.File) string {
	t.Helper()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	return string(data)
}

func TestFloorRunLifetimeLauncherFilesMatrix(t *testing.T) { //nolint:gocognit // One files matrix covers every launcher exit through the same pipe harness.
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("true executable unavailable: %v", err)
	}
	t.Run("success", func(t *testing.T) {
		reportReader, reportWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		capabilityReader, capabilityWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := capabilityWriter.WriteString(floorLauncherCapability); err != nil {
			t.Fatal(err)
		}
		_ = capabilityWriter.Close()
		code := floorLauncherFiles(t, reportWriter, capabilityReader, truePath)
		report := floorLauncherReport(t, reportReader)
		if code != 0 {
			t.Fatalf("launcher files(true) = %d, want 0 (report %q)", code, report)
		}
		var handshake lifetimeHandshake
		if err := json.Unmarshal([]byte(report), &handshake); err != nil {
			t.Fatalf("launcher report %q: %v", report, err)
		}
		if handshake.ProcessGroup <= 0 {
			t.Fatalf("launcher handshake = %+v, want a payload process group", handshake)
		}
	})
	t.Run("capability mismatch", func(t *testing.T) {
		reportReader, reportWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		capabilityReader, capabilityWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := capabilityWriter.WriteString("ffffffffffffffffffffffffffffffff"); err != nil {
			t.Fatal(err)
		}
		_ = capabilityWriter.Close()
		code := floorLauncherFiles(t, reportWriter, capabilityReader, truePath)
		report := floorLauncherReport(t, reportReader)
		if code != 1 {
			t.Fatalf("launcher files with a wrong capability = %d, want 1 (report %q)", code, report)
		}
		if !strings.Contains(report, "launcher capability failed") {
			t.Fatalf("launcher report %q, want the capability failure", report)
		}
	})
	t.Run("payload start failure", func(t *testing.T) {
		reportReader, reportWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		capabilityReader, capabilityWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := capabilityWriter.WriteString(floorLauncherCapability); err != nil {
			t.Fatal(err)
		}
		_ = capabilityWriter.Close()
		code := floorLauncherFiles(t, reportWriter, capabilityReader, "nonexistent-floor-binary")
		report := floorLauncherReport(t, reportReader)
		if code != 1 {
			t.Fatalf("launcher files with a missing payload = %d, want 1 (report %q)", code, report)
		}
	})
	t.Run("report write failure", func(t *testing.T) {
		reportReader, reportWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_ = reportWriter.Close()
		capabilityReader, capabilityWriter, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := capabilityWriter.WriteString(floorLauncherCapability); err != nil {
			t.Fatal(err)
		}
		_ = capabilityWriter.Close()
		code := floorLauncherFiles(t, reportReader, capabilityReader, truePath)
		_ = reportReader.Close()
		if code != 1 {
			t.Fatalf("launcher files with an unwritable report = %d, want 1", code)
		}
	})
}
