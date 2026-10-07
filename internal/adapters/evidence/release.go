package evidence

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	domain "github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

type monitorOutput struct {
	file   *Writer
	stream io.Writer
}

func (output *monitorOutput) Append(value domain.ReleaseSample) error {
	if output.file != nil {
		return output.file.AppendJSON(value)
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')
	written, err := output.stream.Write(encoded)
	if err == nil && written != len(encoded) {
		return io.ErrShortWrite
	}

	return err
}

func (output *monitorOutput) Close() error {
	if output.file != nil {
		return output.file.Close()
	}

	return nil
}

// OpenReleaseOutput opens one bounded raw file or caller-owned stream.
func (ReleaseRepository) OpenReleaseOutput(config application.MonitorConfig) (application.ReleaseOutput, string, error) {
	if config.RawOutput != nil {
		output := &monitorOutput{stream: config.RawOutput}
		return application.ReleaseOutput{Append: output.Append, Close: output.Close}, "", nil
	}

	root := filepath.Dir(config.OutputPath)
	if err := Cleanup(root, config.Now(), config.OutputPath, config.SummaryPath); err != nil {
		return application.ReleaseOutput{}, "", err
	}

	output, err := NewWriter(config.OutputPath, Limits{ChunkBytes: config.EvidenceLimits.ChunkBytes, Chunks: config.EvidenceLimits.Chunks})

	owned := &monitorOutput{file: output}
	return application.ReleaseOutput{Append: owned.Append, Close: owned.Close}, root, err
}

// WriteReleaseSummary stores the completed session summary exactly once.
func (ReleaseRepository) WriteReleaseSummary(path string, destination io.Writer, summary policy.ReleaseSummary) error {
	if summary.SampleCount == 0 {
		return errors.New("release monitor has no samples")
	}

	if destination != nil {
		encoded, err := json.Marshal(summary)
		if err != nil {
			return err
		}

		written, err := destination.Write(append(encoded, '\n'))
		if err == nil && written != len(encoded)+1 {
			return io.ErrShortWrite
		}

		return err
	}

	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = file.Write(append(encoded, '\n'))

	return err
}

// ReleaseRepository stores release evidence in private, bounded files.
type ReleaseRepository struct{}

// ReadReleaseSummary reads a completed release summary.
func (ReleaseRepository) ReadReleaseSummary(path string) ([]byte, error) { return os.ReadFile(path) }

// CleanupRelease retires expired evidence while retaining active destinations.
func (ReleaseRepository) CleanupRelease(root string, now time.Time, active ...string) error {
	return Cleanup(root, now, active...)
}
