package integration_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPublicHostHelperPreparationExternalTestDependencyBoundary(t *testing.T) {
	prepare, err := filepath.Abs("../support/hostprobe/prepare.sh")
	if err != nil {
		t.Fatal(err)
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, foreign := range []bool{false, true} {
		name := "same-package-external-test"
		if foreign {
			name = "foreign-dependency"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			helperParent := t.TempDir()
			module := "module github.com/wahidyankf/hippo\n\ngo 1.26.0\n"
			external := "package hostprobe_test\nimport (\"testing\"; \"github.com/wahidyankf/hippo/tests/support/hostprobe\")\nfunc TestExternal(t *testing.T) { if hostprobe.Value() != 1 { t.Fatal(\"fixture value\") } }\n"
			files := map[string]string{
				"go.sum":                           "",
				"tests/support/hostprobe/probe.go": "package hostprobe\nfunc Value() int { return 1 }\n",
			}
			if foreign {
				module += "require example.invalid/foreign v0.0.0\nreplace example.invalid/foreign => ./foreign\n"
				files["foreign/go.mod"] = "module example.invalid/foreign\n\ngo 1.26.0\n"
				files["foreign/value.go"] = "package foreign\nconst Value = 1\n"
				external = "package hostprobe_test\nimport (\"testing\"; \"example.invalid/foreign\"; \"github.com/wahidyankf/hippo/tests/support/hostprobe\")\nfunc TestExternal(t *testing.T) { if hostprobe.Value() != foreign.Value { t.Fatal(\"fixture value\") } }\n"
			}
			files["go.mod"] = module
			files["tests/support/hostprobe/probe_test.go"] = external
			for relative, contents := range files {
				path := filepath.Join(root, relative)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, "sh", "-c", `. "$1"
trap public_host_helper_cleanup EXIT
public_host_helper_prepare "$2"
test -x "$HIPPO_PUBLIC_HOST_HELPER"
printf 'prepared\n'
`, "helper-preparation-fixture", prepare, root)
			command.Dir = root
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				switch key {
				case "HIPPO_PUBLIC_HOST_HELPER", "HIPPO_PUBLIC_HOST_HELPER_SHA256", "HIPPO_GO_BINARY", "HIPPO_E2E_TEMP_PARENT":
					continue
				}
				command.Env = append(command.Env, entry)
			}
			command.Env = append(command.Env, "HIPPO_GO_BINARY="+goBinary, "HIPPO_E2E_TEMP_PARENT="+helperParent)
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			command.Cancel = func() error {
				killError := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
				if errors.Is(killError, syscall.ESRCH) {
					return os.ErrProcessDone
				}
				return killError
			}
			command.WaitDelay = time.Second
			output, runError := command.CombinedOutput()
			if foreign {
				if runError == nil || !strings.Contains(string(output), "public host helper dependency escaped stdlib boundary: example.invalid/foreign") {
					t.Fatalf("foreign helper dependency was not refused: %v\n%s", runError, output)
				}
			} else if runError != nil || string(output) != "prepared\n" {
				t.Fatalf("same-package external test wrapper was refused: %v\n%s", runError, output)
			}
			entries, readError := os.ReadDir(helperParent)
			if readError != nil || len(entries) != 0 {
				t.Fatalf("helper preparation left owned artifacts: %v %v", entries, readError)
			}
		})
	}
}
