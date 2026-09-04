package contract

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	messages "github.com/cucumber/messages/go/v34"
)

const (
	// FeaturePath points every adapter at the canonical recursive Gherkin corpus.
	FeaturePath = "../../specs/behaviours"
	// Unit identifies the deterministic in-process adapter.
	Unit = "unit"
	// Integration identifies the local filesystem and process adapter.
	Integration = "integration"
	// E2E identifies the compiled public-binary adapter.
	E2E = "e2e"
	// syntheticHostPressureReason documents scenarios that require deterministic host samples.
	syntheticHostPressureReason = "requires synthetic samples that cannot be injected through the compiled binary"
	hostEvidenceBoundary        = "host evidence"
	leaseOwnershipBoundary      = "lease ownership"
	processControlBoundary      = "process control"
	evidenceFilesystemBoundary  = "evidence filesystem"
)

// StepBinding keeps one canonical Godog expression adjacent to its handler.
type StepBinding struct {
	Pattern string
	Handler any
}

// Adapter describes one behavior layer and its exemption tag.
type Adapter struct {
	Name         string
	ExemptionTag string
}

// Exemption documents the boundary and reason that prevent one scenario from
// running through an adapter.
type Exemption struct {
	Scenario string
	Boundary string
	Reason   string
}

// liveLeaseExemption explains scenarios that cannot mutate real lease ownership.
const liveLeaseExemption = "would replace or contend with live process ownership outside the isolated binary fixture"

// ApprovedExemptions is the reviewed, exact adapter exemption inventory.
var ApprovedExemptions = map[string][]Exemption{
	Unit:        {},
	Integration: {},
	E2E: {
		{Scenario: "Healthy consecutive samples admit work", Boundary: hostEvidenceBoundary, Reason: syntheticHostPressureReason},
		{Scenario: "Stable macOS warning admits degraded work", Boundary: hostEvidenceBoundary, Reason: syntheticHostPressureReason},
		{Scenario: "Growing pressure defers degraded work", Boundary: hostEvidenceBoundary, Reason: "requires synthetic compressor counters that cannot be injected through the compiled binary"},
		{Scenario: "Strict work never uses degraded admission", Boundary: hostEvidenceBoundary, Reason: syntheticHostPressureReason},
		{Scenario: "Balanced work falls back on a small runner", Boundary: hostEvidenceBoundary, Reason: "requires synthetic host capacity that cannot be injected through the compiled binary"},
		{Scenario: "Minimal work still runs on a tiny machine", Boundary: hostEvidenceBoundary, Reason: "requires synthetic host capacity that cannot be injected through the compiled binary"},
		{Scenario: "Exhausted storage requires cleanup", Boundary: hostEvidenceBoundary, Reason: "requires synthetic disk evidence that cannot be injected through the compiled binary"},
		{Scenario: "Swap-out growth is normalized to the policy window", Boundary: hostEvidenceBoundary, Reason: "requires synthetic swap counters that cannot be injected through the compiled binary"},
		{Scenario: "Compressor growth requires both payload and growth", Boundary: hostEvidenceBoundary, Reason: "requires synthetic compressor counters that cannot be injected through the compiled binary"},
		{Scenario: "A strict transaction does not silently downgrade", Boundary: "task capacity", Reason: "requires a synthetic capacity mismatch that cannot be injected through the compiled binary"},
		{Scenario: "Linux cgroup memory limits host capacity", Boundary: hostEvidenceBoundary, Reason: "requires synthetic proc and cgroup files instead of the public host filesystem"},
		{Scenario: "Linux without swap remains usable", Boundary: hostEvidenceBoundary, Reason: "requires synthetic swap capabilities instead of the public host state"},
		{Scenario: "Linux PSI detects active memory contention", Boundary: hostEvidenceBoundary, Reason: "requires synthetic PSI evidence instead of the public host state"},
		{Scenario: "A live heavy lease defers a second owner", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "A long-lived service never holds the heavy-work lease", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "Concurrent services keep their own inheritable sessions", Boundary: leaseOwnershipBoundary, Reason: liveLeaseExemption},
		{Scenario: "An inherited session runs without reacquiring the lease", Boundary: "session inheritance", Reason: "requires synthetic inherited session state unavailable to an isolated binary fixture"},
		{Scenario: "A failed child keeps its own exit code", Boundary: processControlBoundary, Reason: "requires deterministic admission and child process control unavailable to the public-host fixture"},
		{Scenario: "Canonical concurrency remains ecosystem neutral", Boundary: processControlBoundary, Reason: "requires deterministic admission and inspection of an isolated child environment"},
		{Scenario: "Consumers select their concurrency environment mappings", Boundary: processControlBoundary, Reason: "requires deterministic admission and inspection of an isolated child environment"},
		{Scenario: "Invalid concurrency mappings fail before child execution", Boundary: processControlBoundary, Reason: "requires deterministic admission setup and inspection proving that an isolated child never starts"},
		{Scenario: "Guarded child streams remain composable", Boundary: processControlBoundary, Reason: "requires deterministic admission and isolated standard streams unavailable to the public-host fixture"},
		{Scenario: "An interrupted guard signals once and then force-stops the child", Boundary: processControlBoundary, Reason: "requires deterministic interrupt timing and process signaling unavailable to the public-host fixture"},
		{Scenario: "A supervision failure reaps the guarded child before releasing ownership", Boundary: processControlBoundary, Reason: "requires an injected collector failure while controlling and inspecting the child process lifecycle"},
		{Scenario: "Critical pressure sheds eligible work", Boundary: processControlBoundary, Reason: "requires synthetic critical pressure while controlling the child process lifecycle"},
		{Scenario: "Worsening warning sheds degraded work", Boundary: processControlBoundary, Reason: "requires synthetic warning growth while controlling the child process lifecycle"},
		{Scenario: "Active evidence rotates without truncating its lifetime summary", Boundary: evidenceFilesystemBoundary, Reason: "requires test-sized rotation limits and direct inspection of private runtime files"},
		{Scenario: "A shared root admits at most twenty live evidence streams", Boundary: evidenceFilesystemBoundary, Reason: "requires concurrent private writer ownership and direct inspection of the shared runtime root"},
		{Scenario: "Inactive evidence is pruned to the shared storage budget", Boundary: evidenceFilesystemBoundary, Reason: "requires synthetic sparse evidence files and direct inspection of private retention state"},
		{Scenario: "Release admission preserves the requested capacity envelope", Boundary: hostEvidenceBoundary, Reason: "requires synthetic release capacity that cannot be injected through the compiled binary"},
		{Scenario: "Development monitor emits machine-readable transitions", Boundary: hostEvidenceBoundary, Reason: "requires deterministic repeated and changing host states that cannot be injected through the compiled binary"},
		{Scenario: "Release raw evidence streams to standard output", Boundary: hostEvidenceBoundary, Reason: "requires injected health probes and deterministic cancellation unavailable to the compiled binary fixture"},
		{Scenario: "Release summary streams to standard output", Boundary: hostEvidenceBoundary, Reason: "requires injected health probes and deterministic cancellation unavailable to the compiled binary fixture"},
		{Scenario: "Release streaming propagates downstream failure", Boundary: "output stream", Reason: "requires an injected failing writer unavailable through the compiled binary process boundary"},
		{Scenario: "Release builds stay outside repository history", Boundary: "repository state", Reason: "Git ignore policy is outside the compiled binary boundary"},
		{Scenario: "End-to-end binaries are temporary", Boundary: "test harness", Reason: "binary cleanup is owned by the harness outside the compiled binary boundary"},
		{Scenario: "Bootstrap cache retention is bounded", Boundary: "bootstrap wrapper", Reason: "cache retention is owned by the wrapper outside the compiled binary boundary"},
		{Scenario: "Lint gate wiring is exhaustive and module scoped", Boundary: "repository configuration", Reason: "lint configuration is outside the compiled binary boundary"},
		{Scenario: "Behavior adapter wiring is complete", Boundary: "test harness", Reason: "adapter registration is outside the compiled binary boundary"},
		{Scenario: "Contributor gate wiring is complete", Boundary: "repository configuration", Reason: "hooks and CI configuration are outside the compiled binary boundary"},
		{Scenario: "Machine-local configuration and binaries stay private", Boundary: "repository state", Reason: "Git index and ignore policy are outside the compiled binary boundary"},
	},
}

// Scenario is the compliance view of one compiled Gherkin scenario.
type Scenario struct {
	Name  string
	Steps []string
	Tags  []string
}

// Driver is the shared contract implemented by every behavior adapter.
type Driver interface {
	Bindings() []StepBinding
	Reset()
	Close()
}

// Adapters returns the complete behavior adapter inventory in execution order.
func Adapters() []Adapter {
	return []Adapter{
		{Name: Unit},
		{Name: Integration, ExemptionTag: "@integration-exempt"},
		{Name: E2E, ExemptionTag: "@e2e-exempt"},
	}
}

// AdapterByName returns one configured adapter.
func AdapterByName(name string) (Adapter, error) {
	for _, adapter := range Adapters() {
		if adapter.Name == name {
			return adapter, nil
		}
	}

	return Adapter{}, fmt.Errorf("unknown behavior adapter %q", name)
}

// SuiteOptions returns strict Godog options shared by compliance and execution.
func SuiteOptions(adapter Adapter) *godog.Options {
	tags := ""
	if adapter.ExemptionTag != "" {
		tags = "~" + adapter.ExemptionTag
	}

	return &godog.Options{
		Format: "progress",
		Paths:  []string{FeaturePath},
		Tags:   tags,
		Strict: true,
	}
}

// Run executes the canonical corpus through one strict adapter.
func Run(t *testing.T, adapter Adapter, driver Driver) {
	t.Helper()
	defer driver.Close()

	bindings := driver.Bindings()
	if err := Verify(adapter, bindings); err != nil {
		t.Fatal(err)
	}

	options := SuiteOptions(adapter)
	options.TestingT = t
	initialize := func(scenarioContext *godog.ScenarioContext) {
		scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
			driver.Reset()

			return ctx, nil
		})

		for _, binding := range bindings {
			scenarioContext.Step(binding.Pattern, binding.Handler)
		}
	}

	status := godog.TestSuite{
		Name:                "hippo-" + adapter.Name,
		ScenarioInitializer: initialize,
		Options:             options,
	}.Run()

	if status != 0 {
		t.Fatalf("%s behavior suite exited %d", adapter.Name, status)
	}
}

// ValidateScenarioStructure requires an explicit action and outcome.
func ValidateScenarioStructure(name string, keywordTypes []string) error {
	hasAction, hasOutcome := false, false

	for _, keywordType := range keywordTypes {
		hasAction = hasAction || keywordType == "Action"
		hasOutcome = hasOutcome || keywordType == "Outcome"
	}
	if !hasAction || !hasOutcome {
		return fmt.Errorf("scenario %q requires explicit When and Then steps", name)
	}

	return nil
}

// ValidateHandlers rejects missing and non-function step handlers.
func ValidateHandlers(bindings []StepBinding) []error {
	errorsFound := []error{}

	for _, binding := range bindings {
		handler := reflect.ValueOf(binding.Handler)
		if !handler.IsValid() || handler.Kind() != reflect.Func || handler.IsNil() {
			errorsFound = append(errorsFound, fmt.Errorf("binding %q requires a nonnil function handler", binding.Pattern))
		}
	}

	return errorsFound
}

// ValidateBindings rejects invalid expressions, invalid handlers, undefined or ambiguous steps, and unused bindings.
func ValidateBindings(bindings []StepBinding, steps []string) []error {
	compiled := make([]*regexp.Regexp, len(bindings))
	errorsFound := ValidateHandlers(bindings)

	for index, binding := range bindings {
		expression, err := regexp.Compile(binding.Pattern)
		if err != nil {
			errorsFound = append(errorsFound, fmt.Errorf("invalid binding %q: %w", binding.Pattern, err))
			continue
		}
		compiled[index] = expression
	}

	used := make([]bool, len(bindings))

	for _, step := range steps {
		matches := []int{}

		for index, expression := range compiled {
			if expression != nil && expression.MatchString(step) {
				matches = append(matches, index)
			}
		}

		switch len(matches) {
		case 0:
			errorsFound = append(errorsFound, fmt.Errorf("undefined behavior step %q", step))
		case 1:
			used[matches[0]] = true
		default:
			errorsFound = append(errorsFound, fmt.Errorf("ambiguous behavior step %q", step))
		}
	}

	for index, wasUsed := range used {
		if !wasUsed && compiled[index] != nil {
			errorsFound = append(errorsFound, fmt.Errorf("unused behavior binding %q", bindings[index].Pattern))
		}
	}

	return errorsFound
}

// ValidateExemptions requires exact reviewed tags with a concrete boundary and
// reason. Adapters without an exemption tag must execute the complete corpus.
func ValidateExemptions(adapter Adapter, scenarios []Scenario) []error {
	errorsFound := []error{}
	approved := map[string]Exemption{}
	exemptions := ApprovedExemptions[adapter.Name]

	if adapter.ExemptionTag == "" && len(exemptions) > 0 {
		errorsFound = append(errorsFound, fmt.Errorf("%s adapter does not permit exemptions", adapter.Name))
	}

	for _, exemption := range exemptions {
		if _, duplicate := approved[exemption.Scenario]; duplicate {
			errorsFound = append(errorsFound, fmt.Errorf("%s exemption %q is duplicated", adapter.Name, exemption.Scenario))
		}
		if strings.TrimSpace(exemption.Boundary) == "" {
			errorsFound = append(errorsFound, fmt.Errorf("%s exemption %q has no boundary", adapter.Name, exemption.Scenario))
		}
		if strings.TrimSpace(exemption.Reason) == "" {
			errorsFound = append(errorsFound, fmt.Errorf("%s exemption %q has no reason", adapter.Name, exemption.Scenario))
		}

		approved[exemption.Scenario] = exemption
	}

	seen := map[string]bool{}
	knownTags := map[string]bool{}

	for _, configuredAdapter := range Adapters() {
		if configuredAdapter.ExemptionTag != "" {
			knownTags[configuredAdapter.ExemptionTag] = true
		}
	}

	for _, scenario := range scenarios {
		tagged := false

		for _, tag := range scenario.Tags {
			if strings.HasSuffix(tag, "-exempt") && !knownTags[tag] {
				errorsFound = append(errorsFound, fmt.Errorf("scenario %q uses unknown exemption tag %q", scenario.Name, tag))
			}
			tagged = tagged || tag == adapter.ExemptionTag
		}

		_, expected := approved[scenario.Name]
		if tagged != expected {
			errorsFound = append(errorsFound, fmt.Errorf("%s exemption mismatch for scenario %q", adapter.Name, scenario.Name))
		}
		if tagged {
			seen[scenario.Name] = true
		}
	}

	for scenario := range approved {
		if !seen[scenario] {
			errorsFound = append(errorsFound, fmt.Errorf("approved %s exemption %q is absent", adapter.Name, scenario))
		}
	}

	return errorsFound
}

type corpusInspection struct {
	scenarios       []Scenario
	steps           []string
	structuralError []error
	astScenarios    int
}

func countDiskFeatures() (int, error) {
	count := 0
	err := filepath.WalkDir(FeaturePath, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if !entry.IsDir() && filepath.Ext(path) == ".feature" {
			count++
		}
		return nil
	})

	return count, err
}

func inspectScenario(scenario *messages.Scenario) error {
	keywords := make([]string, 0, len(scenario.Steps))
	for _, step := range scenario.Steps {
		keywords = append(keywords, step.KeywordType.String())
	}

	return ValidateScenarioStructure(scenario.Name, keywords)
}

func inspectFeature(uri string, feature *messages.Feature, pickles []*messages.Pickle) corpusInspection {
	inspection := corpusInspection{}
	if feature == nil {
		inspection.structuralError = append(inspection.structuralError, fmt.Errorf("%s has no Feature declaration", uri))
		return inspection
	}

	for _, child := range feature.Children {
		if scenario := child.Scenario; scenario != nil {
			inspection.astScenarios++
			if err := inspectScenario(scenario); err != nil {
				inspection.structuralError = append(inspection.structuralError, err)
			}
		}
		if rule := child.Rule; rule != nil {
			inspection.inspectRule(rule)
		}
	}

	inspection.inspectPickles(pickles)

	return inspection
}

func (inspection *corpusInspection) inspectRule(rule *messages.Rule) {
	for _, child := range rule.Children {
		if scenario := child.Scenario; scenario != nil {
			inspection.astScenarios++
			if err := inspectScenario(scenario); err != nil {
				inspection.structuralError = append(inspection.structuralError, err)
			}
		}
	}
}

func (inspection *corpusInspection) inspectPickles(pickles []*messages.Pickle) {
	for _, pickle := range pickles {
		scenario := Scenario{Name: pickle.Name}

		for _, step := range pickle.Steps {
			scenario.Steps = append(scenario.Steps, step.Text)
			inspection.steps = append(inspection.steps, step.Text)
		}
		for _, tag := range pickle.Tags {
			scenario.Tags = append(scenario.Tags, tag.Name)
		}

		inspection.scenarios = append(inspection.scenarios, scenario)
	}
}

func mergeInspection(target *corpusInspection, source corpusInspection) {
	target.scenarios = append(target.scenarios, source.scenarios...)
	target.steps = append(target.steps, source.steps...)
	target.structuralError = append(target.structuralError, source.structuralError...)
	target.astScenarios += source.astScenarios
}

func runnableScenarios(adapter Adapter, scenarios []Scenario) int {
	count := 0

	for _, scenario := range scenarios {
		exempt := false

		for _, tag := range scenario.Tags {
			exempt = exempt || tag == adapter.ExemptionTag
		}

		if !exempt {
			count++
		}
	}

	return count
}

func combineErrors(errorsFound []error) error {
	if len(errorsFound) == 0 {
		return nil
	}

	messagesFound := make([]string, 0, len(errorsFound))
	for _, found := range errorsFound {
		messagesFound = append(messagesFound, found.Error())
	}

	sort.Strings(messagesFound)

	return errors.New(strings.Join(messagesFound, "\n"))
}

// Verify validates the recursive corpus, bindings, structure, and one adapter policy.
func Verify(adapter Adapter, bindings []StepBinding) error {
	corpusOptions := &godog.Options{Paths: []string{FeaturePath}, Strict: true}
	features, err := (godog.TestSuite{Options: corpusOptions}).RetrieveFeatures()
	if err != nil {
		return fmt.Errorf("retrieve behavior corpus: %w", err)
	}

	diskFeatures, err := countDiskFeatures()
	if err != nil {
		return fmt.Errorf("walk behavior corpus: %w", err)
	}
	if diskFeatures == 0 || len(features) != diskFeatures {
		return fmt.Errorf("parsed %d of %d canonical behavior features", len(features), diskFeatures)
	}

	inspection := corpusInspection{}

	for _, feature := range features {
		mergeInspection(&inspection, inspectFeature(feature.Uri, feature.Feature, feature.Pickles))
	}

	if inspection.astScenarios == 0 || len(inspection.scenarios) == 0 {
		inspection.structuralError = append(inspection.structuralError, errors.New("behavior corpus has no scenarios"))
	}
	inspection.structuralError = append(inspection.structuralError, ValidateBindings(bindings, inspection.steps)...)
	inspection.structuralError = append(inspection.structuralError, ValidateExemptions(adapter, inspection.scenarios)...)
	if runnableScenarios(adapter, inspection.scenarios) == 0 {
		inspection.structuralError = append(inspection.structuralError, fmt.Errorf("%s adapter runs no scenarios", adapter.Name))
	}

	return combineErrors(inspection.structuralError)
}
