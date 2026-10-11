package tui_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// Focused task-only fixture controls; the full original public suite remains
// the acceptance gate. Neither control changes an original scenario predicate.
func TestTUIBDDPublicFixtureFailureRecovery(t *testing.T) {
	if flag.Lookup("test.run").Value.String() != "^TestTUIBDDPublicFixtureFailureRecovery$" {
		return
	}
	if os.Getenv("HIPPO_BDD_SUITE") != contract.E2E || os.Getenv("HIPPO_BDD_ADAPTER") != contract.E2E {
		t.Fatal("public fixture controls require explicit E2E isolation")
	}
	t.Run("scenario-finalization-before-cancel-and-next-reset", requirePublicFixtureFailedFinalization)
	t.Run("original-inspection-classifier-with-native-hangup", requirePublicFixtureInspectionFailure)
}

// This control holds a real first reading, so the original readiness budget
// genuinely expires while the selected candidate is still owned and unready.
func TestTUIBDDPublicUnreadyFixtureRecovery(t *testing.T) {
	if flag.Lookup("test.run").Value.String() != "^(TestTUIBDDPublicUnreadyFixtureRecovery|TestTUIBDDPublicQuitSurfaces)$" {
		return
	}
	if os.Getenv("HIPPO_BDD_SUITE") != contract.E2E || os.Getenv("HIPPO_BDD_ADAPTER") != contract.E2E {
		t.Fatal("public unready control requires explicit E2E isolation")
	}
	driver := newComposedBDDDriver(contract.Adapter{Name: contract.E2E})
	driver.Reset()
	t.Cleanup(driver.Close)
	fixture := driver.public
	var transcript bytes.Buffer
	var afterErrors []error
	var bodyErrors []error
	var scenarioContexts []context.Context
	started, following := 0, 0
	result := godog.TestSuite{
		Name: "actual-unready-public-finalization",
		Options: &godog.Options{Strict: true, Format: "progress", Output: &transcript, DefaultContext: t.Context(), FeatureContents: []godog.Feature{{Name: "unready.fixture.feature", Contents: []byte(`Feature: Unready owned public cleanup
  Scenario: Original readiness failure retains independent cleanup
    Given an owned public terminal with a held real reading
    Then its original readiness budget expires
  Scenario: Next fixture remains genuinely usable
    Given an actual following public terminal
    Then that following terminal accepts help
`)}}},
		ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
			scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				driver.Reset()
				started++
				return ctx, nil
			})
			scenarioContext.After(func(ctx context.Context, scenario *godog.Scenario, bodyError error) (context.Context, error) {
				bodyErrors = append(bodyErrors, bodyError)
				scenarioContexts = append(scenarioContexts, ctx)
				afterContext, err := driver.finalizePublicScenario(ctx, scenario, bodyError)
				afterErrors = append(afterErrors, err)
				return afterContext, err
			})
			scenarioContext.Step(`^an owned public terminal with a held real reading$`, func(ctx context.Context) (context.Context, error) {
				return fixture.prepareInteractive(ctx, "known", 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20}, true)
			})
			scenarioContext.Step(`^its original readiness budget expires$`, func(ctx context.Context) error {
				began := time.Now()
				err := fixture.live(ctx)
				if !errors.Is(err, context.DeadlineExceeded) || fixture.transport.Result().TerminalReadyAvailable {
					return errors.New("held real reading did not produce the original unready deadline failure")
				}
				t.Logf("actual held-reading readiness elapsed=%s error=%v", time.Since(began), err)
				return err // Retain the actual original body failure in Godog.
			})
			scenarioContext.Step(`^an actual following public terminal$`, func(ctx context.Context) (context.Context, error) {
				return fixture.prepareInteractive(ctx, "known", 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false)
			})
			scenarioContext.Step(`^that following terminal accepts help$`, func(ctx context.Context) error {
				if err := fixture.key(ctx, "?", "HIPPO | Help"); err != nil {
					return err
				}
				following++
				return nil
			})
		},
	}.Run()
	t.Logf("actual unready body/After control transcript:\n%s", transcript.String())
	if result != 1 || started != 2 || following != 1 || len(afterErrors) != 2 || len(bodyErrors) != 2 || !errors.Is(bodyErrors[0], context.DeadlineExceeded) || bodyErrors[1] != nil || len(fixture.completed) != 2 {
		t.Fatalf("original unready failure/next scenario not proved: exit=%d started=%d following=%d bodyErrors=%v afterErrors=%v receipts=%d", result, started, following, bodyErrors, afterErrors, len(fixture.completed))
	}
	requirePublicUnreadyRetirements(t, fixture, scenarioContexts, afterErrors)
	if fixture.completed[0].Candidate.TerminalReadyAvailable || fixture.completed[0].Candidate.NaturalExit() || !fixture.completed[1].Candidate.NaturalExit() || fixture.completed[1].Candidate.ExitCode != 0 || fixture.completed[0].Candidate.Identity.PID == fixture.completed[1].Candidate.Identity.PID {
		t.Fatal("unready abort was relabeled natural q or following fixture reused the owned child")
	}
	driver.Reset()
}

func TestTUIBDDPublicQuitSurfaces(t *testing.T) {
	if flag.Lookup("test.run").Value.String() != "^(TestTUIBDDPublicUnreadyFixtureRecovery|TestTUIBDDPublicQuitSurfaces)$" {
		return
	}
	runPublicCanonicalSelectionAtLine(t, "@tui-terminal", "terminal.feature", 29, map[string]int{"Quit works from every TUI surface": 5}, 5, 25)
}

// Task-scoped regression executes unchanged affected canonical rows after full registry verification.
func TestTUIBDDPublicDemonstratedCauseRegression(t *testing.T) {
	if flag.Lookup("test.run").Value.String() != "^TestTUIBDDPublicDemonstratedCauseRegression$" {
		t.Skip("targeted public repair requires its explicit isolated selector")
	}
	runPublicCanonicalSelection(t, "~@e2e-exempt && @tui-refresh-02,~@e2e-exempt && @tui-refresh-04,~@e2e-exempt && @tui-refresh-09,~@e2e-exempt && @tui-views-02,~@e2e-exempt && @tui-views-04,~@e2e-exempt && @tui-keys-01,~@e2e-exempt && @tui-keys-03,~@e2e-exempt && @tui-keys-06", map[string]int{
		"Repeated refresh requests coalesce while observation is busy": 1,
		"Initial observation failure does not invent empty accounting": 1,
		"Exiting a busy TUI ignores late observations":                 1,
		"Too-small view recovers without losing live selection":        1,
		"Large lists scroll while retaining the selected identity":     1,
		"Dashboard keyboard actions reach their declared destination":  11,
		"Manual refresh is reachable from every view":                  5,
		"No-color output retains every state distinction":              1,
	}, 22, 66)
}

// Task-only regression: the full original125-row entry remains the gate.
func TestTUIBDDPublicResizeEpochRegression(t *testing.T) {
	if flag.Lookup("test.run").Value.String() != "^TestTUIBDDPublicResizeEpochRegression$" {
		t.Skip("resize/epoch regression requires its explicit isolated selector")
	}
	runPublicCanonicalSelection(t, "~@e2e-exempt && @tui-views-02,~@e2e-exempt && @tui-views-04", map[string]int{
		"Too-small view recovers without losing live selection":    1,
		"Large lists scroll while retaining the selected identity": 1,
	}, 2, 6)
}

func requirePublicFixtureFailedFinalization(t *testing.T) {
	t.Helper()
	driver := newComposedBDDDriver(contract.Adapter{Name: contract.E2E})
	driver.Reset()
	t.Cleanup(driver.Close)
	fixture := driver.public
	var transcript bytes.Buffer
	var contexts []context.Context
	var finalizationErrors []error
	started, passed := 0, 0
	bodyError := errors.New("deliberate fixture control assertion failure")
	result := godog.TestSuite{
		Name: "owned-public-failed-scenario-cleanup-control",
		Options: &godog.Options{Strict: true, Format: "progress", Output: &transcript, DefaultContext: t.Context(), FeatureContents: []godog.Feature{{Name: "fixture-cleanup.feature", Contents: []byte(`Feature: Owned public fixture cleanup
  Scenario: Failed body still owns live cleanup
    Given an actual owned public terminal
    Then the fixture control assertion fails
  Scenario: Following body starts after real retirement
    Given an actual owned public terminal
    Then the following terminal remains usable
`)}}},
		ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
			scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				driver.Reset()
				started++
				return ctx, nil
			})
			scenarioContext.After(func(ctx context.Context, scenario *godog.Scenario, stepError error) (context.Context, error) {
				afterContext, err := driver.finalizePublicScenario(ctx, scenario, stepError)
				finalizationErrors = append(finalizationErrors, err)
				return afterContext, err
			})
			scenarioContext.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
				if ctx.Err() != nil {
					return ctx, errors.New("Godog canceled the scenario before public finalization")
				}
				contexts = append(contexts, ctx)
				return ctx, nil
			})
			scenarioContext.Step(`^an actual owned public terminal$`, func(ctx context.Context) (context.Context, error) {
				return fixture.prepareInteractive(ctx, "known", 120, 32, "30s", support.PublicTUIHostReply{CPUPercent: 20}, false)
			})
			scenarioContext.Step(`^the fixture control assertion fails$`, func() error { return bodyError })
			scenarioContext.Step(`^the following terminal remains usable$`, func(ctx context.Context) error {
				if err := fixture.key(ctx, "?", "HIPPO | Help"); err != nil {
					return err
				}
				passed++
				return nil
			})
		},
	}.Run()
	t.Logf("actual failed-scenario control transcript:\n%s", transcript.String())
	if result != 1 || !strings.Contains(transcript.String(), bodyError.Error()) || started != 2 || passed != 1 || len(contexts) != 2 || len(fixture.completed) != 2 {
		t.Fatalf("original failed body was lost or poisoned the next real fixture: exit=%d started=%d passed=%d contexts=%d receipts=%d", result, started, passed, len(contexts), len(fixture.completed))
	}
	if len(finalizationErrors) != 2 {
		t.Fatalf("shared finalizer ran %d times, want both cases", len(finalizationErrors))
	}
	for index, err := range finalizationErrors {
		if err != nil {
			t.Fatalf("shared finalizer failed for case %d independently of deliberate body error: %v", index, err)
		}
	}
	for index, receipt := range fixture.completed {
		if !errors.Is(contexts[index].Err(), context.Canceled) || !receipt.Candidate.Joined() || !receipt.Candidate.NaturalExit() || receipt.Candidate.ExitCode != 0 || receipt.Candidate.HolderExitCode != 0 || !receipt.Collector.Closed || receipt.Collector.Active != 0 {
			t.Fatalf("actual failed-body fixture did not naturally q/join/close before Godog cancellation: %+v", receipt)
		}
		t.Logf("actual finalized fixture attempt=%d candidate=%+v collector=%+v", index, receipt.Candidate, receipt.Collector)
	}
	if fixture.completed[0].Candidate.Identity.PID == fixture.completed[1].Candidate.Identity.PID {
		t.Fatal("following scenario reused the prior closed public candidate")
	}
	driver.Reset()
}

func requirePublicFixtureInspectionFailure(t *testing.T) {
	t.Helper()
	driver := support.NewDriver(contract.Adapter{Name: contract.E2E})
	driver.Reset()
	t.Cleanup(driver.Close)
	handlers := make(map[string]any)
	for _, binding := range driver.Bindings() {
		handlers[binding.Pattern] = binding.Handler
	}
	if err := requirePublicFailureHandler(t, handlers, `^TUI terminal capability inspection fails$`)(); err != nil {
		t.Fatal(err)
	}
	if err := requirePublicFailureContextHandler(t, handlers, `^TUI is requested$`)(t.Context()); err != nil {
		t.Fatalf("actual inspection fixture refused: %v; evidence=%s", err, driver.PublicTUIInspectionEvidence())
	}
	t.Logf("actual parent native inspection evidence=%s", driver.PublicTUIInspectionEvidence())
	if err := requirePublicFailureHandler(t, handlers, `^it exits 125 with a privacy-safe diagnostic before terminal entry or observation$`)(); err != nil {
		t.Fatal(err)
	}
}

func requirePublicFailureHandler(t *testing.T, handlers map[string]any, pattern string) func() error {
	t.Helper()
	handler, valid := handlers[pattern].(func() error)
	if !valid {
		t.Fatalf("original failure binding %q has an unexpected handler type", pattern)
	}
	return handler
}

func requirePublicFailureContextHandler(t *testing.T, handlers map[string]any, pattern string) func(context.Context) error {
	t.Helper()
	handler, valid := handlers[pattern].(func(context.Context) error)
	if !valid {
		t.Fatalf("original failure binding %q has an unexpected handler type", pattern)
	}
	return handler
}

func requirePublicUnreadyRetirements(t *testing.T, fixture *publicBDDObservation, scenarioContexts []context.Context, afterErrors []error) {
	t.Helper()
	if len(scenarioContexts) != len(afterErrors) {
		t.Fatalf("scenario context/finalizer count differs: contexts=%d finalizers=%d", len(scenarioContexts), len(afterErrors))
	}
	for index, err := range afterErrors {
		if err != nil {
			t.Fatalf("actual finalizer %d failed independently of body: %v", index, err)
		}
		scenarioContext := scenarioContexts[index]
		if scenarioContext == nil || !errors.Is(scenarioContext.Err(), context.Canceled) {
			t.Fatal("scenario cancellation was not retained after After")
		}
		receipt := fixture.completed[index]
		candidate := receipt.Candidate
		if !candidate.Joined() || candidate.HolderExitCode != 0 || !candidate.PostChildAcknowledged || !candidate.TerminalAfterAvailable || candidate.TerminalBefore.Mode != candidate.TerminalAfter.Mode || candidate.TerminalBefore.Foreground != candidate.TerminalAfter.Foreground || !receipt.Collector.Closed || receipt.Collector.Active != 0 {
			t.Fatalf("actual unready/following Wait/EOF/restoration/probe retirement incomplete: %+v", receipt)
		}
		t.Logf("actual unready/following retirement index=%d candidate=%+v collector=%+v", index, candidate, receipt.Collector)
	}
}
