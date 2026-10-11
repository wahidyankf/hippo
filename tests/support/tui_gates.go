package support

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wahidyankf/hippo/tests/contract"
)

type tuiGateState struct{ quick, full, compiled, policy, terminalTests, monitorGuide string }

func (driver *Driver) tuiGateBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^TUI verification wiring is inspected$`, driver.inspectTUIGates),
		step(`^deterministic TUI model and view tests run in the quick gate$`, driver.requireTUIQuickModelGate),
		step(`^isolated terminal lifecycle tests run in the integration gate$`, driver.requireTUIIntegrationGate),
		step(`^standalone TUI command journeys run in the full gate$`, driver.requireTUIStandaloneGate),
		step(`^real tmux and WezTerm verification remains a separately recorded host proof$`, driver.requireTUIHostGateSeparation),
		step(`^no unit scenario is exempted for a terminal or display boundary$`, driver.requireNoUnitExemptions),
	}
}

func (driver *Driver) inspectTUIGates() error {
	if driver.mode == contract.E2E {
		return errors.New("e2e TUI gate inspection requires repository files outside the public binary boundary")
	}
	root, err := toolRoot()
	if err != nil {
		return err
	}
	state := &tuiGateState{}
	for _, file := range []struct {
		path   string
		target *string
	}{
		{"scripts/test-quick.sh", &state.quick},
		{"scripts/test.sh", &state.full},
		{"tests/e2e/run.sh", &state.compiled},
		{"repo-governance/development/quality-gates.md", &state.policy},
		{"tests/integration/tui_pty_test.go", &state.terminalTests},
		{"docs/how-to/monitor-work-with-the-tui.md", &state.monitorGuide},
	} {
		data, err := os.ReadFile(filepath.Join(root, file.path))
		if err != nil {
			return err
		}
		*file.target = strings.ReplaceAll(string(data), "\\\n", "")
	}
	for _, script := range []*string{&state.quick, &state.full, &state.compiled} {
		var commands []string
		for line := range strings.SplitSeq(*script, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				commands = append(commands, line)
			}
		}
		*script = strings.Join(commands, "\n")
	}
	unitAdapter, err := contract.AdapterByName(contract.Unit)
	if err != nil {
		return err
	}
	driver.unitExemptionsForbidden = unitAdapter.ExemptionTag == "" && len(contract.ApprovedExemptions[contract.Unit]) == 0
	driver.tuiGates = state
	return nil
}

func (driver *Driver) requireTUIQuickModelGate() error {
	state := driver.tuiGates
	if state == nil {
		return errors.New("TUI gate wiring was not inspected")
	}
	calls, valid := gateTestCommands(state.quick)
	if !valid {
		return errors.New("quick gate contains an invalid test command")
	}
	ordinary := false
	for _, call := range calls {
		if call.suite == contract.Integration || call.suite == contract.E2E {
			return errors.New("quick gate invokes a native executing corpus")
		}
		if slices.Contains(call.args, "./cmd/...") && slices.Contains(call.args, "./internal/...") && gateOrdinarySelection(call.args, gateOrdinaryEntries()) {
			ordinary = true
		}
	}
	if !ordinary {
		return errors.New("quick gate omits unrestricted deterministic tests with exact corpus exclusions")
	}
	return nil
}

func (driver *Driver) requireTUIIntegrationGate() error {
	state := driver.tuiGates
	if state == nil {
		return errors.New("TUI gate wiring was not inspected")
	}
	calls, valid := gateTestCommands(state.full)
	if !valid {
		return errors.New("full gate contains an invalid test command")
	}
	ordinary, composed := false, 0
	for _, call := range calls {
		if slices.Contains(call.args, "./tests/integration") && !slices.Contains(call.args, "-race") && len(gateFlagValues(call.args, "-run")) == 0 {
			ordinary = true
		}
		if gateExecutingEntry(call, contract.Integration, "TestIntegrationBehaviours", false) {
			composed++
		}
	}
	if !ordinary || !strings.Contains(state.terminalTests, "func Test") {
		return errors.New("full gate omits actual isolated terminal lifecycle fixtures")
	}
	if composed != 1 {
		return errors.New("integration corpus requires one explicit mode and exclusive composed entry")
	}
	return nil
}

func (driver *Driver) requireTUIStandaloneGate() error {
	state := driver.tuiGates
	if state == nil {
		return errors.New("TUI gate wiring was not inspected")
	}
	if !gateHasE2ECommand(state.full) || gateHasE2ECommand(state.quick) || !gateHasOldReleasePreparation(state.compiled) {
		return errors.New("compiled TUI journeys are not confined to the full gate")
	}
	calls, valid := gateTestCommands(state.compiled)
	if !valid {
		return errors.New("compiled gate contains an invalid test command")
	}
	entries := gateRequiredPublicEntries()
	seen := make(map[string]int, len(entries))
	for _, call := range calls {
		if !slices.Contains(call.args, "./internal/adapters/tui") {
			continue
		}
		entry := ""
		for _, required := range entries {
			if gateExecutingEntry(call, contract.E2E, required, false) {
				entry = required
			}
		}
		if entry == "" || call.binary != "$temporary_dir/hippo" ||
			len(gateFlagValues(call.args, "-count")) != 1 || gateFlag(call.args, "-count") != "1" ||
			(call.adapter != contract.E2E && (entry != e2eBehaviourEntry || call.adapter != "")) {
			return errors.New("compiled candidate contains a nonexecuting or unowned public entry")
		}
		for _, argument := range call.args {
			if strings.HasPrefix(argument, "./") && argument != "./internal/adapters/tui" {
				return errors.New("compiled public entry contains another package selector")
			}
		}
		seen[entry]++
	}
	for _, entry := range entries {
		if seen[entry] != 1 {
			return fmt.Errorf("compiled candidate requires exactly one explicit %s entry", entry)
		}
	}
	return nil
}

func (driver *Driver) requireTUIHostGateSeparation() error {
	state := driver.tuiGates
	if state == nil {
		return errors.New("TUI gate wiring was not inspected")
	}
	for _, name := range []string{"tmux", "WezTerm"} {
		if !strings.Contains(state.monitorGuide, name) {
			return fmt.Errorf("TUI guide omits %s host navigation", name)
		}
	}
	if !strings.Contains(state.policy, "separate native host proof") || !strings.Contains(state.policy, "fake callbacks do not establish terminal restoration") || !strings.Contains(state.quick, "./tests/host/evidence-verifier ./tests/host/integration") {
		return errors.New("host evidence is not retained as a separate native proof boundary")
	}
	return nil
}
