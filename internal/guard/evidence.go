package guard

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/wahidyankf/resource-guard/internal/evidence"
	"github.com/wahidyankf/resource-guard/internal/policy"
)

// EvidenceWriter appends bounded private samples and produces one lifetime summary.
type EvidenceWriter struct {
	output      *evidence.Writer
	summaryPath string
	summary     EvidenceSummary
	first       *policy.Sample
	last        *policy.Sample
	cpu         *evidence.Histogram
	resolution  policy.Resolution
	configHash  string
}

// NewEvidenceWriter creates an exclusive rotating evidence stream below root.
func NewEvidenceWriter(root, identifier string, limits evidence.Limits) (*EvidenceWriter, error) {
	output, err := evidence.NewWriter(filepath.Join(root, identifier+".jsonl"), limits)
	if err != nil {
		return nil, err
	}

	return &EvidenceWriter{
		output:      output,
		summaryPath: filepath.Join(root, identifier+".summary.json"),
		summary: EvidenceSummary{
			SchemaVersion:          3,
			CompressorAvailableAll: true,
		},
		cpu: evidence.NewHistogram(100, 0.01),
	}, nil
}

// SetContext attaches resolved, non-sensitive policy metadata to the summary.
func (writer *EvidenceWriter) SetContext(resolution policy.Resolution, configHash string) {
	writer.resolution = resolution
	writer.configHash = configHash
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}

	result := *value

	return &result
}

func copyInt(value *int) *int {
	if value == nil {
		return nil
	}

	result := *value

	return &result
}

func minimum(current, candidate *int64) *int64 {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate < *current {
		return copyInt64(candidate)
	}

	return current
}

func maximumInt64(current, candidate *int64) *int64 {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate > *current {
		return copyInt64(candidate)
	}

	return current
}

func maximumInt(current, candidate *int) *int {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate > *current {
		return copyInt(candidate)
	}

	return current
}

// Append records one sample and updates fixed-memory lifetime aggregates.
func (writer *EvidenceWriter) Append(sample policy.Sample) error {
	if writer == nil || writer.output == nil {
		return errors.New("evidence writer is closed")
	}
	if err := writer.output.AppendJSON(sample); err != nil {
		return err
	}

	if writer.first == nil {
		first := sample
		writer.first = &first
		writer.summary.AvailableParallelism = sample.AvailableParallelism
		writer.summary.Platform = sample.Platform
		writer.summary.Capabilities = append([]string(nil), sample.Capabilities...)
	}

	last := sample
	writer.last = &last
	writer.summary.SampleCount++

	available := sample.AvailableMemoryBytes
	if available == nil {
		available = sample.AvailableNonCompressedEstimateBytes
	}

	writer.summary.AvailableNonCompressedEstimateMinBytes = minimum(writer.summary.AvailableNonCompressedEstimateMinBytes, available)
	writer.summary.MemoryPressureLevelMax = maximumInt(writer.summary.MemoryPressureLevelMax, sample.MemoryPressureLevel)
	writer.summary.CompressorPayloadPeakBytes = maximumInt64(writer.summary.CompressorPayloadPeakBytes, sample.CompressorPayloadBytes)
	writer.summary.DiskFreeMinBytes = minimum(writer.summary.DiskFreeMinBytes, sample.DiskFreeBytes)
	writer.summary.SwapFreeMinBytes = minimum(writer.summary.SwapFreeMinBytes, sample.SwapFreeBytes)

	if sample.CompressorAvailable == nil || !*sample.CompressorAvailable {
		writer.summary.CompressorAvailableAll = false
	}
	if sample.CPUUtilizationPercent != nil {
		writer.cpu.Add(*sample.CPUUtilizationPercent)
	}

	return nil
}

// EvidenceSummary captures bounded aggregate evidence for one guarded task.
type EvidenceSummary struct {
	SchemaVersion                          int      `json:"schemaVersion"`
	SampleCount                            int      `json:"sampleCount"`
	TaskClass                              string   `json:"taskClass"`
	Outcome                                string   `json:"outcome"`
	AvailableParallelism                   int      `json:"availableParallelism"`
	AvailableNonCompressedEstimateMinBytes *int64   `json:"availableNonCompressedEstimateMinBytes"`
	MemoryPressureLevelMax                 *int     `json:"memoryPressureLevelMax"`
	CompressorAvailableAll                 bool     `json:"compressorAvailableAll"`
	CompressorPayloadPeakBytes             *int64   `json:"compressorPayloadPeakBytes"`
	CPUUtilizationP95Percent               float64  `json:"cpuUtilizationP95Percent"`
	DiskFreeMinBytes                       *int64   `json:"diskFreeMinBytes"`
	SwapInsDelta                           int64    `json:"swapInsDelta"`
	SwapOutsDelta                          int64    `json:"swapOutsDelta"`
	SwapFreeMinBytes                       *int64   `json:"swapFreeMinBytes"`
	HealthFailures                         int      `json:"healthFailures"`
	Platform                               string   `json:"platform,omitempty"`
	Capabilities                           []string `json:"capabilities,omitempty"`
	RequestedProfile                       string   `json:"requestedProfile,omitempty"`
	ResolvedProfile                        string   `json:"resolvedProfile,omitempty"`
	FallbackChain                          []string `json:"fallbackChain,omitempty"`
	Concurrency                            int      `json:"concurrency,omitempty"`
	ConfigHash                             string   `json:"configHash,omitempty"`
}

func delta(first, last *int64) int64 {
	if first == nil || last == nil || *last <= *first {
		return 0
	}

	return *last - *first
}

// Finalize closes the sample stream and writes its aggregate summary once.
func (writer *EvidenceWriter) Finalize(taskClass policy.TaskClass, outcome string, healthFailures int) (EvidenceSummary, error) {
	if writer == nil || writer.output == nil {
		return EvidenceSummary{}, errors.New("evidence writer is closed")
	}

	if err := writer.output.Close(); err != nil {
		return EvidenceSummary{}, err
	}
	writer.output = nil

	writer.summary.TaskClass = string(taskClass)
	writer.summary.Outcome = outcome
	writer.summary.HealthFailures = healthFailures
	writer.summary.RequestedProfile = writer.resolution.RequestedProfile
	writer.summary.ResolvedProfile = writer.resolution.ResolvedProfile
	writer.summary.FallbackChain = append([]string(nil), writer.resolution.FallbackChain...)
	writer.summary.Concurrency = writer.resolution.Concurrency
	writer.summary.ConfigHash = writer.configHash

	if writer.first != nil && writer.last != nil {
		writer.summary.SwapInsDelta = delta(writer.first.SwapIns, writer.last.SwapIns)
		writer.summary.SwapOutsDelta = delta(writer.first.SwapOuts, writer.last.SwapOuts)
	}
	if value, ok := writer.cpu.Quantile(0.95); ok {
		writer.summary.CPUUtilizationP95Percent = value
	}

	encoded, err := json.MarshalIndent(writer.summary, "", "  ")
	if err != nil {
		return EvidenceSummary{}, err
	}

	file, err := os.OpenFile(writer.summaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return EvidenceSummary{}, err
	}

	buffer := bufio.NewWriter(file)
	_, writeError := buffer.Write(append(encoded, '\n'))
	flushError := buffer.Flush()
	closeError := file.Close()

	if err := errors.Join(writeError, flushError, closeError); err != nil {
		return EvidenceSummary{}, err
	}

	return writer.summary, nil
}

var unsafeIdentifier = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// EvidenceIdentifier returns a filesystem-safe, process-specific evidence name.
func EvidenceIdentifier(prefix string, now time.Time, pid int) string {
	return unsafeIdentifier.ReplaceAllString(fmt.Sprintf("%s-%d-%d", prefix, now.UnixMilli(), pid), "-")
}
