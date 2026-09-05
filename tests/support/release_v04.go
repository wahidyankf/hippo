package support

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const releaseFixtureVersion = "v0.4.0"

// The CI smoke build must hand the builder and the validator one identical
// release identity, so both commands are read rather than matched literally.
var (
	ciReleaseBuilderCommand   = regexp.MustCompile(`\./scripts/build-release\.sh (\S+) "\$GITHUB_SHA" dist`)
	ciReleaseValidatorCommand = regexp.MustCompile(`\./tests/artifacts/release-assets\.sh dist (\S+) "\$GITHUB_SHA"`)
	exactReleaseVersion       = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

func runGitV04(directory string, arguments ...string) ([]byte, error) {
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %s: %w", strings.Join(arguments, " "), bytes.TrimSpace(output), err)
	}

	return output, nil
}

func copyFixtureEntryV04(sourceRoot, targetRoot, name string) error {
	source := filepath.Join(sourceRoot, filepath.FromSlash(name))
	target := filepath.Join(targetRoot, filepath.FromSlash(name))
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, readError := os.Readlink(source)
		if readError != nil {
			return readError
		}

		return os.Symlink(link, target)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err = io.Copy(output, input); err != nil {
		_ = output.Close()

		return err
	}

	return output.Close()
}

func cleanReleaseFixtureV04(root string) (string, string, error) {
	sourceRoot, err := moduleRootV04()
	if err != nil {
		return "", "", err
	}
	fixtureRoot := filepath.Join(root, "repository")
	if err = os.MkdirAll(fixtureRoot, 0o700); err != nil {
		return "", "", err
	}
	listed, err := runGitV04(sourceRoot, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", "", err
	}
	for name := range strings.SplitSeq(string(listed), "\x00") {
		if name == "" {
			continue
		}
		if err = copyFixtureEntryV04(sourceRoot, fixtureRoot, name); err != nil {
			return "", "", fmt.Errorf("copy fixture entry: %w", err)
		}
	}
	if _, err = runGitV04(fixtureRoot, "init", "-q", "-b", "main"); err != nil {
		return "", "", err
	}
	if _, err = runGitV04(fixtureRoot, "add", "-A"); err != nil {
		return "", "", err
	}
	if _, err = runGitV04(fixtureRoot, "-c", "user.name=HIPPO fixture", "-c", "user.email=fixture@example.invalid", "commit", "-q", "-m", fixtureOwner); err != nil {
		return "", "", err
	}
	head, err := runGitV04(fixtureRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}

	return fixtureRoot, strings.TrimSpace(string(head)), nil
}

func installFakeGoV04(root string) (string, error) {
	bin := filepath.Join(root, "fake-bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		return "", err
	}
	script := `#!/bin/sh
set -eu
output=
while [ "$#" -gt 0 ]; do
	if [ "$1" = "-o" ]; then
		shift
		output=$1
		break
	fi
	shift
done
test -n "$output"
printf '#!/bin/sh\nexit 0\n' > "$output"
chmod 755 "$output"
`
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o700); err != nil {
		return "", err
	}

	return bin, nil
}

func installSourceAwareFakeGoV04(root string) (string, error) {
	bin, err := installFakeGoV04(root)
	if err != nil {
		return "", err
	}
	script := `#!/bin/sh
set -eu
if find cmd/hippo -name 'injected_*.go' -print | grep -q .; then
	exit 97
fi
output=
while [ "$#" -gt 0 ]; do
	if [ "$1" = "-o" ]; then
		shift
		output=$1
		break
	fi
	shift
done
test -n "$output"
printf '#!/bin/sh\nexit 0\n' > "$output"
chmod 755 "$output"
`
	if err = os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o700); err != nil {
		return "", err
	}

	return bin, nil
}

func replaceEnvironmentV04(environment []string, name, value string) []string {
	prefix := name + "="
	replacement := prefix + value
	for index, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			environment[index] = replacement

			return environment
		}
	}

	return append(environment, replacement)
}

func invokeReleaseBuilderEnvironmentV04(repository, version, commit, output, fakeGo string, extra map[string]string) error {
	command := exec.Command(filepath.Join(repository, "scripts", "build-release.sh"), version, commit, output)
	command.Dir = repository
	command.Env = append([]string{}, os.Environ()...)
	if fakeGo != "" {
		command.Env = replaceEnvironmentV04(command.Env, "PATH", fakeGo+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	for name, value := range extra {
		command.Env = replaceEnvironmentV04(command.Env, name, value)
	}
	combined, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("release builder: %s: %w", bytes.TrimSpace(combined), err)
	}

	return nil
}

func invokeReleaseBuilderV04(repository, version, commit, output, fakeGo string) error {
	return invokeReleaseBuilderEnvironmentV04(repository, version, commit, output, fakeGo, nil)
}

func shellQuoteV04(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func installGitStatusFailureToolchainV04(root string) (string, error) {
	bin, err := installFakeGoV04(root)
	if err != nil {
		return "", err
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		return "", err
	}
	script := `#!/bin/sh
for argument in "$@"; do
	if [ "$argument" = status ]; then
		exit 86
	fi
done
exec ` + shellQuoteV04(realGit) + ` "$@"
`
	if err = os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
		return "", err
	}

	return bin, nil
}

func releaseBuilderRejectedV04(repository, version, commit, output, fakeGo string) error {
	buildError := invokeReleaseBuilderV04(repository, version, commit, output, fakeGo)
	if buildError == nil {
		return errors.New("invalid release identity was accepted")
	}

	return nil
}

func releaseBuilderRejectedBeforeOutputV04(repository, version, commit, output, fakeGo string) error {
	if err := releaseBuilderRejectedV04(repository, version, commit, output, fakeGo); err != nil {
		return err
	}
	_, statError := os.Stat(output)
	if !errors.Is(statError, os.ErrNotExist) {
		return fmt.Errorf("invalid release identity reached output: %w", statError)
	}

	return nil
}

func requireV04ReleaseVersionSyntax(root string) error {
	repository, head, err := cleanReleaseFixtureV04(filepath.Join(root, "version"))
	if err != nil {
		return err
	}
	fakeGo, err := installFakeGoV04(root)
	if err != nil {
		return err
	}
	for index, version := range []string{"1.2.3", "v1.2", "v1.2.3trailing", "v01.2.3", "v1.02.3", "v1.2.03"} {
		output := filepath.Join(root, fmt.Sprintf("invalid-version-%d", index))
		if err = releaseBuilderRejectedV04(repository, version, head, output, fakeGo); err != nil {
			return fmt.Errorf("version %q: %w", version, err)
		}
	}
	if err = invokeReleaseBuilderV04(repository, releaseFixtureVersion, head, filepath.Join(root, "valid-version"), fakeGo); err != nil {
		return fmt.Errorf("exact semantic version rejected: %w", err)
	}

	return nil
}

func requireV04ReleaseCommitIdentity(root string) error {
	repository, head, err := cleanReleaseFixtureV04(filepath.Join(root, "commit"))
	if err != nil {
		return err
	}
	fakeGo, err := installFakeGoV04(root)
	if err != nil {
		return err
	}
	for index, commit := range []string{head[:12], strings.ToUpper(head), strings.Repeat("0", 40)} {
		if err = releaseBuilderRejectedV04(repository, releaseFixtureVersion, commit, filepath.Join(root, fmt.Sprintf("invalid-commit-%d", index)), fakeGo); err != nil {
			return fmt.Errorf("commit %q: %w", commit, err)
		}
	}
	if err = invokeReleaseBuilderV04(repository, releaseFixtureVersion, head, filepath.Join(root, "valid-commit"), fakeGo); err != nil {
		return fmt.Errorf("full lowercase real commit rejected: %w", err)
	}

	return nil
}

func appendFixtureCommitV04(repository, name string) (string, error) {
	if err := os.WriteFile(filepath.Join(repository, name), []byte(name+"\n"), 0o600); err != nil {
		return "", err
	}
	if _, err := runGitV04(repository, "add", name); err != nil {
		return "", err
	}
	if _, err := runGitV04(repository, "-c", "user.name=HIPPO fixture", "-c", "user.email=fixture@example.invalid", "commit", "-q", "-m", name); err != nil {
		return "", err
	}
	head, err := runGitV04(repository, "rev-parse", "HEAD")

	return strings.TrimSpace(string(head)), err
}

func requireV04ReleaseHeadEquality(root string) error {
	repository, previousHead, err := cleanReleaseFixtureV04(filepath.Join(root, "head"))
	if err != nil {
		return err
	}
	if _, err = appendFixtureCommitV04(repository, "next"); err != nil {
		return err
	}
	fakeGo, err := installFakeGoV04(root)
	if err != nil {
		return err
	}

	return releaseBuilderRejectedBeforeOutputV04(repository, releaseFixtureVersion, previousHead, filepath.Join(root, "non-head-output"), fakeGo)
}

func requireV04ReleaseTrackedClean(root string) error {
	repository, head, err := cleanReleaseFixtureV04(filepath.Join(root, "tracked"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(repository, "README.md"), []byte("tracked dirt\n"), 0o600); err != nil {
		return err
	}
	fakeGo, err := installFakeGoV04(root)
	if err != nil {
		return err
	}

	return releaseBuilderRejectedV04(repository, releaseFixtureVersion, head, filepath.Join(root, "tracked-output"), fakeGo)
}

func requireV04ReleaseUntrackedClean(root string) error {
	repository, head, err := cleanReleaseFixtureV04(filepath.Join(root, "untracked"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(repository, "untracked-release-input"), []byte("untracked dirt\n"), 0o600); err != nil {
		return err
	}
	fakeGo, err := installFakeGoV04(root)
	if err != nil {
		return err
	}

	return releaseBuilderRejectedV04(repository, releaseFixtureVersion, head, filepath.Join(root, "untracked-output"), fakeGo)
}

func requireV04ReleaseNoOutputClass(root, identity string) error {
	repository, head, err := cleanReleaseFixtureV04(filepath.Join(root, "pre-output"))
	if err != nil {
		return err
	}
	fakeGo, err := installFakeGoV04(root)
	if err != nil {
		return err
	}
	version := releaseFixtureVersion
	commit := head
	switch identity {
	case "version syntax":
		version = "v1.2"
	case "commit syntax":
		commit = head[:12]
	case "nonexistent commit":
		commit = strings.Repeat("0", 40)
	case "non-HEAD commit":
		if _, err = appendFixtureCommitV04(repository, "next"); err != nil {
			return err
		}
	case "tracked dirt":
		if err = os.WriteFile(filepath.Join(repository, "README.md"), []byte("tracked dirt\n"), 0o600); err != nil {
			return err
		}
	case "untracked dirt":
		if err = os.WriteFile(filepath.Join(repository, "untracked-release-input"), []byte("untracked dirt\n"), 0o600); err != nil {
			return err
		}
	case "git status failure":
		fakeGo, err = installGitStatusFailureToolchainV04(root)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown invalid release identity %q", identity)
	}

	return releaseBuilderRejectedBeforeOutputV04(repository, version, commit, filepath.Join(root, "forbidden-output"), fakeGo)
}

func requireV04ReleaseStatusFailure(root string) error {
	return requireV04ReleaseNoOutputClass(root, "git status failure")
}

func installFailingGoV04(root string) (string, error) {
	bin, err := installFakeGoV04(root)
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nexit 97\n"), 0o700); err != nil {
		return "", err
	}

	return bin, nil
}

func installSecondAllocationFailureV04(root, temporaryRoot string) (string, error) {
	bin, err := installFakeGoV04(root)
	if err != nil {
		return "", err
	}
	realMktemp, err := exec.LookPath("mktemp")
	if err != nil {
		return "", err
	}
	script := `#!/bin/sh
set -eu
counter="$TMPDIR/.hippo-mktemp-count"
count=0
if [ -f "$counter" ]; then count=$(cat "$counter"); fi
count=$((count + 1))
printf '%s\n' "$count" >"$counter"
if [ "$count" -eq 2 ]; then exit 94; fi
exec ` + shellQuoteV04(realMktemp) + ` "$@"
`
	if err = os.WriteFile(filepath.Join(bin, "mktemp"), []byte(script), 0o700); err != nil {
		return "", err
	}
	if err = os.MkdirAll(temporaryRoot, 0o700); err != nil {
		return "", err
	}

	return bin, nil
}

func requireV04ReleaseTempCleanup(root, result string) error {
	repository, head, err := cleanReleaseFixtureV04(filepath.Join(root, "temporary-repository"))
	if err != nil {
		return err
	}
	temporaryRoot := filepath.Join(root, "controlled-temporary")
	if err = os.MkdirAll(temporaryRoot, 0o700); err != nil {
		return err
	}
	var toolchain string
	switch result {
	case succeedsOutcome:
		toolchain, err = installFakeGoV04(filepath.Join(root, "success-tool"))
	case "build fails":
		toolchain, err = installFailingGoV04(filepath.Join(root, "failure-tool"))
	case "second allocation fails":
		toolchain, err = installSecondAllocationFailureV04(filepath.Join(root, "allocation-tool"), temporaryRoot)
	default:
		return fmt.Errorf("unknown release result %q", result)
	}
	if err != nil {
		return err
	}
	buildError := invokeReleaseBuilderEnvironmentV04(
		repository,
		releaseFixtureVersion,
		head,
		filepath.Join(root, "temporary-assets"),
		toolchain,
		map[string]string{"TMPDIR": temporaryRoot},
	)
	if result == succeedsOutcome && buildError != nil {
		return buildError
	}
	if result != succeedsOutcome && buildError == nil {
		return fmt.Errorf("release result %q unexpectedly succeeded", result)
	}
	entries, err := os.ReadDir(temporaryRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "hippo-release.") || strings.HasPrefix(entry.Name(), "hippo-release-source.") {
			return fmt.Errorf("release staging remained after %s", result)
		}
	}

	return nil
}

func commitFixtureIgnoreV04(repository, pattern string) error {
	ignorePath := filepath.Join(repository, ".gitignore")
	file, err := os.OpenFile(ignorePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintln(file, pattern); err != nil {
		_ = file.Close()

		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if _, err = runGitV04(repository, "add", ".gitignore"); err != nil {
		return err
	}
	_, err = runGitV04(repository, "-c", "user.name=HIPPO fixture", "-c", "user.email=fixture@example.invalid", "commit", "-q", "-m", "ignore fixture source")

	return err
}

func configureExcludedSourceV04(caseRoot, repository, kind, relative string) error {
	switch kind {
	case "ignored":
		return commitFixtureIgnoreV04(repository, "/"+relative)
	case "global":
		excludesPath := filepath.Join(caseRoot, "global-excludes")
		if err := os.WriteFile(excludesPath, []byte(relative+"\n"), 0o600); err != nil {
			return err
		}
		_, err := runGitV04(repository, "config", "core.excludesFile", excludesPath)

		return err
	case "repository":
		return os.WriteFile(filepath.Join(repository, ".git", "info", "exclude"), []byte(relative+"\n"), 0o600)
	default:
		return errors.New("unknown release exclusion fixture")
	}
}

func requireV04ReleaseExactSource(root string) error {
	for _, kind := range []string{"ignored", "global", "repository"} {
		caseRoot := filepath.Join(root, kind)
		repository, _, err := cleanReleaseFixtureV04(caseRoot)
		if err != nil {
			return err
		}
		relative := filepath.ToSlash(filepath.Join("cmd", "hippo", "injected_"+kind+".go"))
		if err = configureExcludedSourceV04(caseRoot, repository, kind, relative); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(repository, filepath.FromSlash(relative)), []byte("this is not valid Go source\n"), 0o600); err != nil {
			return err
		}
		status, err := runGitV04(repository, statusCommandName, "--porcelain", "--untracked-files=all")
		if err != nil || len(bytes.TrimSpace(status)) != 0 {
			return fmt.Errorf("%s excluded fixture is not status-clean: %s: %w", kind, bytes.TrimSpace(status), err)
		}
		head, err := runGitV04(repository, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		fakeGo, err := installSourceAwareFakeGoV04(filepath.Join(caseRoot, "tool"))
		if err != nil {
			return err
		}
		output := filepath.Join(caseRoot, "assets")
		if err = invokeReleaseBuilderV04(repository, releaseFixtureVersion, strings.TrimSpace(string(head)), output, fakeGo); err != nil {
			return fmt.Errorf("%s excluded source affected exact-commit build: %w", kind, err)
		}
	}

	return nil
}

func requireV04ReleaseInventory(root string) error {
	repository, _, err := cleanReleaseFixtureV04(filepath.Join(root, "inventory"))
	if err != nil {
		return err
	}
	policyPath := filepath.Join(repository, "tests", "artifacts", "run.sh")
	policy, err := os.ReadFile(policyPath)
	if err != nil {
		return err
	}
	for _, required := range []string{
		"conformance.manifest.json.example", "cmd/hippo-conformance/main.go", "internal/conformance/conformance.go",
		"internal/conformance/conformance_test.go", "internal/guard/lifetime.go", "internal/guard/reservation.go",
		"internal/guard/run_test.go", "internal/guard/terminal.go", "internal/policy/units.go",
		"specs/behaviours/conformance.feature", "specs/behaviours/reservations.feature", "specs/behaviours/terminal.feature",
		"tests/integration/conformance_test.go", "tests/integration/pty_test.go", "tests/integration/reservation_test.go",
		"tests/support/blockers_v04.go", "tests/support/pending_v04.go", "tests/support/release_v04.go",
		"tests/support/review_v04.go", "tests/unit/config_schema2_errors_test.go", "tests/unit/conformance_test.go",
		"tests/unit/reservation_test.go",
	} {
		if !bytes.Contains(policy, []byte(required)) {
			return fmt.Errorf("artifact policy omits version-four path %q", required)
		}
	}
	command := exec.Command(policyPath)
	command.Dir = repository
	if output, runError := command.CombinedOutput(); runError != nil {
		return fmt.Errorf("artifact policy: %s: %w", bytes.TrimSpace(output), runError)
	}

	return nil
}

func invokeReleaseValidatorCustomV04(repository, output, commit, toolchain string, extra map[string]string) error {
	validatorPath := filepath.Join(repository, "tests", "artifacts", "release-assets.sh")
	validator := exec.Command(validatorPath, output, releaseFixtureVersion, commit)
	validator.Dir = repository
	if toolchain != "" {
		validator.Env = replaceEnvironmentV04(append([]string{}, os.Environ()...), "PATH", toolchain+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if validator.Env == nil && len(extra) != 0 {
		validator.Env = append([]string{}, os.Environ()...)
	}
	for name, value := range extra {
		validator.Env = replaceEnvironmentV04(validator.Env, name, value)
	}
	combined, err := validator.CombinedOutput()
	if err != nil {
		return fmt.Errorf("release asset validation: %s: %w", bytes.TrimSpace(combined), err)
	}

	return nil
}

func invokeReleaseValidatorEnvironmentV04(repository, output, commit, toolchain string) error {
	return invokeReleaseValidatorCustomV04(repository, output, commit, toolchain, nil)
}

func invokeReleaseValidatorV04(repository, output, commit string) error {
	return invokeReleaseValidatorEnvironmentV04(repository, output, commit, "")
}

func installReleaseAwareFakeGoV04(root, commit string) (string, error) {
	bin := filepath.Join(root, "fake-release-bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		return "", err
	}
	realGo, err := exec.LookPath("go")
	if err != nil {
		return "", err
	}
	expectedJSON := fmt.Sprintf(`{"schemaVersion":1,"version":%q,"commit":%q}`, releaseFixtureVersion, commit)
	binaryScript := "#!/bin/sh\nprintf '%s\\n' " + shellQuoteV04(expectedJSON) + "\n"
	script := `#!/bin/sh
set -eu
if [ "$1" = env ]; then
	if [ "$2" = GOOS ]; then printf '%s\n' darwin; else printf '%s\n' amd64; fi
	exit 0
fi
if [ "$1" = version ] && [ "$2" = -m ]; then
	printf '%s\n' "$3: go1.25"
	printf '\tbuild\tvcs.revision=%s\n' ` + shellQuoteV04(commit) + `
	printf '\tbuild\tvcs.modified=false\n'
	exit 0
fi
if [ "$1" = run ]; then
	exec ` + shellQuoteV04(realGo) + ` "$@"
fi
output=
while [ "$#" -gt 0 ]; do
	if [ "$1" = -o ]; then
		shift
		output=$1
		break
	fi
	shift
done
test -n "$output"
printf '%s' ` + shellQuoteV04(binaryScript) + ` >"$output"
chmod 755 "$output"
`
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o700); err != nil {
		return "", err
	}

	return bin, nil
}

func buildValidFakeReleaseAssetsV04(root, name string) (string, string, string, string, error) {
	repository, head, err := cleanReleaseFixtureV04(filepath.Join(root, name))
	if err != nil {
		return "", "", "", "", err
	}
	toolchain, err := installReleaseAwareFakeGoV04(filepath.Join(root, name+"-tool"), head)
	if err != nil {
		return "", "", "", "", err
	}
	output := filepath.Join(root, name+"-assets")
	if err = invokeReleaseBuilderV04(repository, releaseFixtureVersion, head, output, toolchain); err != nil {
		return "", "", "", "", err
	}

	return repository, head, output, toolchain, nil
}

func buildFakeReleaseAssetsV04(root, name string) (string, string, string, error) {
	repository, head, err := cleanReleaseFixtureV04(filepath.Join(root, name))
	if err != nil {
		return "", "", "", err
	}
	fakeGo, err := installFakeGoV04(filepath.Join(root, name+"-tool"))
	if err != nil {
		return "", "", "", err
	}
	output := filepath.Join(root, name+"-assets")
	if err = invokeReleaseBuilderV04(repository, releaseFixtureVersion, head, output, fakeGo); err != nil {
		return "", "", "", err
	}

	return repository, head, output, nil
}

func requireV04ReleaseAssetSet(root string) error {
	repository, head, output, err := buildFakeReleaseAssetsV04(root, "asset-set")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(output, "unexpected.asset"), []byte("unexpected\n"), 0o600); err != nil {
		return err
	}
	if err = invokeReleaseValidatorV04(repository, output, head); err == nil {
		return errors.New("release validator accepted an extra asset outside the exact platform set")
	}

	return nil
}

func regenerateChecksumsV04(output string) error {
	command := exec.Command("sh", "-c", `if command -v sha256sum >/dev/null 2>&1; then sha256sum hippo_*.tar.gz >checksums.txt; else shasum -a 256 hippo_*.tar.gz >checksums.txt; fi`)
	command.Dir = output
	combined, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("regenerate checksums: %s: %w", bytes.TrimSpace(combined), err)
	}

	return nil
}

func requireV04ReleaseArchiveMember(root string) error {
	repository, head, output, err := buildFakeReleaseAssetsV04(root, "archive-member")
	if err != nil {
		return err
	}
	staging := filepath.Join(root, "bad-archive")
	if err = os.MkdirAll(staging, 0o700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(staging, "hippo"), []byte("not executable\n"), 0o600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(staging, "extra"), []byte("extra\n"), 0o600); err != nil {
		return err
	}
	archive := filepath.Join(output, "hippo_"+releaseFixtureVersion+"_linux_amd64.tar.gz")
	command := exec.Command("tar", "-C", staging, "-czf", archive, "hippo", "extra")
	if combined, archiveError := command.CombinedOutput(); archiveError != nil {
		return fmt.Errorf("create invalid archive: %s: %w", bytes.TrimSpace(combined), archiveError)
	}
	if err = regenerateChecksumsV04(output); err != nil {
		return err
	}
	if err = invokeReleaseValidatorV04(repository, output, head); err == nil {
		return errors.New("release validator accepted extra or non-executable archive members")
	}

	return nil
}

func requireV04ReleaseArchiveLink(root string) error {
	repository, head, err := cleanReleaseFixtureV04(filepath.Join(root, "archive-link"))
	if err != nil {
		return err
	}
	output := filepath.Join(root, "archive-link-assets")
	if err = invokeReleaseBuilderV04(repository, releaseFixtureVersion, head, output, ""); err != nil {
		return err
	}
	nativeTargetCommand := exec.Command("go", "env", "GOOS", "GOARCH")
	platform, err := nativeTargetCommand.Output()
	if err != nil {
		return err
	}
	fields := strings.Fields(string(platform))
	if len(fields) != 2 {
		return errors.New("native Go platform is unavailable")
	}
	nativeTarget := fields[0] + "_" + fields[1]
	nativeArchive := filepath.Join(output, "hippo_"+releaseFixtureVersion+"_"+nativeTarget+".tar.gz")
	privateTarget := filepath.Join(root, "private-host-target")
	for _, fixture := range []struct {
		name     string
		typeFlag byte
		linkName string
	}{
		{name: "symbolic link", typeFlag: tar.TypeSymlink, linkName: privateTarget},
		{name: "hard link", typeFlag: tar.TypeLink, linkName: privateTarget},
		{name: "FIFO", typeFlag: tar.TypeFifo},
	} {
		if err = writeUnsafeReleaseArchiveV04(nativeArchive, fixture.typeFlag, fixture.linkName); err != nil {
			return fmt.Errorf("create %s archive: %w", fixture.name, err)
		}
		if err = regenerateChecksumsV04(output); err != nil {
			return err
		}
		validationError := invokeReleaseValidatorV04(repository, output, head)
		if validationError == nil {
			return fmt.Errorf("release validator accepted a one-member %s archive", fixture.name)
		}
		if strings.Contains(validationError.Error(), privateTarget) {
			return fmt.Errorf("release validator exposed a host target for %s", fixture.name)
		}
	}

	return nil
}

func rewriteReleaseArchiveOwnershipV04(path string) error {
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	compressed, err := gzip.NewReader(input)
	if err != nil {
		_ = input.Close()

		return err
	}
	reader := tar.NewReader(compressed)
	header, err := reader.Next()
	if err != nil {
		_ = compressed.Close()
		_ = input.Close()

		return err
	}
	content, err := io.ReadAll(reader)
	closeError := errors.Join(compressed.Close(), input.Close())
	if err != nil {
		return errors.Join(err, closeError)
	}
	if closeError != nil {
		return closeError
	}
	header.Uid = 123
	header.Gid = 456
	header.Uname = "builder"
	header.Gname = "staff"
	header.Format = tar.FormatPAX
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	encoded := gzip.NewWriter(output)
	archive := tar.NewWriter(encoded)
	if err = archive.WriteHeader(header); err == nil {
		_, err = archive.Write(content)
	}
	return errors.Join(err, archive.Close(), encoded.Close(), output.Close())
}

func requireV04ReleaseArchiveOwnership(root string) error {
	repository, head, output, toolchain, err := buildValidFakeReleaseAssetsV04(root, "archive-ownership")
	if err != nil {
		return err
	}
	if err = invokeReleaseValidatorEnvironmentV04(repository, output, head, toolchain); err != nil {
		return fmt.Errorf("valid ownership fixture failed before mutation: %w", err)
	}
	archive := filepath.Join(output, "hippo_"+releaseFixtureVersion+"_linux_amd64.tar.gz")
	if err = rewriteReleaseArchiveOwnershipV04(archive); err != nil {
		return err
	}
	if err = regenerateChecksumsV04(output); err != nil {
		return err
	}
	if err = invokeReleaseValidatorEnvironmentV04(repository, output, head, toolchain); err == nil {
		return errors.New("release validator accepted noncanonical archive ownership")
	}
	if !strings.Contains(err.Error(), "archive ownership metadata must be normalized") {
		return fmt.Errorf("release validator rejected the ownership fixture for the wrong reason: %w", err)
	}

	return nil
}

func requireV04ReleaseSpacedPath(root string) error {
	repository, head, output, toolchain, err := buildValidFakeReleaseAssetsV04(filepath.Join(root, "directory with spaces"), "quoted-native")
	if err != nil {
		return err
	}

	temporaryRoot := filepath.Join(root, "validator temporary with spaces")
	if err = os.MkdirAll(temporaryRoot, 0o700); err != nil {
		return err
	}

	return invokeReleaseValidatorCustomV04(repository, output, head, toolchain, map[string]string{"TMPDIR": temporaryRoot})
}

func writeUnsafeReleaseArchiveV04(path string, typeFlag byte, linkName string) (returnError error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { returnError = errors.Join(returnError, file.Close()) }()
	compressed := gzip.NewWriter(file)
	defer func() { returnError = errors.Join(returnError, compressed.Close()) }()
	archive := tar.NewWriter(compressed)
	defer func() { returnError = errors.Join(returnError, archive.Close()) }()

	return archive.WriteHeader(&tar.Header{
		Name: "hippo", Mode: 0o755, Typeflag: typeFlag, Linkname: linkName, Format: tar.FormatPAX,
	})
}

func requireV04ReleaseBinaryIdentity(root string) error {
	repository, head, fakeOutput, err := buildFakeReleaseAssetsV04(root, "binary-identity-invalid")
	if err != nil {
		return err
	}
	if err = invokeReleaseValidatorV04(repository, fakeOutput, head); err == nil {
		return errors.New("release validator accepted binaries without VCS and native version identity")
	}
	realRepository, realHead, err := cleanReleaseFixtureV04(filepath.Join(root, "binary-identity-valid"))
	if err != nil {
		return err
	}
	realOutput := filepath.Join(root, "binary-identity-valid-assets")
	if err = invokeReleaseBuilderV04(realRepository, releaseFixtureVersion, realHead, realOutput, ""); err != nil {
		return err
	}

	return invokeReleaseValidatorV04(realRepository, realOutput, realHead)
}

func workflowIdentityGateV04(repository, tag, eventCommit string) error {
	command := exec.Command("git", "fetch", "--quiet", "--no-tags", "origin", "main:refs/remotes/origin/main")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("fetch origin main: %s: %w", bytes.TrimSpace(output), err)
	}
	peeled, err := runGitV04(repository, "rev-parse", tag+"^{}")
	if err != nil {
		return err
	}
	commit := strings.TrimSpace(string(peeled))
	if commit != eventCommit {
		return errors.New("peeled tag does not equal event commit")
	}
	_, err = runGitV04(repository, "merge-base", "--is-ancestor", commit, "origin/main")

	return err
}

func initializeWorkflowHistoryV04(root string) (string, error) {
	repository, _, err := cleanReleaseFixtureV04(filepath.Join(root, "workflow"))
	if err != nil {
		return "", err
	}
	origin := filepath.Join(root, "origin.git")
	if output, initError := exec.Command("git", "init", "-q", "--bare", origin).CombinedOutput(); initError != nil {
		return "", fmt.Errorf("initialize origin: %s: %w", bytes.TrimSpace(output), initError)
	}
	if _, err = runGitV04(repository, "remote", "add", "origin", origin); err != nil {
		return "", err
	}
	if _, err = runGitV04(repository, "push", "-q", "-u", "origin", "main"); err != nil {
		return "", err
	}

	return repository, nil
}

func requireV04ReleaseTagPeeling(root string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	workflow, err := os.ReadFile(filepath.Join(moduleRoot, ".github", "workflows", "release.yml"))
	if err != nil {
		return err
	}
	text := string(workflow)
	if !strings.Contains(text, `$GITHUB_REF_NAME^{}`) {
		return errors.New("release workflow does not peel the release tag")
	}
	repository, err := initializeWorkflowHistoryV04(root)
	if err != nil {
		return err
	}
	if _, err = runGitV04(repository, "tag", "reachable-lightweight"); err != nil {
		return err
	}
	// An annotated tag records a tagger, so it needs an identity the same way a
	// commit does. Supplying it per invocation keeps the fixture working on a
	// runner with no ambient identity without configuring one anywhere.
	if _, err = runGitV04(
		repository, "-c", "user.name=HIPPO fixture", "-c", "user.email=fixture@example.invalid",
		"tag", "-a", "reachable-annotated", "-m", "reachable",
	); err != nil {
		return err
	}
	for _, tag := range []string{"reachable-lightweight", "reachable-annotated"} {
		peeled, peelError := runGitV04(repository, "rev-parse", tag+"^{}")
		if peelError != nil {
			return peelError
		}
		if err = workflowIdentityGateV04(repository, tag, strings.TrimSpace(string(peeled))); err != nil {
			return fmt.Errorf("reachable tag %s rejected: %w", tag, err)
		}
	}

	return nil
}

func requireV04ReleaseMainReachability(root string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	workflow, err := os.ReadFile(filepath.Join(moduleRoot, ".github", "workflows", "release.yml"))
	if err != nil {
		return err
	}
	if !bytes.Contains(workflow, []byte("merge-base --is-ancestor")) || !bytes.Contains(workflow, []byte("origin/main")) {
		return errors.New("release workflow omits the origin-main ancestry gate")
	}
	repository, err := initializeWorkflowHistoryV04(root)
	if err != nil {
		return err
	}
	if _, err = runGitV04(repository, "tag", "reachable"); err != nil {
		return err
	}
	if _, err = appendFixtureCommitV04(repository, "side"); err != nil {
		return err
	}
	if _, err = runGitV04(repository, "tag", "unreachable"); err != nil {
		return err
	}
	reachable, err := runGitV04(repository, "rev-parse", "reachable^{}")
	if err != nil {
		return err
	}
	unreachable, err := runGitV04(repository, "rev-parse", "unreachable^{}")
	if err != nil {
		return err
	}
	if err = workflowIdentityGateV04(repository, "reachable", strings.TrimSpace(string(reachable))); err != nil {
		return fmt.Errorf("reachable peeled tag rejected: %w", err)
	}
	if err = workflowIdentityGateV04(repository, "unreachable", strings.TrimSpace(string(unreachable))); err == nil {
		return errors.New("unreachable peeled tag passed the release identity gate")
	}

	return nil
}

func requireV04ReleaseEventCommitMismatch(root string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	workflow, err := os.ReadFile(filepath.Join(moduleRoot, ".github", "workflows", "release.yml"))
	if err != nil {
		return err
	}
	if !bytes.Contains(workflow, []byte(`test "$tag_commit" = "$GITHUB_SHA"`)) {
		return errors.New("release workflow omits peeled-tag equality to the event commit")
	}
	repository, err := initializeWorkflowHistoryV04(root)
	if err != nil {
		return err
	}
	if _, err = runGitV04(repository, "tag", "release-event-mismatch"); err != nil {
		return err
	}
	tagCommit, err := runGitV04(repository, "rev-parse", "release-event-mismatch^{}")
	if err != nil {
		return err
	}
	eventCommit, err := appendFixtureCommitV04(repository, "later-event")
	if err != nil {
		return err
	}
	if _, err = runGitV04(repository, "push", "-q", "origin", "main"); err != nil {
		return err
	}
	if strings.TrimSpace(string(tagCommit)) == eventCommit {
		return errors.New("event mismatch fixture did not diverge")
	}
	if err = workflowIdentityGateV04(repository, "release-event-mismatch", eventCommit); err == nil {
		return errors.New("release identity gate accepted a different reachable event commit")
	}

	return nil
}

func workflowReleaseSectionV04(path, job string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(content)
	if job == "" {
		return text, nil
	}
	index := strings.Index(text, "  "+job+":")
	if index < 0 {
		return "", fmt.Errorf("workflow job %q is missing", job)
	}

	return text[index:], nil
}

func requireV04ReleaseCacheDisabled(string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	checks := []struct {
		path string
		job  string
	}{
		{path: filepath.Join(moduleRoot, ".github", "workflows", "release.yml")},
		{path: filepath.Join(moduleRoot, ".github", "workflows", "ci.yml"), job: "release-build"},
	}
	for _, check := range checks {
		section, readError := workflowReleaseSectionV04(check.path, check.job)
		if readError != nil {
			return readError
		}
		setupIndex := strings.Index(section, "uses: actions/setup-go@")
		if setupIndex < 0 {
			return errors.New("release asset job does not configure Go")
		}
		afterSetup := section[setupIndex:]
		if nextStep := strings.Index(afterSetup[1:], "\n      - "); nextStep >= 0 {
			afterSetup = afterSetup[:nextStep+1]
		}
		if !strings.Contains(afterSetup, "cache: false") {
			return errors.New("release asset job leaves setup-go implicit caching enabled")
		}
	}

	return nil
}

func requireV04CIValidatorCommit(string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	section, err := workflowReleaseSectionV04(filepath.Join(moduleRoot, ".github", "workflows", "ci.yml"), "release-build")
	if err != nil {
		return err
	}
	builder := ciReleaseBuilderCommand.FindStringSubmatch(section)
	validator := ciReleaseValidatorCommand.FindStringSubmatch(section)
	if builder == nil || validator == nil {
		return errors.New("CI release builder and validator do not receive the same exact commit")
	}
	if builder[1] != validator[1] {
		return fmt.Errorf(
			"CI release builder and validator disagree on the release version: builder %q validator %q",
			builder[1], validator[1],
		)
	}
	// Pinning the literal placeholder here would let the smoke build drift away
	// from the identity rule the real release enforces. Requiring an exact
	// version instead keeps this job exercising the same validation a publish
	// does, which is how a non-exact placeholder was caught reaching it.
	if !exactReleaseVersion.MatchString(builder[1]) {
		return fmt.Errorf("CI release smoke build uses a non-exact release version %q", builder[1])
	}

	return nil
}

func requireV04ReleaseStorageNeutrality(string) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	workflow, err := os.ReadFile(filepath.Join(moduleRoot, ".github", "workflows", "release.yml"))
	if err != nil {
		return err
	}
	text := string(workflow)
	for _, forbidden := range []string{"actions/upload-artifact", "actions/cache", "packages:"} {
		if strings.Contains(text, forbidden) {
			return fmt.Errorf("release workflow adds forbidden Actions storage surface %q", forbidden)
		}
	}
	if err = requireV04ReleaseCacheDisabled(""); err != nil {
		return err
	}

	return nil
}
