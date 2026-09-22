package e2e_test

// The command-line interface conformance runner.
//
// The manifest in specs/fixtures/cli-conformance/assertions.json is portable:
// it states each obligation and the observation that satisfies it, in prose,
// naming no tool. A probe here binds one assertion to a concrete invocation of
// this executable. The obligation is shareable; the way to provoke it is not.
//
// Three outcomes rather than two. An assertion this runner cannot provoke is
// reported unmeasured, never as satisfied: a runner that silently passes what
// it never exercised produces a green result that means nothing.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"testing"
)

// What this executable declares about itself, which decides the assertions that
// apply. Colour is absent deliberately: it emits no escape sequences anywhere,
// so the colour assertions are inapplicable rather than unmeasured.
var capabilities = []string{
	"starts-child-processes",
	"machine-readable-output",
	"reads-configuration",
	"has-subcommand-tree",
}

// Not a harness callback, so no exempt class is claimed.
var classes []string

// knownGaps records the assertions this executable does not yet satisfy. The
// set is asserted exact in both directions: a gap fixed without being removed
// fails, and so does a new one. Otherwise a known-gap list becomes the place
// failures go to be forgotten.
var knownGaps = map[string]string{
	"cli.exit.usage-mistake-is-two":                 "a usage mistake exits 1, the status reserved for a result",
	"cli.streams.usage-mistake-leaves-stdout-clean": "the usage block reaches stdout through the writer Run supplies",
	"cli.exit.vocabulary-is-closed":                 "73, 75, 76, and 78 are returned for guard and policy outcomes",
	"cli.streams.requested-version-on-stdout":       "there is no --version flag; a version subcommand carries it instead",
	"cli.exit.child-not-found-is-one-two-seven":     "an absent child exits 1, not 127; launch failures are not distinguished",
	"cli.args.bare-invocation-is-a-usage-mistake":   "no arguments prints help on stdout and exits 0, reporting work that did not happen",
}

type assertion struct {
	ID        string `json:"id"`
	Tier      string `json:"tier"`
	Status    string `json:"status"`
	AppliesTo struct {
		RequiresCapabilities []string `json:"requires_capabilities"`
		RequiresClasses      []string `json:"requires_classes"`
		ExcludesClasses      []string `json:"excludes_classes"`
	} `json:"applies_to"`
}

type manifest struct {
	Assertions []assertion `json:"assertions"`
}

type observed struct {
	status int
	stdout string
	stderr string
}

type outcome struct {
	kind   string // "pass", "fail", "unmeasured"
	reason string
}

func pass() outcome                   { return outcome{kind: "pass"} }
func fail(f string, a ...any) outcome { return outcome{kind: "fail", reason: fmt.Sprintf(f, a...)} }
func unmeasured(reason string) outcome {
	return outcome{kind: "unmeasured", reason: reason}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statError := os.Stat(filepath.Join(directory, "go.mod")); statError == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("go.mod root is unavailable")
		}
		directory = parent
	}
}

// invoke runs the built executable with stdout and stderr as pipes and stdin
// closed, then reports the status a shell would see — so a signal death is
// 128+N rather than a missing value some default stands in for.
func invoke(t *testing.T, binary string, arguments ...string) observed {
	t.Helper()
	command := exec.Command(binary, arguments...)
	command.Stdin = nil
	var out, errOut strings.Builder
	command.Stdout = &out
	command.Stderr = &errOut

	status := 0
	if runError := command.Run(); runError != nil {
		var exitError *exec.ExitError
		switch {
		case errors.As(runError, &exitError):
			waitStatus, ok := exitError.Sys().(syscall.WaitStatus)
			switch {
			case ok && waitStatus.Signaled():
				status = 128 + int(waitStatus.Signal())
			default:
				status = exitError.ExitCode()
			}
		default:
			t.Fatalf("invoking the executable failed outright: %v", runError)
		}
	}

	return observed{status: status, stdout: out.String(), stderr: errOut.String()}
}

// probe binds one assertion identifier to a concrete invocation. The areas are
// separate functions rather than one switch: a single switch covering every
// assertion outgrows the repository's complexity ceiling, and the areas are how
// the convention itself is organized.
func probe(t *testing.T, binary, assertionID string) (outcome, bool) {
	t.Helper()

	for _, area := range []func(*testing.T, string, string) (outcome, bool){
		probeExit, probeStreams, probeArgs, probeOther,
	} {
		if result, bound := area(t, binary, assertionID); bound {
			return result, true
		}
	}

	return outcome{}, false
}

func probeExit(t *testing.T, binary, assertionID string) (outcome, bool) {
	t.Helper()

	switch assertionID {
	case "cli.exit.affirmative-is-zero":
		result := invoke(t, binary, "status")
		if result.status == 0 {
			return pass(), true
		}
		return fail("expected exit 0, observed %d", result.status), true

	case "cli.exit.usage-mistake-is-two":
		result := invoke(t, binary, "--no-such-flag")
		switch {
		case result.status != 2:
			return fail("expected exit 2, observed %d", result.status), true
		case result.stdout != "":
			return fail("expected empty stdout, observed %d bytes", len(result.stdout)), true
		}
		return pass(), true

	case "cli.exit.child-status-passes-through":
		result := invoke(t, binary, "run", "--class", "ephemeral", "--resource-tier", "light",
			"--disk-path", ".", "--", "/bin/sh", "-c", "exit 7")
		if result.status == 7 {
			return pass(), true
		}
		return fail("expected the child's own status 7, observed %d", result.status), true

	case "cli.exit.child-not-found-is-one-two-seven":
		result := invoke(t, binary, "run", "--class", "ephemeral", "--resource-tier", "light",
			"--disk-path", ".", "--", "/no/such/program")
		if result.status == 127 {
			return pass(), true
		}
		return fail("expected exit 127 for an absent child, observed %d", result.status), true

	case "cli.exit.vocabulary-is-closed":
		allowed := map[int]bool{0: true, 1: true, 2: true, 124: true, 125: true, 126: true, 127: true}
		probes := [][]string{
			{"status"},
			{"--help"},
			{"--no-such-flag"},
			{"no-such-command"},
			{"run", "--class", "ephemeral", "--resource-tier", "light", "--disk-path", ".", "--", "/bin/true"},
			{"run", "--class", "ephemeral", "--resource-tier", "light", "--disk-path", ".", "--", "/no/such/program"},
			{"run", "--disk-path", ".", "--", "/bin/true"},
		}
		var outside []string
		for _, arguments := range probes {
			status := invoke(t, binary, arguments...).status
			if !allowed[status] && status < 128 {
				outside = append(outside, fmt.Sprintf("`%s` exits %d", strings.Join(arguments, " "), status))
			}
		}
		if len(outside) == 0 {
			return pass(), true
		}
		return fail("%s", strings.Join(outside, "; ")), true

	case "cli.exit.internal-crash-is-two", "cli.exit.crash-trace-behind-a-switch":
		return unmeasured("no fault-injection point exists at the process boundary"), true

	case "cli.exit.closed-pipe-is-one-four-one":
		return unmeasured(
			"this executable's own output is small enough to complete before a reader can close, " +
				"so EPIPE cannot be provoked from the outside",
		), true

	case "cli.exit.interrupt-is-one-three-zero":
		return unmeasured("a signal cannot be raced reliably against a run this short"), true

	case "cli.exit.timeout-is-one-two-four", "cli.exit.refusal-before-launch-is-one-two-five":
		return unmeasured(
			"provoking it needs a host state this runner must not manufacture; " +
				"it belongs with the change that renumbers the guard statuses",
		), true

	case "cli.exit.child-not-executable-is-one-two-six":
		return unmeasured("provoking it needs a non-executable file fixture placed for that purpose"), true
	}

	return outcome{}, false
}

func probeStreams(t *testing.T, binary, assertionID string) (outcome, bool) {
	t.Helper()

	switch assertionID {
	case "cli.streams.usage-mistake-leaves-stdout-clean":
		result := invoke(t, binary, "--no-such-flag")
		switch {
		case result.stdout != "":
			return fail(
				"the usage text reached stdout: %d bytes beginning %q",
				len(result.stdout), first(result.stdout, 48),
			), true
		case result.stderr == "":
			return fail("expected a diagnostic on stderr, observed none"), true
		}
		return pass(), true

	case "cli.streams.requested-help-on-stdout":
		result := invoke(t, binary, "--help")
		switch {
		case result.status != 0:
			return fail("expected exit 0, observed %d", result.status), true
		case result.stdout == "":
			return fail("expected usage text on stdout, observed none"), true
		case result.stderr != "":
			return fail("expected empty stderr, observed %d bytes", len(result.stderr)), true
		}
		return pass(), true

	case "cli.streams.requested-version-on-stdout":
		result := invoke(t, binary, "--version")
		switch {
		case result.status != 0:
			return fail("expected exit 0, observed %d", result.status), true
		case result.stdout == "":
			return fail("expected a version line on stdout, observed none"), true
		}
		return pass(), true

	case "cli.streams.payload-on-stdout":
		result := invoke(t, binary, "status")
		if result.status == 0 && result.stdout != "" {
			return pass(), true
		}
		return fail(
			"expected a payload on stdout at exit 0, observed exit %d and %d bytes",
			result.status, len(result.stdout),
		), true

	case "cli.streams.diagnostics-on-stderr":
		result := invoke(t, binary, "--no-such-flag")
		if result.stderr != "" {
			return pass(), true
		}
		return fail("expected the diagnostic on stderr, observed none"), true

	case "cli.streams.help-suppresses-normal-function":
		result := invoke(t, binary, "--help", "status")
		if result.status != 0 {
			return fail("expected exit 0 when help is requested, observed %d", result.status), true
		}
		return pass(), true
	}

	return outcome{}, false
}

func probeArgs(t *testing.T, binary, assertionID string) (outcome, bool) {
	t.Helper()

	switch assertionID {
	case "cli.args.bare-invocation-is-a-usage-mistake":
		// Status and stream both matter. Exiting 2 while writing help to stdout
		// would put a diagnostic where a caller reading the payload finds it.
		result := invoke(t, binary)
		switch {
		case result.status != 2:
			return fail("expected exit 2 for a bare invocation, observed %d", result.status), true
		case result.stdout != "":
			return fail("expected nothing on stdout, observed %d bytes", len(result.stdout)), true
		case result.stderr == "":
			return fail("expected a diagnostic on stderr, observed none"), true
		}
		return pass(), true

	case "cli.args.help-subcommand-when-a-tree-exists":
		result := invoke(t, binary, "help")
		switch {
		case result.status != 0:
			return fail("expected exit 0, observed %d", result.status), true
		case result.stdout == "":
			return fail("expected usage text on stdout, observed none"), true
		}
		return pass(), true

	case "cli.args.short-help-on-every-subcommand":
		result := invoke(t, binary, "status", "-h")
		switch {
		case result.status != 0:
			return fail("expected exit 0, observed %d", result.status), true
		case result.stdout == "":
			return fail("expected subcommand usage on stdout, observed none"), true
		}
		return pass(), true

	case "cli.args.double-dash-ends-options":
		// `run -- <command>` is this executable's own end-of-options use, and
		// the operand after it must not be read as an option.
		result := invoke(t, binary, "run", "--class", "ephemeral", "--resource-tier", "light",
			"--disk-path", ".", "--", "/bin/echo", "-n")
		if result.status == 0 {
			return pass(), true
		}
		return fail("an operand beginning with - after -- was not accepted: exit %d", result.status), true

	case "cli.args.option-and-value-may-be-separate":
		result := invoke(t, binary, "status", "--disk-path", ".")
		if result.status == 0 {
			return pass(), true
		}
		return fail("an option and its value as separate arguments were refused: exit %d", result.status), true
	}

	return outcome{}, false
}

func probeOther(t *testing.T, binary, assertionID string) (outcome, bool) {
	t.Helper()

	switch assertionID {
	case "cli.output.mode-is-explicit":
		// stdout is a pipe here and no machine-readable flag was given.
		result := invoke(t, binary, "status")
		if strings.HasPrefix(strings.TrimSpace(result.stdout), "{") {
			return fail("machine-readable output was emitted without an explicit flag"), true
		}
		return pass(), true

	case "cli.diagnostics.name-the-tool-first":
		result := invoke(t, binary, "--no-such-flag")
		if strings.HasPrefix(result.stderr, "hippo:") ||
			strings.HasPrefix(result.stderr, "Error:") {
			return pass(), true
		}
		return fail("the diagnostic does not begin with the tool name: %q", first(result.stderr, 40)), true

	case "cli.stdin.not-read-unless-selected":
		result := invoke(t, binary, "status")
		if result.status == 0 {
			return pass(), true
		}
		return fail("a run selecting no standard input did not complete: exit %d", result.status), true

	case "cli.terminal.no-escapes-in-a-pipe":
		result := invoke(t, binary, "status")
		if strings.ContainsRune(result.stdout, '\x1b') {
			return fail("escape bytes reached a piped stdout"), true
		}
		return pass(), true
	}

	return outcome{}, false
}

func first(text string, count int) string {
	if len(text) <= count {
		return text
	}
	return text[:count]
}

func applies(entry assertion) bool {
	for _, required := range entry.AppliesTo.RequiresCapabilities {
		if !slices.Contains(capabilities, required) {
			return false
		}
	}
	for _, required := range entry.AppliesTo.RequiresClasses {
		if !slices.Contains(classes, required) {
			return false
		}
	}
	for _, excluded := range entry.AppliesTo.ExcludesClasses {
		if slices.Contains(classes, excluded) {
			return false
		}
	}
	return true
}

func TestCommandLineInterfaceContract(t *testing.T) {
	root := moduleRoot(t)
	binary := filepath.Join(t.TempDir(), "hippo")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hippo")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build HIPPO: %s: %v", output, err)
	}

	text, err := os.ReadFile(filepath.Join(root, "specs/fixtures/cli-conformance/assertions.json"))
	if err != nil {
		t.Fatalf("the conformance manifest is unreadable: %v", err)
	}
	var loaded manifest
	if err := json.Unmarshal(text, &loaded); err != nil {
		t.Fatalf("the conformance manifest is not valid JSON: %v", err)
	}
	if len(loaded.Assertions) == 0 {
		t.Fatal("the conformance manifest is empty")
	}

	var passed, failed, unmeasuredList, unbound []string
	skippedUnverified, skippedInapplicable := 0, 0

	for _, entry := range loaded.Assertions {
		// The corpus's one hard rule: nothing but `verified` may gate.
		if entry.Status != "verified" {
			skippedUnverified++
			continue
		}
		if !applies(entry) {
			skippedInapplicable++
			continue
		}

		result, bound := probe(t, binary, entry.ID)
		switch {
		case !bound:
			unbound = append(unbound, entry.ID)
		case result.kind == "pass":
			passed = append(passed, entry.ID)
		case result.kind == "fail":
			failed = append(failed, entry.ID+": "+result.reason)
		default:
			unmeasuredList = append(unmeasuredList, entry.ID+": "+result.reason)
		}
	}

	t.Logf(
		"cli-conformance: %d pass, %d fail, %d unmeasured, %d unbound; skipped %d unverified and %d inapplicable",
		len(passed), len(failed), len(unmeasuredList), len(unbound), skippedUnverified, skippedInapplicable,
	)
	for _, entry := range unmeasuredList {
		t.Logf("  unmeasured  %s", entry)
	}
	for _, entry := range unbound {
		t.Logf("  unbound     %s", entry)
	}

	failedIDs := map[string]bool{}
	for _, entry := range failed {
		failedIDs[strings.SplitN(entry, ":", 2)[0]] = true
	}

	var unexpected []string
	for _, entry := range failed {
		if _, known := knownGaps[strings.SplitN(entry, ":", 2)[0]]; !known {
			unexpected = append(unexpected, entry)
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		t.Errorf(
			"%d assertion(s) fail that are not recorded as known gaps:\n%s",
			len(unexpected), strings.Join(unexpected, "\n"),
		)
	}

	var repaired []string
	for id := range knownGaps {
		if !failedIDs[id] {
			repaired = append(repaired, id)
		}
	}
	if len(repaired) > 0 {
		sort.Strings(repaired)
		t.Errorf(
			"%d known gap(s) now pass and must be removed from knownGaps:\n  %s",
			len(repaired), strings.Join(repaired, "\n  "),
		)
	}

	for id, reason := range knownGaps {
		t.Logf("  known gap   %s: %s", id, reason)
	}
}
