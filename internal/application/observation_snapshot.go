package application

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

func cloneObservationPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneObservationSample(sample policy.Sample) policy.Sample {
	sample.Capabilities = slices.Clone(sample.Capabilities)
	sample.AvailableMemoryBytes = cloneObservationPointer(sample.AvailableMemoryBytes)
	sample.AvailableNonCompressedEstimateBytes = cloneObservationPointer(sample.AvailableNonCompressedEstimateBytes)
	sample.MemoryPressureLevel = cloneObservationPointer(sample.MemoryPressureLevel)
	sample.CompressorAvailable = cloneObservationPointer(sample.CompressorAvailable)
	sample.CompressorPayloadBytes = cloneObservationPointer(sample.CompressorPayloadBytes)
	sample.CPUUtilizationPercent = cloneObservationPointer(sample.CPUUtilizationPercent)
	sample.DiskFreeBytes = cloneObservationPointer(sample.DiskFreeBytes)
	sample.DiskTotalBytes = cloneObservationPointer(sample.DiskTotalBytes)
	sample.PageSizeBytes = cloneObservationPointer(sample.PageSizeBytes)
	sample.CompressorStoredPages = cloneObservationPointer(sample.CompressorStoredPages)
	sample.CompressorOccupiedPages = cloneObservationPointer(sample.CompressorOccupiedPages)
	sample.SwapIns = cloneObservationPointer(sample.SwapIns)
	sample.SwapOuts = cloneObservationPointer(sample.SwapOuts)
	sample.SwapTotalBytes = cloneObservationPointer(sample.SwapTotalBytes)
	sample.SwapUsedBytes = cloneObservationPointer(sample.SwapUsedBytes)
	sample.SwapFreeBytes = cloneObservationPointer(sample.SwapFreeBytes)
	sample.SwapAllowanceBytes = cloneObservationPointer(sample.SwapAllowanceBytes)
	sample.MemoryPSISomeAvg10 = cloneObservationPointer(sample.MemoryPSISomeAvg10)
	sample.MemoryPSIFullAvg10 = cloneObservationPointer(sample.MemoryPSIFullAvg10)
	sample.OOMEvents = cloneObservationPointer(sample.OOMEvents)
	sample.OOMKillEvents = cloneObservationPointer(sample.OOMKillEvents)
	return sample
}

func cloneObservationEntries(entries []coordination.ReservationEntry) []coordination.ReservationEntry {
	result := slices.Clone(entries)
	for index := range result {
		result[index].Tags = maps.Clone(result[index].Tags)
	}
	return result
}

func cloneObservationStatus(view StatusView) StatusView {
	view.Sample = cloneObservationSample(view.Sample)
	view.Profile.FallbackChain = slices.Clone(view.Profile.FallbackChain)
	view.Coordination.Owners = cloneObservationEntries(view.Coordination.Owners)
	view.Coordination.Waiters = cloneObservationEntries(view.Coordination.Waiters)
	view.Coordination.AbandonedProcessGroups = slices.Clone(view.Coordination.AbandonedProcessGroups)
	return view
}

// ObservationRequest freezes validated invocation values without reading providers.
type ObservationRequest struct {
	configPath, diskPath, source string
	requestedProfile             policy.ProfileName
	tags                         map[string]string
}

// PrepareObservation validates source before tags without performing observation I/O.
func PrepareObservation(request StatusRequest) (ObservationRequest, error) {
	if err := identity.ValidateOverrides(request.Source, nil); err != nil {
		return ObservationRequest{}, status.Fail(status.CodeArgsInvalid, "--source: %v", err)
	}
	tags, err := identity.ParseTags(request.Tags)
	if err != nil {
		return ObservationRequest{}, status.Fail(status.CodeArgsInvalid, "--tag: %v", err)
	}
	return ObservationRequest{configPath: request.ConfigPath, diskPath: request.DiskPath, source: request.Source, requestedProfile: request.RequestedProfile, tags: maps.Clone(tags)}, nil
}

// ObservationRow carries ledger evidence and its explicit display availability.
type ObservationRow struct {
	Entry          coordination.ReservationEntry `json:"Entry"`
	Label          DisplayAnnotation             `json:"Label"`
	RequestedKnown bool                          `json:"RequestedKnown"`
	AllocatedKnown bool                          `json:"AllocatedKnown"`
	MinimumKnown   bool                          `json:"MinimumKnown"`
	MaximumKnown   bool                          `json:"MaximumKnown"`
	RegisteredAt   *time.Time                    `json:"RegisteredAt"`
	Deadline       *time.Time                    `json:"Deadline"`
}

// ObservationSnapshot freezes a status observation for asynchronous display.
// Status remains the sole legacy JSON and watch projection.
type ObservationSnapshot struct {
	Status             StatusView       `json:"Status"`
	SampledAt          *time.Time       `json:"SampledAt"`
	Running            []ObservationRow `json:"Running"`
	Queue              []ObservationRow `json:"Queue"`
	CapacityKnown      bool             `json:"CapacityKnown"`
	UsageKnown         bool             `json:"UsageKnown"`
	QueueRegistered    bool             `json:"QueueRegistered"`
	AnnotationAdvisory string           `json:"AnnotationAdvisory"`
}

func observationTime(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func observationVectorKnown(vector coordination.ReservationVector) bool {
	return vector.CPU > 0 && vector.MemoryBytes > 0
}

func observationRows(entries []coordination.ReservationEntry, registered bool, annotations map[string]DisplayAnnotation) []ObservationRow {
	rows := make([]ObservationRow, len(entries))
	for index, entry := range cloneObservationEntries(entries) {
		annotation := normalizeObservationAnnotation(annotations[entry.RunID])
		rows[index] = ObservationRow{
			Entry: entry, Label: annotation,
			RequestedKnown: registered && observationVectorKnown(entry.Requested),
			AllocatedKnown: registered && observationVectorKnown(entry.Allocated),
			MinimumKnown:   registered && observationVectorKnown(entry.Minimum),
			MaximumKnown:   registered && observationVectorKnown(entry.Maximum),
			RegisteredAt:   observationTime(entry.RegisteredAt), Deadline: observationTime(entry.Deadline),
		}
	}
	return rows
}

func normalizeObservationAnnotation(annotation DisplayAnnotation) DisplayAnnotation {
	switch annotation.Availability {
	case DisplayAvailable:
		if annotation.Label.String() != "" {
			return annotation
		}
	case DisplayMissing, DisplayInvalid, DisplayUnsupported, DisplayUnreadable:
		return DisplayAnnotation{Availability: annotation.Availability}
	case DisplayUnavailable:
	}
	return DisplayAnnotation{}
}

func observationIDs(totals coordination.ReservationTotals) []string {
	ids := make([]string, 0, len(totals.Owners)+len(totals.Waiters))
	seen := make(map[string]bool, cap(ids))
	for _, entries := range [][]coordination.ReservationEntry{totals.Owners, totals.Waiters} {
		for _, entry := range entries {
			if !seen[entry.RunID] {
				seen[entry.RunID] = true
				ids = append(ids, entry.RunID)
			}
		}
	}
	return ids
}

func (services ObservationServices) projectObservation(ctx context.Context, root string, view StatusView) ObservationSnapshot {
	view = cloneObservationStatus(view)
	registered := view.Coordination.Mode == "reservation"
	result := ObservationSnapshot{
		Status: view, SampledAt: observationTime(view.MeasuredAt),
		CapacityKnown: registered && observationVectorKnown(view.Coordination.Capacity),
		UsageKnown:    registered, QueueRegistered: registered,
	}
	var annotations map[string]DisplayAnnotation
	if services.Annotations != nil {
		var err error
		annotations, err = services.Annotations.ReadDisplay(ctx, root, observationIDs(view.Coordination))
		if err != nil {
			annotations = nil
			result.AnnotationAdvisory = "Display labels unavailable"
		}
	}
	result.Running = observationRows(view.Coordination.Owners, registered, annotations)
	result.Queue = observationRows(view.Coordination.Waiters, registered, annotations)
	return result
}
