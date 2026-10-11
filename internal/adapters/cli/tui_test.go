package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

type tuiCountingCollector struct {
	calls int
}

func (collector *tuiCountingCollector) Collect(context.Context, policy.CPUState, string) (policy.Reading, error) {
	collector.calls++
	return policy.Reading{}, errors.New("unexpected host collection during help")
}

func TestTUIRootHelpDiscoversMonitoringCommand(t *testing.T) {
	var output, diagnostics bytes.Buffer
	collector := &tuiCountingCollector{}
	boundary := cli.Application{Stdout: &output, Stderr: &diagnostics, Collector: collector}
	code, err := boundary.Run(t.Context(), []string{"--help"})
	if err != nil || code != 0 || diagnostics.Len() != 0 || collector.calls != 0 {
		t.Fatalf("root help code=%d error=%v diagnostics=%q hostCalls=%d", code, err, diagnostics.String(), collector.calls)
	}
	if !strings.Contains(output.String(), "  tui ") {
		t.Fatalf("root help does not expose the TUI command: %s", output.String())
	}
}

func TestTUIRequestedHelpDescribesItsOptionsWithoutObservation(t *testing.T) {
	cases := []struct {
		name      string
		arguments []string
	}{
		{name: "plain", arguments: []string{"tui", "--help"}},
		{name: "malformed interval", arguments: []string{"tui", "--interval", "invalid", "--help"}},
		{name: "unknown flag", arguments: []string{"tui", "--help", "--no-such-flag"}},
		{name: "invalid filters", arguments: []string{"tui", "--source", "/private", "--tag", "invalid", "--help"}},
		{name: "invalid global output", arguments: []string{"--output", "XML", "tui", "--help"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var output, diagnostics bytes.Buffer
			collector := &tuiCountingCollector{}
			boundary := cli.Application{Stdout: &output, Stderr: &diagnostics, Collector: collector}
			code, err := boundary.Run(t.Context(), test.arguments)
			if err != nil || code != 0 || diagnostics.Len() != 0 || collector.calls != 0 {
				t.Fatalf("TUI help code=%d error=%v diagnostics=%q hostCalls=%d", code, err, diagnostics.String(), collector.calls)
			}
			if !strings.Contains(output.String(), "hippo tui [flags]") {
				t.Fatalf("requested TUI help resolved to a different command: %s", output.String())
			}
			for _, option := range []string{"--config", "--disk-path", "--profile", "--source", "--tag", "--interval"} {
				if !strings.Contains(output.String(), option) {
					t.Errorf("TUI help omits %s: %s", option, output.String())
				}
			}
		})
	}
}

type tuiBoundaryFixture struct {
	boundary                     cli.Application
	output, diagnostics          bytes.Buffer
	collector                    tuiCountingCollector
	capabilities, runs           int
	inputTTY, outputTTY          bool
	capabilityError, runnerError error
	interruption                 status.Interruption
	request                      application.ObservationRequest
	interval                     time.Duration
	styling, restored            bool
	contextMatches               bool
	checkContext                 func(context.Context) bool
}

func newTUIBoundaryFixture() *tuiBoundaryFixture {
	fixture := &tuiBoundaryFixture{inputTTY: true, outputTTY: true}
	fixture.boundary = cli.Application{Stdout: &fixture.output, Stderr: &fixture.diagnostics, Collector: &fixture.collector, Environment: []string{"TERM=xterm-256color"}, TerminalCapabilities: fixture.inspect, RunTUI: fixture.run}
	return fixture
}

func (fixture *tuiBoundaryFixture) inspect() (bool, bool, error) {
	fixture.capabilities++
	return fixture.inputTTY, fixture.outputTTY, fixture.capabilityError
}

func (fixture *tuiBoundaryFixture) run(ctx context.Context, request application.ObservationRequest, interval time.Duration, styling bool) (status.Interruption, error) {
	fixture.runs++
	fixture.request, fixture.interval, fixture.styling = request, interval, styling
	if fixture.checkContext != nil {
		fixture.contextMatches = fixture.checkContext(ctx)
	}
	fixture.restored = true
	return fixture.interruption, fixture.runnerError
}

func TestTUIProjectsValidatedRequestAndRefreshDefaults(t *testing.T) {
	for _, test := range []struct {
		name     string
		args     []string
		interval time.Duration
		request  application.StatusRequest
	}{
		{"default", []string{"tui"}, 5 * time.Second, application.StatusRequest{DiskPath: "."}},
		{"explicit", []string{"tui", "--interval", "1s", "--config", "custom.json", "--disk-path", "disk", "--profile", "balanced", "--source", "hippo", "--tag", "role=build", "--tag", "role=test"}, time.Second, application.StatusRequest{ConfigPath: "custom.json", DiskPath: "disk", RequestedProfile: policy.ProfileName("balanced"), Source: "hippo", Tags: []string{"role=build", "role=test"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTUIBoundaryFixture()
			ctx := t.Context()
			fixture.checkContext = func(actual context.Context) bool { return actual == ctx }
			code, err := fixture.boundary.Run(ctx, test.args)
			want, prepareError := application.PrepareObservation(test.request)
			if code != 0 || err != nil || prepareError != nil || fixture.runs != 1 || fixture.capabilities != 1 || fixture.collector.calls != 0 || fixture.interval != test.interval || !fixture.contextMatches || !reflect.DeepEqual(fixture.request, want) || fixture.diagnostics.Len() != 0 || fixture.output.Len() != 0 {
				t.Fatalf("request projection changed: code=%d error=%v calls=%d/%d interval=%v request=%#v want=%#v diagnostics=%q", code, err, fixture.capabilities, fixture.runs, fixture.interval, fixture.request, want, fixture.diagnostics.String())
			}
		})
	}
}

func TestTUIRejectsInvocationMistakesBeforeCapabilitiesOrObservation(t *testing.T) {
	for _, args := range [][]string{
		{"tui", "--interval", "999ms"},
		{"tui", "--interval", "0s"},
		{"tui", "--interval", "-1s"},
		{"tui", "--interval", "bad"},
		{"tui", "--source", "/private"},
		{"tui", "--tag", "bad"},
		{"tui", "--source", "/private", "--tag", "bad"},
		{"tui", "--refresh", "10s"},
		{"tui", "--json"},
		{"tui", "--jsonl"},
		{"tui", "unexpected"},
		{"tui", "--output", "XML"},
		{"tui", "--color", "bogus"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fixture := newTUIBoundaryFixture()
			code, err := fixture.boundary.Run(t.Context(), args)
			failure, classified := errors.AsType[status.Failure](err)
			if code != 2 || !classified || failure.Code != status.CodeArgsInvalid || fixture.capabilities != 0 || fixture.runs != 0 || fixture.collector.calls != 0 || fixture.output.Len() != 0 {
				t.Fatalf("usage refusal had effects: code=%d error=%v capabilities=%d runs=%d host=%d output=%q", code, err, fixture.capabilities, fixture.runs, fixture.collector.calls, fixture.output.String())
			}
			if len(args) == 5 && args[1] == "--source" && !strings.Contains(failure.Message, "--source") {
				t.Fatalf("filter precedence changed: %v", err)
			}
		})
	}
}

func TestTUIRefusesUnusableTerminalAndSafelyClassifiesInspectionFailure(t *testing.T) {
	for _, test := range []struct {
		name          string
		input, output bool
		term          string
		err           error
		code, calls   int
	}{
		{"input", false, true, "xterm", nil, 2, 1},
		{"output", true, false, "xterm", nil, 2, 1},
		{"dumb", true, true, "dumb", nil, 2, 0},
		{"inspection", true, true, "xterm", errors.New("private terminal inspection /fixture/private/terminal-inspection"), 125, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTUIBoundaryFixture()
			fixture.inputTTY, fixture.outputTTY, fixture.capabilityError = test.input, test.output, test.err
			fixture.boundary.Environment = []string{"TERM=" + test.term}
			code, err := fixture.boundary.Run(t.Context(), []string{"tui", "--color", "always"})
			if code != test.code || err == nil || fixture.capabilities != test.calls || fixture.runs != 0 || fixture.collector.calls != 0 || fixture.output.Len() != 0 || strings.Contains(fixture.diagnostics.String(), "/fixture/private/terminal-inspection") {
				t.Fatalf("terminal refusal code=%d error=%v calls=%d/%d diagnostics=%q", code, err, fixture.capabilities, fixture.runs, fixture.diagnostics.String())
			}
			if test.code == 2 && (!strings.Contains(fixture.diagnostics.String(), "hippo status") || !strings.Contains(fixture.diagnostics.String(), "hippo watch")) {
				t.Fatalf("terminal refusal has no actionable alternative: %q", fixture.diagnostics.String())
			}
		})
	}
}

func TestTUIHelpAvoidsTerminalCapabilitiesAndRunner(t *testing.T) {
	fixture := newTUIBoundaryFixture()
	fixture.boundary.Environment = []string{"TERM=dumb"}
	code, err := fixture.boundary.Run(t.Context(), []string{"tui", "--interval", "bad", "--help"})
	if err != nil || code != 0 || fixture.capabilities != 0 || fixture.runs != 0 || fixture.collector.calls != 0 || fixture.diagnostics.Len() != 0 || !strings.Contains(fixture.output.String(), "hippo tui [flags]") || !strings.Contains(fixture.output.String(), "default 5s") {
		t.Fatalf("help started effects or omitted refresh default: code=%d error=%v calls=%d/%d output=%q diagnostics=%q", code, err, fixture.capabilities, fixture.runs, fixture.output.String(), fixture.diagnostics.String())
	}
}

func TestTUIForwardsExistingColorPolicy(t *testing.T) {
	for _, test := range []struct {
		name, flag  string
		environment []string
		styling     bool
	}{
		{"auto", "", []string{"TERM=xterm"}, false},
		{"environment color", "", []string{"TERM=xterm", "HIPPO_COLOR=1"}, true},
		{"no color", "", []string{"TERM=xterm", "HIPPO_COLOR=1", "NO_COLOR="}, false},
		{"explicit always", "always", []string{"TERM=xterm", "NO_COLOR=1"}, true},
		{"explicit never", "never", []string{"TERM=xterm"}, false},
		{"environment never", "", []string{"TERM=xterm", "HIPPO_COLOR=never"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTUIBoundaryFixture()
			fixture.boundary.Environment = test.environment
			args := []string{"tui"}
			if test.flag != "" {
				args = append(args, "--color", test.flag)
			}
			code, err := fixture.boundary.Run(t.Context(), args)
			if code != 0 || err != nil || fixture.runs != 1 || fixture.styling != test.styling {
				t.Fatalf("color forwarding: code=%d error=%v styling=%v want=%v", code, err, fixture.styling, test.styling)
			}
		})
	}
}

func TestTUIRunnerStopsPreserveInterruptAndClassifiedFailurePriority(t *testing.T) {
	for _, test := range []struct {
		name                string
		signal, localSignal int
		failure             error
		code                int
	}{
		{"quit", 0, 0, nil, 0},
		{"raw Ctrl-C", 0, 2, nil, 130},
		{"cancelled raw Ctrl-C", 0, 2, context.Canceled, 130},
		{"SIGINT", 2, 0, context.Canceled, 130},
		{"SIGTERM", 15, 0, context.Canceled, 143},
		{"restore failure after raw Ctrl-C", 0, 2, status.Fail(status.CodeSupervisionFailed, "terminal restoration failed"), 125},
		{"restore failure after SIGTERM", 15, 0, status.Fail(status.CodeSupervisionFailed, "terminal restoration failed"), 125},
		{"unchecked failure", 0, 0, errors.New("private runner failure /fixture/private/terminal-runner"), 125},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTUIBoundaryFixture()
			fixture.runnerError = test.failure
			fixture.interruption = status.Interruption{Signal: test.localSignal, Name: "signal.example.invalid"}
			ctx, stop := context.WithCancelCause(t.Context())
			defer stop(nil)
			if test.signal != 0 {
				original := fixture.boundary.RunTUI
				fixture.boundary.RunTUI = func(ctx context.Context, request application.ObservationRequest, interval time.Duration, styling bool) (status.Interruption, error) {
					stop(status.Interruption{Signal: test.signal})
					return original(ctx, request, interval, styling)
				}
			}
			code, err := fixture.boundary.Run(ctx, []string{"tui"})
			if code != test.code || fixture.runs != 1 || !fixture.restored || fixture.collector.calls != 0 || strings.Contains(fixture.diagnostics.String(), "/fixture/private/terminal-runner") {
				t.Fatalf("terminal outcome code=%d want=%d error=%v diagnostics=%q", code, test.code, err, fixture.diagnostics.String())
			}
			if test.code == 125 {
				if err == nil || !strings.Contains(fixture.diagnostics.String(), "hippo.supervision.failed") {
					t.Fatalf("classified terminal failure lost: %v/%q", err, fixture.diagnostics.String())
				}
			} else if err != nil || fixture.diagnostics.Len() != 0 {
				t.Fatalf("quit/signal reported a failure: %v/%q", err, fixture.diagnostics.String())
			}
		})
	}
}

type tuiCleanupWriter struct {
	fixture       *tuiBoundaryFixture
	beforeRestore bool
}

func (writer *tuiCleanupWriter) Write(value []byte) (int, error) {
	if !writer.fixture.restored {
		writer.beforeRestore = true
	}
	return writer.fixture.diagnostics.Write(value)
}

func TestTUIJSONFailureBodyNamesCommandAfterRunnerCleanup(t *testing.T) {
	for _, args := range [][]string{{"--output", "json", "tui", "--color", "never"}, {"tui", "--output", "json", "--color", "always"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fixture := newTUIBoundaryFixture()
			fixture.runnerError = status.Fail(status.CodeSupervisionFailed, "terminal restoration failed")
			writer := &tuiCleanupWriter{fixture: fixture}
			fixture.boundary.Stderr = writer
			code, err := fixture.boundary.Run(t.Context(), args)
			lines := strings.Split(strings.TrimSpace(fixture.diagnostics.String()), "\n")
			var body struct {
				Command  string `json:"command"`
				ExitCode int    `json:"exitCode"`
			}
			if code != 125 || err == nil || writer.beforeRestore || !fixture.restored || fixture.output.Len() != 0 || len(lines) != 2 {
				t.Fatalf("failure body/cleanup: code=%d error=%v early=%v restored=%v output=%q diagnostics=%q", code, err, writer.beforeRestore, fixture.restored, fixture.output.String(), fixture.diagnostics.String())
			}
			if decodeError := json.Unmarshal([]byte(lines[1]), &body); decodeError != nil || body.Command != "hippo tui" || body.ExitCode != 125 {
				t.Fatalf("invalid TUI failure body: %+v error=%v", body, decodeError)
			}
		})
	}
}

func TestTUILateDefaultsRefreshWithoutOverridingCustomCallbacks(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{false: "defaults", true: "custom"}[custom], func(t *testing.T) {
			fixture := newTUIBoundaryFixture()
			boundary := fixture.boundary
			if !custom {
				boundary.TerminalCapabilities = nil
				boundary.RunTUI = nil
			}
			composed := 0
			latestEnvironment := ""
			compose := func(current cli.Application) cli.Application {
				composed++
				capturedEnvironment := strings.Join(current.Environment, ",")
				return current.WithTUIDefaults(func() (bool, bool, error) { fixture.capabilities++; return true, true, nil }, func(ctx context.Context, request application.ObservationRequest, interval time.Duration, styling bool) (status.Interruption, error) {
					latestEnvironment = capturedEnvironment
					return fixture.run(ctx, request, interval, styling)
				})
			}
			boundary = compose(boundary)
			boundary.Compose = compose
			boundary.Environment = []string{"TERM=xterm", "VERSION=latest"}
			code, err := boundary.Run(t.Context(), []string{"tui"})
			if code != 0 || err != nil || composed != 2 || fixture.capabilities != 1 || fixture.runs != 1 {
				t.Fatalf("late defaults/custom callbacks: code=%d error=%v composed=%d capabilities=%d runs=%d", code, err, composed, fixture.capabilities, fixture.runs)
			}
			if custom && latestEnvironment != "" || !custom && latestEnvironment != "TERM=xterm,VERSION=latest" {
				t.Fatalf("default stale capture or custom callback replaced: %q", latestEnvironment)
			}
		})
	}
}

func TestTUIReportsMissingCapabilitiesAsSupervisionFailure(t *testing.T) {
	for _, missing := range []string{"capabilities", "runner"} {
		t.Run(missing, func(t *testing.T) {
			fixture := newTUIBoundaryFixture()
			if missing == "capabilities" {
				fixture.boundary.TerminalCapabilities = nil
			} else {
				fixture.boundary.RunTUI = nil
			}
			code, err := fixture.boundary.Run(t.Context(), []string{"tui"})
			failure, classified := errors.AsType[status.Failure](err)
			if code != 125 || !classified || failure.Code != status.CodeSupervisionFailed || fixture.runs != 0 || fixture.collector.calls != 0 {
				t.Fatalf("missing %s: code=%d error=%v runs=%d host=%d", missing, code, err, fixture.runs, fixture.collector.calls)
			}
		})
	}
}

func TestRunDisplayLabelFlagPreservesExplicitEmptyAndRejectsBeforeProviders(t *testing.T) {
	for _, label := range []string{"", "../private"} {
		t.Run(label, func(t *testing.T) {
			fixture := newTUIBoundaryFixture()
			code, err := fixture.boundary.Run(t.Context(), []string{"run", "--label", label, "--", "echo"})
			failure, classified := errors.AsType[status.Failure](err)
			if code != 2 || !classified || failure.Code != status.CodeArgsInvalid || failure.Field != "--label" || fixture.collector.calls != 0 || fixture.capabilities != 0 || fixture.runs != 0 {
				t.Fatalf("explicit label was lost or reached providers: code=%d error=%v host=%d", code, err, fixture.collector.calls)
			}
		})
	}
	fixture := newTUIBoundaryFixture()
	code, err := fixture.boundary.Run(t.Context(), []string{"run", "--help"})
	if code != 0 || err != nil || !strings.Contains(fixture.output.String(), "--label") {
		t.Fatalf("run label is undiscoverable: code=%d error=%v help=%q", code, err, fixture.output.String())
	}
}
