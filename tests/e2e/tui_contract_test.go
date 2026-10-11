package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// invokeCompiledTUI uses the stamped candidate selected by TestMain, closed
// non-terminal input, and captured output. A missing explicit configuration
// makes an accidental observation distinguishable from an early refusal.
func invokeCompiledTUI(t *testing.T, terminal string, arguments ...string) observed {
	t.Helper()
	binary := os.Getenv("HIPPO_BIN")
	if binary == "" {
		t.Fatal("TestMain did not select a compiled candidate")
	}
	root := t.TempDir()
	state := filepath.Join(root, "state")
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "HIPPO_") || name == "TERM" || name == "NO_COLOR" {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, "HIPPO_ROOT="+state, "TERM="+terminal, "NO_COLOR=1")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, arguments...) // #nosec G702 -- TestMain intentionally selects the stamped harness executable; arguments stay separate and execution bounded.
	command.Dir, command.Env = root, environment
	command.Stdin = strings.NewReader("")
	var output, diagnostic strings.Builder
	command.Stdout, command.Stderr = &output, &diagnostic
	result := observed{}
	if runError := command.Run(); runError != nil {
		if ctx.Err() != nil {
			t.Fatalf("compiled invocation exceeded its bound: %v", runError)
		}
		var exitError *exec.ExitError
		if !errors.As(runError, &exitError) {
			t.Fatalf("compiled invocation could not start: %v", runError)
		}
		result.status = exitError.ExitCode()
	}
	result.stdout, result.stderr = output.String(), diagnostic.String()
	t.Logf("argv=%q status=%d stdout=%q stderr=%q", arguments, result.status, result.stdout, result.stderr)
	if _, err := os.Stat(state); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("early invocation created coordination state: %v", err)
	}
	return result
}

func requireCompiledTUIUsage(t *testing.T, result observed, reason string) {
	t.Helper()
	if result.status != 2 || result.stdout != "" || !strings.Contains(result.stderr, "[hippo.args.invalid]") || !strings.Contains(result.stderr, reason) {
		t.Errorf("usage refusal: status=%d stdout=%q stderr=%q; want status 2, args.invalid, and %q", result.status, result.stdout, result.stderr, reason)
	}
	if strings.Contains(result.stdout+result.stderr, "\x1b") {
		t.Error("plain compiled refusal contains terminal escape bytes")
	}
}

func TestCompiledTUIUsesStampedCandidate(t *testing.T) {
	result := invokeCompiledTUI(t, "xterm", "version", "--json")
	var identity struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &identity); err != nil {
		t.Fatal(err)
	}
	if result.status != 0 || result.stderr != "" || identity.Version != "v0.0.0-test" || identity.Commit != strings.Repeat("0", 40) {
		t.Fatalf("candidate identity: %+v, invocation=%+v", identity, result)
	}
}

func TestCompiledTUIHelpPrecedesValidationAndTerminalInspection(t *testing.T) {
	for _, arguments := range [][]string{
		{"tui", "--help"},
		{"tui", "--interval", "nonsense", "--help"},
		{"tui", "--help", "--no-such-flag"},
		{"tui", "--source", "/private", "--tag", "bad", "--help"},
		{"--output", "XML", "tui", "--help"},
	} {
		t.Run(strings.Join(arguments, " "), func(t *testing.T) {
			result := invokeCompiledTUI(t, "dumb", arguments...)
			if result.status != 0 || result.stderr != "" || !strings.Contains(result.stdout, "hippo tui [flags]") {
				t.Fatalf("help did not precede validation/terminal refusal: %+v", result)
			}
			for _, text := range []string{"--config", "--disk-path", "--profile", "--source", "--tag", "--interval", "(default 5s)", `(default ".")`} {
				if !strings.Contains(result.stdout, text) {
					t.Errorf("compiled TUI help omits %q", text)
				}
			}
		})
	}
}

func TestCompiledTUIRejectsInvalidOptionsBeforeObservation(t *testing.T) {
	for _, test := range []struct {
		name      string
		arguments []string
		reason    string
	}{
		{"subsecond interval", []string{"--interval", "999ms"}, "--interval"},
		{"zero interval", []string{"--interval", "0s"}, "--interval"},
		{"negative interval", []string{"--interval", "-1s"}, "--interval"},
		{"source before tag", []string{"--source", "/private", "--tag", "bad"}, "--source"},
		{"invalid tag", []string{"--tag", "bad"}, "--tag"},
		{"unsupported JSON frames", []string{"--json"}, "unknown flag"},
		{"unsupported refresh alias", []string{"--refresh", "1s"}, "unknown flag"},
	} {
		t.Run(test.name, func(t *testing.T) {
			arguments := append([]string{"tui", "--config", "missing.json"}, test.arguments...)
			result := invokeCompiledTUI(t, "xterm", arguments...)
			requireCompiledTUIUsage(t, result, test.reason)
			if test.name == "source before tag" && strings.Contains(result.stderr, "--tag") {
				t.Error("tag validation replaced the first source diagnostic")
			}
		})
	}
}

func TestCompiledTUINonTerminalIsUsageRefusal(t *testing.T) {
	result := invokeCompiledTUI(t, "xterm", "tui", "--config", "missing.json", "--source", "hippo", "--tag", "role=build", "--color", "never")
	requireCompiledTUIUsage(t, result, "use hippo status or hippo watch")
}

func TestCompiledTUIDumbTerminalIsUsageRefusal(t *testing.T) {
	result := invokeCompiledTUI(t, "dumb", "tui", "--config", "missing.json")
	requireCompiledTUIUsage(t, result, "use hippo status or hippo watch")
}

func TestCompiledTUIJSONRefusalNamesCommandAndUsageStatus(t *testing.T) {
	result := invokeCompiledTUI(t, "xterm", "--output", "json", "tui", "--config", "missing.json", "--color", "never")
	requireCompiledTUIUsage(t, result, "use hippo status or hippo watch")
	lines := strings.Split(strings.TrimSpace(result.stderr), "\n")
	if len(lines) != 2 {
		t.Fatalf("want one diagnostic and one failure body, got %q", result.stderr)
	}
	var body struct {
		SchemaVersion int    `json:"schemaVersion"`
		Command       string `json:"command"`
		ExitCode      int    `json:"exitCode"`
		Error         struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &body); err != nil {
		t.Fatal(err)
	}
	if body.SchemaVersion != 1 || body.Command != "hippo tui" || body.ExitCode != 2 || body.Error.Code != "hippo.args.invalid" || body.Error.Retryable {
		t.Errorf("compiled refusal body=%+v; want hippo tui usage failure", body)
	}
}

func TestCompiledTUILabelValidationPrecedesAdmission(t *testing.T) {
	for _, label := range []string{"", "../private", "\x1b[31mprivate", strings.Repeat("x", 65)} {
		t.Run("label="+label, func(t *testing.T) {
			result := invokeCompiledTUI(t, "xterm", "run", "--config", "missing.json", "--label", label, "--", "/bin/sh", "-c", "exit 19")
			requireCompiledTUIUsage(t, result, "label")
		})
	}
}
