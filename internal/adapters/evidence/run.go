package evidence

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	domain "github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

// RunWriter owns bounded sample persistence and exclusive summary publication.
type RunWriter struct {
	*domain.RunAccumulator

	host        system.System
	output      *Writer
	summaryPath string
}

// NewRunWriter creates an exclusive rotating evidence stream below root.
func NewRunWriter(root, identifier string, limits Limits) (*RunWriter, error) {
	return newRunWriter(system.Production, root, identifier, limits)
}

func newRunWriter(port system.System, root, identifier string, limits Limits) (*RunWriter, error) {
	output, err := newWriter(port, filepath.Join(root, identifier+".jsonl"), limits)
	if err != nil {
		return nil, err
	}

	return &RunWriter{
		host:           port,
		output:         output,
		summaryPath:    filepath.Join(root, identifier+".summary.json"),
		RunAccumulator: domain.NewRunAccumulator(identifier),
	}, nil
}

// Append records one sample before updating its lifetime aggregates.
func (writer *RunWriter) Append(sample policy.Sample) error {
	if writer == nil || writer.output == nil {
		return errors.New("evidence writer is closed")
	}
	if err := writer.output.AppendJSON(sample); err != nil {
		return err
	}
	writer.RunAccumulator.Append(sample)
	return nil
}

// Finalize closes the sample stream and writes its aggregate summary once.
func (writer *RunWriter) Finalize(
	taskClass policy.TaskClass, outcome domain.Outcome, healthFailures int,
) (domain.RunSummary, error) {
	if writer == nil || writer.output == nil {
		return domain.RunSummary{}, errors.New("evidence writer is closed")
	}

	if err := writer.output.Close(); err != nil {
		return domain.RunSummary{}, err
	}
	writer.output = nil

	summary := writer.Complete(taskClass, outcome, healthFailures)
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return domain.RunSummary{}, err
	}

	port := writer.host
	file, err := port.Files.CreateTemp(filepath.Dir(writer.summaryPath), ".summary-*.tmp")
	if err != nil {
		return domain.RunSummary{}, err
	}
	temporaryPath := file.Name()
	defer func() { _ = port.Files.Remove(temporaryPath) }()
	if err = file.Chmod(0o600); err != nil {
		_ = file.Close()

		return domain.RunSummary{}, err
	}

	buffer := bufio.NewWriter(file)
	_, writeError := buffer.Write(append(encoded, '\n'))
	flushError := buffer.Flush()
	syncError := file.Sync()
	closeError := file.Close()

	if err := errors.Join(writeError, flushError, syncError, closeError); err != nil {
		return domain.RunSummary{}, err
	}
	if err = port.Files.Link(temporaryPath, writer.summaryPath); err != nil {
		return domain.RunSummary{}, err
	}

	return summary, nil
}

var unsafeIdentifier = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// Identifier returns a filesystem-safe, process-specific evidence name.
func Identifier(prefix string, now time.Time, pid int) string {
	return unsafeIdentifier.ReplaceAllString(fmt.Sprintf("%s-%d-%d", prefix, now.UnixMilli(), pid), "-")
}
