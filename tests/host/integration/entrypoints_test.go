package host_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func compiledVerifier(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "evidence-verifier")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "build", "-o", binary, "./tests/host/evidence-verifier")
	command.Dir = filepath.Join("..", "..", "..")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build verifier setup: %v\n%s", err, output)
	}
	return binary
}

func TestVerifierCLIHelpAndRefusals(t *testing.T) {
	binary := compiledVerifier(t)
	cases := []struct {
		name string
		args []string
		code int
		text string
	}{
		{"help first", []string{"--host", "invalid", "--help"}, 0, "Usage:"},
		{"missing arguments", nil, 2, "--host"},
		{"unknown flag", []string{"--unknown"}, 2, "unknown"},
		{"unknown host", []string{"--host", "invalid", "--evidence", "/task/proof", "--binary", "/task/hippo"}, 2, "host"},
		{"relative binary", []string{"--host", "tmux", "--evidence", "/task/proof", "--binary", "hippo"}, 2, "absolute"},
		{"relative evidence", []string{"--host", "tmux", "--evidence", "proof", "--binary", "/task/hippo"}, 2, "absolute"},
		{"missing evidence", []string{"--host", "tmux", "--evidence", "/hippo-fixture-missing/proof", "--binary", "/hippo-fixture-missing/hippo"}, 125, "evidence"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, test.args...)
			output, err := command.CombinedOutput()
			actual := 0
			if err != nil {
				if failure, matched := errors.AsType[*exec.ExitError](err); matched {
					actual = failure.ExitCode()
				} else {
					t.Fatalf("invoke verifier setup: %v", err)
				}
			}
			if actual != test.code || !strings.Contains(string(output), test.text) {
				t.Fatalf("expected status%d/%q; got status%d output%q", test.code, test.text, actual, output)
			}
		})
	}
}

func TestHostRunnerInvokesRecorderWithExactArgumentsAndPropagatesFailure(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	recorder := filepath.Join(fixture, "recorder with spaces")
	argumentLog := filepath.Join(fixture, "argv.bin")
	source := "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\0' \"$@\" >\"$HOST_ARGUMENT_LOG\"\nexit 17\n"
	if err := os.WriteFile(recorder, []byte(source), 0o700); err != nil {
		t.Fatal(err)
	}
	arguments := []string{"--host", "tmux", "--binary", filepath.Join(fixture, "hippo with spaces"), "--evidence", filepath.Join(fixture, "proof with spaces"), "--prepare"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(root, "tests", "host", "tui-sequence.sh"), arguments...)
	command.Env = append(os.Environ(), "HOST_EVIDENCE_VERIFIER="+recorder, "HOST_ARGUMENT_LOG="+argumentLog)
	output, err := command.CombinedOutput()
	actual := 0
	if err != nil {
		if failure, matched := errors.AsType[*exec.ExitError](err); matched {
			actual = failure.ExitCode()
		} else {
			t.Fatalf("invoke runner setup: %v", err)
		}
	}
	if actual != 17 {
		t.Errorf("recorder status not preserved: got%d output%q", actual, output)
	}
	data, readError := os.ReadFile(argumentLog)
	if readError != nil {
		t.Fatalf("runner did not invoke recorder: %v", readError)
	}
	expected := strings.Join(append([]string{"--record"}, arguments...), "\x00") + "\x00"
	if string(data) != expected {
		t.Fatalf("expected exact argv%q; got%q", expected, data)
	}
}

func TestHostRunnerHelpDoesNotRunRecorder(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(root, "tests", "host", "tui-sequence.sh"), "--help")
	command.Env = append(os.Environ(), "HOST_EVIDENCE_VERIFIER=/hippo-fixture-missing/recorder")
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "Usage:") {
		t.Fatalf("runner help failed or missing: error%v output%q", err, output)
	}
}
