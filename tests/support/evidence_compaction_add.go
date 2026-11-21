package support

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/tests/contract"
)

// compactionWindows is the shared root and clock the compaction scenario uses.
type compactionWindows struct {
	root string
	now  time.Time
}

// compactionSeed is one file the scenario writes, with the time it is last modified.
type compactionSeed struct {
	path     string
	modified time.Time
}

func (driver *Driver) evidenceCompactionAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^completed prior-day evidence and aged raw and history archives in one shared root$`, driver.seedCompactionWindows),
		step(`^evidence retention is enforced on the next day$`, driver.enforceCompactionWindows),
		step(`^prior-day evidence is archived, today's stream is kept as written, and only archives within seven days of raw and thirty days of history remain$`, driver.requireCompactionWindows),
	}
}

// seedCompactionWindows writes a prior-day stream, a current stream, two raw archives on either side of
// the seven-day window, and two history archives on either side of the thirty-day window.
func (driver *Driver) seedCompactionWindows() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	for _, directory := range []string{"raw", "history"} {
		if err = os.MkdirAll(filepath.Join(root, directory), 0o700); err != nil {
			return err
		}
	}
	seeds := []compactionSeed{
		{path: filepath.Join(root, "run-day.jsonl"), modified: now.Add(-20 * time.Hour)},
		{path: filepath.Join(root, "run-today.jsonl"), modified: now.Add(-time.Hour)},
		{path: filepath.Join(root, "raw", "run-recent.jsonl.gz"), modified: now.Add(-6 * day)},
		{path: filepath.Join(root, "raw", "run-old.jsonl.gz"), modified: now.Add(-8 * day)},
		{path: filepath.Join(root, "history", "2026-08-19.jsonl.gz"), modified: now.Add(-29 * day)},
		{path: filepath.Join(root, "history", "2026-08-17.jsonl.gz"), modified: now.Add(-31 * day)},
	}
	for _, seed := range seeds {
		if err = os.WriteFile(seed.path, []byte("{\"sample\":1}\n"), 0o600); err != nil {
			return err
		}
		if err = os.Chtimes(seed.path, seed.modified, seed.modified); err != nil {
			return err
		}
	}
	driver.addCLI.compaction = compactionWindows{root: root, now: now}

	return nil
}

func (driver *Driver) enforceCompactionWindows() error {
	state := driver.addCLI.compaction
	if state.root == "" {
		return fmt.Errorf("no evidence root was seeded")
	}

	return evidence.Cleanup(state.root, state.now)
}

// requireCompactionWindows checks that the prior-day stream became a raw archive and left no plain copy,
// that the current stream is still plain, and that only the in-window archives survive.
func (driver *Driver) requireCompactionWindows() error {
	root := driver.addCLI.compaction.root
	expectations := []struct {
		path string
		want bool
	}{
		{filepath.Join(root, "raw", "run-day.jsonl.gz"), true},
		{filepath.Join(root, "run-day.jsonl"), false},
		{filepath.Join(root, "run-today.jsonl"), true},
		{filepath.Join(root, "raw", "run-recent.jsonl.gz"), true},
		{filepath.Join(root, "raw", "run-old.jsonl.gz"), false},
		{filepath.Join(root, "history", "2026-08-19.jsonl.gz"), true},
		{filepath.Join(root, "history", "2026-08-17.jsonl.gz"), false},
	}
	for _, expectation := range expectations {
		_, err := os.Stat(expectation.path)
		present := err == nil
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if present != expectation.want {
			return fmt.Errorf("%s present=%t, want %t", expectation.path, present, expectation.want)
		}
	}

	return nil
}
