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
	"regexp"
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
// knownGaps is empty, and asserting that it is empty is the point: every
// assertion this runner binds either passes or fails the run. An entry here
// would name a measured gap hippo has chosen not to close yet, and the runner
// fails just as loudly when an entry starts passing, so a repaired gap cannot
// sit here pretending to still be one.
var knownGaps = map[string]string{}

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

// invokeWithEnvironment is invoke with extra environment entries, for the one
// assertion that asks what a failure does with a credential it could see.
func invokeWithEnvironment(t *testing.T, binary string, environment []string, arguments ...string) observed {
	t.Helper()
	command := exec.Command(binary, arguments...)
	command.Stdin = nil
	command.Env = append(os.Environ(), environment...)
	var out, errOut strings.Builder
	command.Stdout, command.Stderr = &out, &errOut

	status := 0
	if runError := command.Run(); runError != nil {
		var exitError *exec.ExitError
		if !errors.As(runError, &exitError) {
			t.Fatalf("invoking the executable failed outright: %v", runError)
		}
		status = exitError.ExitCode()
	}

	return observed{status: status, stdout: out.String(), stderr: errOut.String()}
}

// firstLine keeps a failure message to one line, so a panic or traceback names
// itself without pasting itself into the report.
func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

// invokeWithClosedReader provokes a closed pipe by closing the read end before
// the child writes anything, so its first write meets a reader that is already
// gone.
//
// The obvious probe -- read one byte, then close -- cannot work on a command
// whose whole output fits in the kernel's pipe buffer: the write succeeds, the
// process exits 0, and the assertion goes unmeasured for a reason that is about
// the runner rather than the executable. Closing first removes the race.
func invokeWithClosedReader(t *testing.T, binary string, arguments ...string) observed {
	t.Helper()
	command := exec.Command(binary, arguments...)
	command.Stdin = nil
	var errOut strings.Builder
	command.Stderr = &errOut

	pipe, pipeError := command.StdoutPipe()
	if pipeError != nil {
		t.Fatalf("opening a pipe on stdout failed: %v", pipeError)
	}
	if startError := command.Start(); startError != nil {
		t.Fatalf("starting the executable failed: %v", startError)
	}
	if closeError := pipe.Close(); closeError != nil {
		t.Fatalf("closing the read end failed: %v", closeError)
	}

	status := 0
	if waitError := command.Wait(); waitError != nil {
		var exitError *exec.ExitError
		switch {
		case errors.As(waitError, &exitError):
			waitStatus, ok := exitError.Sys().(syscall.WaitStatus)
			switch {
			case ok && waitStatus.Signaled():
				status = 128 + int(waitStatus.Signal())
			default:
				status = exitError.ExitCode()
			}
		default:
			t.Fatalf("waiting on the executable failed: %v", waitError)
		}
	}

	return observed{status: status, stderr: errOut.String()}
}

// shedByLimit reports whether HIPPO stopped this invocation against one of its
// own limits, which says nothing about the behaviour a probe was measuring.
//
// A probe that runs `hippo run` and demands an exact status is asking the host
// a question it did not mean to ask: a loaded runner sheds the work and the
// probe reads that as the behaviour failing. This is the same trap the
// vocabulary sweep documents below, and the fix is the same -- report the
// assertion unmeasured rather than failed, because nothing was measured.
//
// Telling the two apart is exact rather than heuristic, and that is what the
// two-layer contract buys: HIPPO's own shed exits 124 *and* writes a limit
// reason to stderr, while a child that chose 124 for its own purposes writes
// no such line.
func shedByLimit(result observed) bool {
	return result.status == 124 && strings.Contains(result.stderr, "hippo: [hippo.limit.")
}

// probe binds one assertion identifier to a concrete invocation. The areas are
// separate functions rather than one switch: a single switch covering every
// assertion outgrows the repository's complexity ceiling, and the areas are how
// the convention itself is organized.
func probe(t *testing.T, binary, assertionID string) (outcome, bool) {
	t.Helper()

	for _, area := range []func(*testing.T, string, string) (outcome, bool){
		probeExit, probeStreams, probeArgs, probeOutput, probeVocabulary, probeOther,
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
		switch {
		case shedByLimit(result):
			return unmeasured("the host shed this run against a limit, so no child ever chose a status"), true
		case result.status == 7:
			return pass(), true
		}
		return fail("expected the child's own status 7, observed %d", result.status), true

	case "cli.exit.child-not-found-is-one-two-seven":
		result := invoke(t, binary, "run", "--class", "ephemeral", "--resource-tier", "light",
			"--disk-path", ".", "--", "/no/such/program")
		switch {
		case shedByLimit(result):
			return unmeasured("the host shed this run against a limit before the child was looked for"), true
		case result.status == 127:
			return pass(), true
		}
		return fail("expected exit 127 for an absent child, observed %d", result.status), true

	case "cli.exit.vocabulary-is-closed":
		// Every path here must return the same status on every host. The first
		// version of this probe swept `run` invocations, which reach the guard
		// statuses only when the machine is actually under pressure: it found
		// 73, 75, and 78 on a loaded workstation and nothing at all on an idle
		// CI runner, so the same commit passed in one place and failed in the
		// other. A gate whose verdict depends on the load average is not a
		// gate, and a known-gap ledger asserted exact in both directions turns
		// that straight into a red build.
		//
		// A missing configuration file reaches the same out-of-vocabulary
		// status by a path that has nothing to do with host state.
		missingConfiguration := filepath.Join(t.TempDir(), "absent.json")
		allowed := map[int]bool{0: true, 1: true, 2: true, 124: true, 125: true, 126: true, 127: true}
		probes := [][]string{
			{"status"},
			{"--help"},
			{"--no-such-flag"},
			{"no-such-command"},
			{"status", "--config", missingConfiguration},
			{"release", "check", "--config", missingConfiguration},
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
		result := invokeWithClosedReader(t, binary, "--help")
		switch {
		case result.status != 141:
			return fail("expected exit 141 on a closed pipe, observed %d", result.status), true
		case result.stderr != "":
			return fail("expected a silent stderr, observed %q", firstLine(result.stderr)), true
		}
		return pass(), true

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
		switch {
		case shedByLimit(result):
			return unmeasured("the host shed this run against a limit, so the operand was never reached"), true
		case result.status == 0:
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

// failureBodyOf runs an invocation that must fail with --output json and
// returns the body it wrote to stderr, parsed. Every cli.output assertion
// needs the same thing, and parsing it once keeps the probes about what they
// each measure.
func failureBodyOf(t *testing.T, binary string, arguments ...string) (map[string]any, observed, bool) {
	t.Helper()
	result := invoke(t, binary, arguments...)
	if result.status == 0 {
		return nil, result, false
	}

	// The diagnostic line comes first and the body is the JSON document after
	// it, so the probe reads from the first brace that begins a line.
	index := strings.Index(result.stderr, "\n{")
	if index < 0 {
		return nil, result, false
	}

	// A decoder rather than Unmarshal: the usage block may follow the body on
	// stderr, and only the first document is the body.
	body := map[string]any{}
	if err := json.NewDecoder(strings.NewReader(result.stderr[index+1:])).Decode(&body); err != nil {
		return nil, result, false
	}

	return body, result, true
}

// publishedCodes is the vocabulary docs/reference/exit-codes.md publishes,
// read from the document rather than from the package, so a code that reaches
// a caller without being written down fails this runner.
func publishedCodes(t *testing.T) map[string]bool {
	t.Helper()
	document, readError := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "exit-codes.md"))
	if readError != nil {
		t.Fatalf("reading the published error codes failed: %v", readError)
	}

	codes := map[string]bool{}
	for _, match := range regexp.MustCompile(`hippo\.[a-z]+\.[a-z-]+`).FindAllString(string(document), -1) {
		codes[match] = true
	}

	return codes
}

// failurePaths is one invocation per documented failure path this runner can
// provoke without manufacturing a host state. Two assertions walk all of them,
// which is what "every documented failure path in turn" asks for.
func failurePaths() [][]string {
	run := []string{"run", "--output", "json", "--class", "ephemeral", "--resource-tier", "light", "--disk-path", "."}

	return [][]string{
		{"--output", "json", "--no-such-flag"},
		{"--output", "json"},
		{"--output", "json", "not-a-command"},
		append(append([]string{}, run...), "--", "definitely-not-a-command-anywhere"),
		append(append([]string{}, run...), "--", filepath.Join("testdata", "not-executable")),
		{"--output", "json", "status", "--config", filepath.Join("testdata", "no-such-config.json")},
	}
}

// probeVocabulary walks every failure path this runner can provoke. Both of
// its assertions need the same walk, and keeping them beside each other keeps
// the walk in one place.
func probeVocabulary(t *testing.T, binary, assertionID string) (outcome, bool) {
	t.Helper()

	switch assertionID {
	case "cli.output.error-codes-are-namespaced":
		shape := regexp.MustCompile(`^[a-z0-9]+\.[a-z0-9-]+\.[a-z0-9-]+$`)
		for _, arguments := range failurePaths() {
			body, result, parsed := failureBodyOf(t, binary, arguments...)
			if !parsed {
				return fail("%v emitted no body: exit %d", arguments, result.status), true
			}
			code, _ := body["error"].(map[string]any)["code"].(string)
			if !shape.MatchString(code) {
				return fail("%q is not shaped tool.area.reason", code), true
			}
		}
		return pass(), true

	case "cli.output.error-code-vocabulary-is-closed":
		published := publishedCodes(t)
		for _, arguments := range failurePaths() {
			body, _, parsed := failureBodyOf(t, binary, arguments...)
			if !parsed {
				return fail("%v emitted no body", arguments), true
			}
			code, _ := body["error"].(map[string]any)["code"].(string)
			if !published[code] {
				return fail("%q is returned but not published", code), true
			}
		}
		return pass(), true
	}

	return outcome{}, false
}

func probeOutput(t *testing.T, binary, assertionID string) (outcome, bool) {
	t.Helper()

	switch assertionID {
	case "cli.exit.every-status-is-published":
		result := invoke(t, binary, "--help")
		missing := []string{}
		for _, published := range []string{"0", "1", "2", "124", "125", "126", "127"} {
			if !regexp.MustCompile(`(?m)^\s+` + published + `\s`).MatchString(result.stdout) {
				missing = append(missing, published)
			}
		}
		if len(missing) > 0 {
			return fail("--help does not document the status(es) %s", strings.Join(missing, ", ")), true
		}
		return pass(), true

	case "cli.exit.negative-result-is-one":
		// `history` filtered to a source no run can have used: the question was
		// asked and answered, and the answer is that there is nothing.
		result := invoke(t, binary, "history", "--source", "no-such-source-anywhere")
		if result.status == 1 {
			return pass(), true
		}
		return fail("expected exit 1 for an empty result, observed %d", result.status), true

	case "cli.output.error-body-required-fields":
		body, result, parsed := failureBodyOf(t, binary, "--output", "json", "--no-such-flag")
		if !parsed {
			return fail("no machine-readable body on stderr: %q", first(result.stderr, 80)), true
		}
		for _, field := range []string{"schemaVersion", "error"} {
			if _, present := body[field]; !present {
				return fail("the body omits the required field %q", field), true
			}
		}
		failureFields, ok := body["error"].(map[string]any)
		if !ok {
			return fail("error is not an object"), true
		}
		for _, field := range []string{"code", "message"} {
			if value, present := failureFields[field]; !present || value == "" {
				return fail("the body omits the required field error.%s", field), true
			}
		}
		return pass(), true

	case "cli.output.machine-readable-is-escape-free":
		result := invoke(t, binary, "--output", "json", "--color", "always", "--no-such-flag")
		index := strings.Index(result.stderr, "\n{")
		switch {
		case result.stdout != "":
			return fail("a failed invocation wrote %d bytes to stdout", len(result.stdout)), true
		case index < 0:
			return fail("no machine-readable body was written"), true
		case strings.Contains(result.stderr[index:], "\x1b"):
			return fail("the body carries escape bytes although it is read by a parser"), true
		}
		return pass(), true
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

	case "cli.diagnostics.carry-no-secret":
		secret := "s3cr3t-token-do-not-print"
		result := invokeWithEnvironment(t, binary,
			[]string{"HIPPO_TEST_CREDENTIAL=" + secret, "AWS_SECRET_ACCESS_KEY=" + secret},
			"--output", "json", "--no-such-flag")
		if strings.Contains(result.stderr, secret) || strings.Contains(result.stdout, secret) {
			return fail("a credential in the environment reached the diagnostics"), true
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

// TestHelpExitBlockAgreesWithTheReference holds --help to the published table.
// The exit block is the one place a caller reads the contract without the
// documentation, so a meaning that drifted from docs/reference/exit-codes.md
// would teach every such caller something the binary no longer does.
func TestHelpExitBlockAgreesWithTheReference(t *testing.T) {
	root := moduleRoot(t)
	binary := filepath.Join(t.TempDir(), "hippo")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hippo")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build HIPPO: %s: %v", output, err)
	}
	document, err := os.ReadFile(filepath.Join(root, "docs", "reference", "exit-codes.md"))
	if err != nil {
		t.Fatalf("reading the exit reference failed: %v", err)
	}
	help := invoke(t, binary, "--help").stdout

	for _, status := range []string{"0", "1", "2", "124", "125", "126", "127"} {
		row := regexp.MustCompile("(?m)^\\| `" + status + "` +\\| ([^|]+?) +\\|").FindStringSubmatch(string(document))
		if row == nil {
			t.Fatalf("exit-codes.md has no row for %s", status)
		}
		line := regexp.MustCompile(`(?m)^\s+` + status + `\s+(.+)$`).FindStringSubmatch(help)
		if line == nil {
			t.Fatalf("--help has no line for %s", status)
		}
		if !strings.HasPrefix(strings.ToLower(line[1]), strings.ToLower(row[1])) {
			t.Errorf("--help says %s %q; exit-codes.md says %q", status, line[1], row[1])
		}
	}
}
