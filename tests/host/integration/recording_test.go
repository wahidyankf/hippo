package host_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecorderPreparesFreshSourceBinaryConfigAndOwnedTarget(t *testing.T) {
	verifier := compiledVerifier(t)
	fixture, source, binary, config, fixtures, tools, owner, body, revision := recordingFixture(t)
	evidence := filepath.Join(fixture, "evidence with spaces")
	prepareRecording(t, verifier, fixture, source, binary, config, fixtures, tools, owner, evidence)
	data, err := os.ReadFile(filepath.Join(evidence, "evidence.json"))
	if err != nil {
		t.Fatalf("prepare did not publish evidence: %v", err)
	}
	var observed struct {
		SourceRoot string `json:"sourceRoot"`
		Candidate  struct {
			Revision     string            `json:"revision"`
			BinarySHA256 string            `json:"binarySHA256"`
			BinaryCommit string            `json:"binaryCommit"`
			ConfigPaths  map[string]string `json:"configPaths"`
		} `json:"candidate"`
		Steps []json.RawMessage `json:"steps"`
	}
	if err := json.Unmarshal(data, &observed); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	if observed.SourceRoot != source || observed.Candidate.Revision != revision || observed.Candidate.BinaryCommit != revision || observed.Candidate.BinarySHA256 != hex.EncodeToString(sum[:]) || len(observed.Candidate.ConfigPaths) != 1 || len(observed.Steps) != 0 {
		t.Fatalf("candidate/config identity missing or invented completed steps: %s", data)
	}
	info, err := os.Stat(filepath.Join(evidence, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence is not private: %o", info.Mode().Perm())
	}
}

func recordingFixture(t *testing.T) (string, string, string, string, string, string, string, string, string) {
	t.Helper()
	fixture := t.TempDir()
	source := filepath.Join(fixture, "source with spaces")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "source.txt"), []byte("synthetic source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, source, "init", "-q", "-b", "main")
	runFixtureGit(t, source, "add", "source.txt")
	runFixtureGit(t, source, "-c", "user.name=HIPPO fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "fixture source")
	revision := strings.TrimSpace(runFixtureGit(t, source, "rev-parse", "HEAD"))
	binary := filepath.Join(fixture, "hippo with spaces")
	body := fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\n[[ \"$*\" == 'version --json' ]]\nprintf '%%s\\n' '{\"schemaVersion\":1,\"version\":\"v0.0.0-test\",\"commit\":\"%s\"}'\n", revision)
	if err := os.WriteFile(binary, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(fixture, "host config.conf")
	if err := os.WriteFile(config, []byte("synthetic effective config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixtures := filepath.Join(fixture, "fixtures.json")
	if err := os.WriteFile(fixtures, []byte(`{"running":["run-A","run-B"],"queue":["wait-A","wait-B"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tools := filepath.Join(fixture, "tools")
	if err := os.Mkdir(tools, 0o700); err != nil {
		t.Fatal(err)
	}
	owner := strings.Repeat("a", 32)
	host := "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\0' \"$@\" >>\"$HOST_ARGUMENT_LOG\"\ncase \"$*\" in\n *capture-pane*) cat -- \"$HOST_CAPTURE\" ;;\n *display-message*) printf '%s\\n' '40x16' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(tools, "tmux"), []byte(host), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "capture.txt"), []byte("HIPPO_HOST_EVIDENCE_OWNER:"+owner+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return fixture, source, binary, config, fixtures, tools, owner, body, revision
}

func prepareRecording(t *testing.T, verifier, fixture, source, binary, config, fixtures, tools, owner, evidence string) {
	t.Helper()
	output, err := invokeRecorder(verifier, fixture, tools, "--record", "--prepare", "--host", "tmux", "--binary", binary, "--evidence", evidence, "--source-root", source, "--config-root", config, "--fixtures", fixtures, "--socket", "hippo-tui-fixture", "--pane-id", "%1", "--owner-token", owner)
	if err != nil {
		t.Fatalf("record prepare did not complete: %v output%q", err, output)
	}
}

func invokeRecorder(verifier, fixture, tools string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, verifier, args...)
	command.Env = append(fixtureEnvironment(), "PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"), "HOST_ARGUMENT_LOG="+filepath.Join(fixture, "host-argv.bin"), "HOST_CAPTURE="+filepath.Join(fixture, "capture.txt"))
	return command.CombinedOutput()
}

func runFixtureGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir, command.Env = directory, fixtureEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git fixture setup: %v output%q", err, output)
	}
	return string(output)
}

func fixtureEnvironment() []string {
	result := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			result = append(result, value)
		}
	}
	return result
}
