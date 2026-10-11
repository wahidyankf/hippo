package support

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

// These actual shell preparation controls replace archive/hash/platform tools
// with recording stand-ins. They prove refusal and orchestration, never release
// provenance. A real checksum-authenticated archive remains separate evidence.
func TestBDDGateOldReleasePreparationChecksEveryHostAndRefusal(t *testing.T) {
	for _, row := range []struct {
		name, system, architecture, failure string
		want                                int
	}{
		{"Darwin arm64", "Darwin", "arm64", "", 0},
		{"Darwin amd64", "Darwin", "x86_64", "", 0},
		{"Linux arm64", "Linux", "aarch64", "", 0},
		{"Linux amd64", "Linux", "x86_64", "", 0},
		{"archive hash mismatch", "Darwin", "arm64", "hash", 1},
		{"archive members invalid", "Darwin", "arm64", "members", 1},
		{"archive member symlink", "Darwin", "arm64", "symlink", 1},
		{"executable identity mismatch", "Darwin", "arm64", "identity", 1},
		{"identity stderr", "Darwin", "arm64", "stderr", 1},
		{"identity failure retained", "Darwin", "arm64", "version-exit", 73},
		{"download failure retained", "Darwin", "arm64", "download", 73},
		{"unsupported host", "Unsupported", "arm64", "", 1},
		{"malformed lock", "Darwin", "arm64", "lock", 1},
		{"supplied archive succeeds", "Darwin", "arm64", "offline", 0},
		{"supplied archive empty", "Darwin", "arm64", "offline-empty", 1},
		{"supplied archive relative", "Darwin", "arm64", "offline-relative", 1},
		{"supplied archive missing", "Darwin", "arm64", "offline-missing", 1},
		{"supplied archive symlink", "Darwin", "arm64", "offline-symlink", 1},
		{"supplied copy failure retained", "Darwin", "arm64", "offline-copy", 73},
		{"supplied archive hash mismatch", "Darwin", "arm64", "offline-hash", 1},
		{"supplied identity mismatch", "Darwin", "arm64", "offline-identity", 1},
	} {
		t.Run(row.name, func(t *testing.T) {
			code, output, directory := runBDDGateOldReleaseFixture(t, row.system, row.architecture, row.failure)
			if code != row.want {
				t.Fatalf("preparation exit=%d want=%d output=%s", code, row.want, output)
			}
			if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("caller did not clean owned old artifacts: %v", err)
			}
			if row.want == 0 && (!strings.Contains(string(output), "BOUND_OLD=") || !strings.Contains(string(output), `"version":"v0.8.4"`)) {
				t.Fatalf("actual stand-in old invocation or binding missing: %s", output)
			}
		})
	}
}

func runBDDGateOldReleaseFixture(t *testing.T, system, architecture, failure string) (int, []byte, string) {
	t.Helper()
	return runBDDGateReleaseFixture(t, system, architecture, failure, false)
}

func runBDDGateReleaseFixture(t *testing.T, system, architecture, failure string, v085 bool) (int, []byte, string) {
	t.Helper()
	owner, err := toolRoot()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "old release fixture with spaces")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	fixtureRoot, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixtureRoot.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, directory := range []string{"bin", "tests/e2e", "owned-e2e"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	release := "v0.8.4"
	adaptV085 := strings.NewReplacer(
		"public_old_release", "public_v085_release", "old-release", "v085-release",
		"hippo-old.lock", "hippo-v085.lock", "HIPPO_PUBLIC_OLD", "HIPPO_PUBLIC_V085",
		"HIPPO_E2E_OLD_ARCHIVE", "HIPPO_E2E_V085_ARCHIVE", "BOUND_OLD", "BOUND_V085",
		"v0.8.4", "v0.8.5", "abde2ecbeff7513549cfd1c0805fd20b5d232c01", "456d24bc2a20ee23a7b81746c1789133cd5c3a97",
	)
	if v085 {
		release = "v0.8.5"
	}
	write := func(path, content string) {
		t.Helper()
		if v085 {
			path, content = adaptV085.Replace(path), adaptV085.Replace(content)
		}
		if err := fixtureRoot.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"tests/e2e/old-release.sh", "tests/e2e/hippo-old.lock"} {
		if v085 {
			path = adaptV085.Replace(path)
		}
		data, err := os.ReadFile(filepath.Join(owner, path))
		if err != nil {
			t.Fatal(err)
		}
		write(path, string(data))
	}
	if failure == "lock" {
		write("tests/e2e/hippo-old.lock", "version=v0.8.4\ncommit=incorrect\ndarwin-arm64=invalid\n")
	}
	prepareRecordedReleaseTools(write)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", filepath.Join(root, "invoke.sh"))
	command.Dir = root
	command.Env = []string{"PATH=" + filepath.Join(root, "bin") + ":/usr/bin:/bin", "FIXTURE_ROOT=" + root, "FIXTURE_SYSTEM=" + system, "FIXTURE_ARCH=" + architecture, "FIXTURE_FAILURE=" + failure, "HIPPO_PUBLIC_OLD_BIN=inherited-unverified-binary", "HIPPO_PUBLIC_OLD_BIN_SHA256=inherited-unverified-digest", "HIPPO_PUBLIC_OLD_VERSION_RECEIPT=inherited-unverified-receipt"}
	if v085 {
		for index, entry := range command.Env {
			command.Env[index] = adaptV085.Replace(entry)
		}
		command.Env = append(command.Env, "FIXTURE_V085=1", `FIXTURE_OLD_RECEIPT={"schemaVersion":1,"version":"v0.8.4","commit":"abde2ecbeff7513549cfd1c0805fd20b5d232c01"}`)
	}
	if strings.HasPrefix(failure, "offline") {
		archive := filepath.Join(root, "supplied archive.tar.gz")
		write("supplied archive.tar.gz", "recorded supplied archive\n")
		archive = selectOfflineReleaseArchive(t, root, failure, archive)
		archiveVariable := "HIPPO_E2E_OLD_ARCHIVE="
		if v085 {
			archiveVariable = "HIPPO_E2E_V085_ARCHIVE="
		}
		command.Env = append(command.Env, archiveVariable+archive)
	}
	output, runError := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("owned preparation control exceeded bound: %v", ctx.Err())
	}
	code := 0
	if runError != nil {
		var exit *exec.ExitError
		if !errors.As(runError, &exit) {
			t.Fatalf("start preparation fixture: %v", runError)
		}
		code = exit.ExitCode()
	}
	requireReleasePreparationObservations(t, root, failure, release, v085, code)
	return code, output, filepath.Join(root, "owned-e2e")
}

func requireReleasePreparationObservations(t *testing.T, root, failure, release string, v085 bool, code int) {
	t.Helper()
	if strings.HasPrefix(failure, "offline") {
		if _, err := os.Stat(filepath.Join(root, "curl.argv")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("supplied archive branch attempted a download: %v", err)
		}
		if data, err := os.ReadFile(filepath.Join(root, "supplied archive.tar.gz")); err != nil || string(data) != "recorded supplied archive\n" {
			t.Fatalf("preparation mutated supplied archive: %q: %v", data, err)
		}
	}
	if code == 0 {
		parser, err := os.ReadFile(filepath.Join(root, "parser.argv"))
		selector := "run\n-p=1\n./tests/support/oldidentity\n"
		if v085 {
			selector += "--v085\n"
		}
		if err != nil || !strings.Contains(string(parser), selector) {
			t.Fatalf("real parser runtime was not registered before binding: %s: %v", parser, err)
		}
	}
	if code == 0 && !strings.HasPrefix(failure, "offline") {
		argv, err := os.ReadFile(filepath.Join(root, "curl.argv"))
		if err != nil || !strings.Contains(string(argv), "https://github.com/wahidyankf/hippo/releases/download/"+release+"/hippo_"+release+"_") || !strings.Contains(string(argv), "--max-time\n120\n") || !strings.Contains(string(argv), "--proto-redir\n=https\n") {
			t.Fatalf("bounded immutable HTTPS registration changed: %s: %v", argv, err)
		}
	}
}

func selectOfflineReleaseArchive(t *testing.T, root, failure, archive string) string {
	t.Helper()
	switch failure {
	case "offline-empty":
		archive = ""
	case "offline-relative":
		archive = "supplied archive.tar.gz"
	case "offline-missing":
		archive = filepath.Join(root, "missing archive.tar.gz")
	case "offline-symlink":
		link := filepath.Join(root, "symlink archive.tar.gz")
		if err := os.Symlink(archive, link); err != nil {
			t.Fatal(err)
		}
		archive = link
	}
	return archive
}

func prepareRecordedReleaseTools(write func(string, string)) {
	write("bin/uname", "#!/bin/sh\nset -eu\ncase \"$1\" in -s) printf '%s\\n' \"$FIXTURE_SYSTEM\" ;; -m) printf '%s\\n' \"$FIXTURE_ARCH\" ;; *) exit 1 ;; esac\n")
	write("bin/curl", `#!/bin/sh
set -eu
[ "$FIXTURE_FAILURE" != download ] || exit 73
printf '%s\n' "$@" >"$FIXTURE_ROOT/curl.argv"
while [ "$#" -gt 0 ]; do
 if [ "$1" = --output ]; then shift; printf 'recorded archive\n' >"$1"; break; fi
 shift
done
`)
	write("bin/cp", `#!/bin/sh
set -eu
[ "$FIXTURE_FAILURE" != offline-copy ] || exit 73
exec /bin/cp "$@"
`)
	write("bin/go", `#!/bin/sh
set -eu
[ "$1:$2:$3" = 'run:-p=1:./tests/support/oldidentity' ]
receipt=$4
if [ "${FIXTURE_V085:-}" = 1 ]; then
 [ "$4" = --v085 ]
 receipt=$5
fi
[ "$receipt" = "$FIXTURE_ROOT/owned-e2e/old-release/version.stdout" ]
[ -f "$receipt" ]
[ "$FIXTURE_FAILURE" != parser ] || exit 73
printf '%s\n' "$@" >"$FIXTURE_ROOT/parser.argv"
`)
	write("bin/tar", `#!/bin/sh
set -eu
if [ "$1" = -tzf ]; then
 if [ "$FIXTURE_FAILURE" = members ]; then printf '../hippo\n'; else printf 'hippo\n'; fi
 exit 0
fi
[ "$1" = -xzf ] && [ "$3" = -C ] && [ "$5" = hippo ]
if [ "$FIXTURE_FAILURE" = symlink ]; then ln -s "$FIXTURE_ROOT/bin/uname" "$4/hippo"; exit 0; fi
cat >"$4/hippo" <<'OLD'
#!/bin/sh
set -eu
[ "$1:$2" = version:--json ]
[ "$FIXTURE_FAILURE" != version-exit ] || exit 73
if [ "$FIXTURE_FAILURE" = stderr ]; then printf 'unexpected stderr\n' >&2; fi
if [ "$FIXTURE_FAILURE" = old-receipt ]; then printf '%s\n' "$FIXTURE_OLD_RECEIPT"; elif
 [ "$FIXTURE_FAILURE" = identity ] || [ "$FIXTURE_FAILURE" = offline-identity ]; then printf '{}\n'; else
 printf '%s\n' '{"schemaVersion":1,"version":"v0.8.4","commit":"abde2ecbeff7513549cfd1c0805fd20b5d232c01"}'
fi
OLD
`)
	write("invoke.sh", `#!/bin/sh
set -eu
owned=$FIXTURE_ROOT/owned-e2e
trap 'rm -rf -- "$owned"' EXIT
. "$FIXTURE_ROOT/tests/e2e/old-release.sh"
public_host_helper_hash() {
 case "$1" in
 */release.tar.gz)
  if [ "$FIXTURE_FAILURE" = hash ] || [ "$FIXTURE_FAILURE" = offline-hash ]; then printf '%064d\n' 0; else
   case "$FIXTURE_SYSTEM:$FIXTURE_ARCH" in
    Darwin:arm64) key=darwin-arm64 ;; Darwin:x86_64) key=darwin-amd64 ;;
    Linux:aarch64) key=linux-arm64 ;; Linux:x86_64) key=linux-amd64 ;;
   esac
   sed -n "s/^$key=//p" "$FIXTURE_ROOT/tests/e2e/hippo-old.lock"
  fi ;;
 */hippo) printf '%064d\n' 1 ;;
 *) return 1 ;;
 esac
}
public_old_release_prepare "$FIXTURE_ROOT" "$owned"
printf 'BOUND_OLD=%s\n' "$HIPPO_PUBLIC_OLD_BIN"
cat "$HIPPO_PUBLIC_OLD_VERSION_RECEIPT"
`)
}
