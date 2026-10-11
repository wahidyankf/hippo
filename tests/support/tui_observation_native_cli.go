package support

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
)

func (driver *Driver) inspectTUINativeStatusCompatibility(ctx context.Context) error {
	state := driver.tuiObservation
	body, err := state.runNativeObservationCLI(ctx, "status.json", []string{statusCommandName, jsonFlag, configFlag, state.native.config})
	if err != nil {
		return err
	}
	state.statusJSON = []byte(strings.TrimSpace(string(body)))
	watchContext, cancel := context.WithCancel(ctx)
	defer cancel()
	state.watchCancel = cancel
	state.watchWaitBase = state.waits
	state.native.watching = true
	defer func() { state.native.watching = false }()
	body, err = state.runNativeObservationCLI(watchContext, "watch.jsonl", []string{watchCommandName, jsonFlag, configFlag, state.native.config, intervalFlagName, "1s"})
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		if line != "" {
			state.watchBodies = append(state.watchBodies, []byte(line))
		}
	}
	return nil
}

func (state *tuiObservationState) runNativeObservationCLI(ctx context.Context, name string, args []string) ([]byte, error) {
	output, err := os.Create(filepath.Join(state.native.root, name))
	if err != nil {
		return nil, err
	}
	diagnostics, err := os.Create(filepath.Join(state.native.root, name+".stderr"))
	if err != nil {
		return nil, errors.Join(err, output.Close())
	}
	boundary := cli.Application{Observer: state.services(), Stdout: output, Stderr: diagnostics}
	code, runError := boundary.Run(ctx, args)
	closeError := errors.Join(output.Close(), diagnostics.Close())
	if runError != nil || code != 0 {
		return nil, errors.Join(fmt.Errorf("actual observation CLI failed: %d/%w", code, runError), closeError)
	}
	if closeError != nil {
		return nil, closeError
	}
	data, err := os.ReadFile(diagnostics.Name())
	if err != nil {
		return nil, err
	}
	if len(data) != 0 {
		return nil, fmt.Errorf("actual observation CLI emitted unexpected diagnostic %q", data)
	}
	return os.ReadFile(output.Name())
}
