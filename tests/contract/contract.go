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
	syntheticHostPressureReason = "requires synthetic host pressure samples"
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

// Exemption documents why one scenario cannot run through an adapter.
type Exemption struct {
	Scenario string
	Reason   string
}

// liveLeaseExemption explains scenarios that cannot mutate real lease ownership.
const liveLeaseExemption = "would mutate live lease ownership"

// ApprovedExemptions is the reviewed, exact adapter exemption inventory.
var ApprovedExemptions = map[string][]Exemption{
	Unit: {
		{"Release builds stay outside repository history", "requires repository Git ignore configuration"},
		{"End-to-end binaries are temporary", "requires subprocess and filesystem lifecycle access"},
		{"Bootstrap cache retention is bounded", "requires the real bootstrap and filesystem cache"},
		{"Lint gate wiring is exhaustive and module scoped", "requires repository lint configuration"},
		{"Behavior adapter wiring is complete", "requires repository test configuration"},
		{"Contributor gate wiring is complete", "requires repository hook and CI configuration"},
		{"Machine-local configuration and binaries stay private", "requires the repository Git index and ignore rules"},
	},
	Integration: {},
	E2E: {
		{"Healthy consecutive samples admit work", syntheticHostPressureReason},
		{"Stable macOS warning admits degraded work", syntheticHostPressureReason},
		{"Growing pressure defers degraded work", "requires synthetic compressor counters"},
		{"Strict work never uses degraded admission", syntheticHostPressureReason},
		{"Balanced work falls back on a small runner", "requires synthetic host capacity"},
		{"Minimal work still runs on a tiny machine", "requires synthetic host capacity"},
		{"Exhausted storage requires cleanup", "requires synthetic disk evidence"},
		{"Swap-out growth is normalized to the policy window", "requires synthetic swap counters"},
		{"Compressor growth requires both payload and growth", "requires synthetic compressor counters"},
		{"A strict transaction does not silently downgrade", "requires synthetic task capacity"},
		{"Linux cgroup memory limits host capacity", "requires synthetic proc and cgroup files"},
		{"Linux without swap remains usable", "requires synthetic swap capabilities"},
		{"Linux PSI detects active memory contention", "requires synthetic PSI evidence"},
		{"A live heavy lease defers a second owner", liveLeaseExemption},
		{"A long-lived service never holds the heavy-work lease", liveLeaseExemption},
		{"Concurrent services keep their own inheritable sessions", liveLeaseExemption},
		{"An inherited session runs without reacquiring the lease", "requires synthetic inherited session state"},
		{"A failed child keeps its own exit code", "requires deterministic admission and child process control"},
		{"An interrupted guard signals once and then force-stops the child", "requires synthetic interrupt delivery and process signaling"},
		{"Critical pressure sheds eligible work", "requires synthetic critical pressure and process signaling"},
		{"Worsening warning sheds degraded work", "requires synthetic warning pressure and process signaling"},
		{"Release admission preserves the requested capacity envelope", "requires synthetic release host samples"},
		{"Release overlap rejects failed health evidence", "requires synthetic failed health evidence"},
		{"Release overlap rejects an unresponsive routed journey", "requires synthetic routed-latency evidence"},
		{"Release monitoring requires generic health inputs", "requires controlled release-monitor environment"},
		{"Release builds stay outside repository history", "requires repository Git ignore configuration"},
		{"End-to-end binaries are temporary", "inspects the test harness rather than the public binary"},
		{"Bootstrap cache retention is bounded", "mutates a synthetic bootstrap cache"},
		{"Lint gate wiring is exhaustive and module scoped", "requires repository lint configuration"},
		{"Behavior adapter wiring is complete", "requires repository test configuration"},
		{"Contributor gate wiring is complete", "requires repository hook and CI configuration"},
		{"Invalid explicit configuration is actionable", "requires an intentionally invalid local configuration"},
		{"Machine-local configuration and binaries stay private", "inspects repository Git policy rather than the public binary"},
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
		{Name: Unit, ExemptionTag: "@unit-exempt"},
		{Name: Integration},
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
		Name:                "resource-guard-" + adapter.Name,
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

// ValidateExemptions requires exact reviewed tags and nonempty reasons.
func ValidateExemptions(adapter Adapter, scenarios []Scenario) []error {
	errorsFound := []error{}
	approved := map[string]string{}

	for _, exemption := range ApprovedExemptions[adapter.Name] {
		if strings.TrimSpace(exemption.Reason) == "" {
			errorsFound = append(errorsFound, fmt.Errorf("%s exemption %q has no reason", adapter.Name, exemption.Scenario))
		}
		approved[exemption.Scenario] = exemption.Reason
	}

	seen := map[string]bool{}
	knownTags := map[string]bool{"@unit-exempt": true, "@e2e-exempt": true}

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
