package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/policy"
)

func statusRequest(options statusOptions) application.StatusRequest {
	return application.StatusRequest{ConfigPath: options.configPath, DiskPath: options.diskPath, RequestedProfile: options.requestedProfile(), Source: options.source, Tags: options.tags}
}

func (boundary Application) status(ctx context.Context, options statusOptions) (int, error) {
	view, code, err := boundary.Observer.Observe(ctx, statusRequest(options))
	if err != nil || code != 0 {
		return code, err
	}
	return renderStatus(boundary.Stdout, options.jsonOutput, view)
}

func renderStatus(destination io.Writer, jsonOutput bool, view application.StatusView) (int, error) {
	if jsonOutput {
		encoded, err := json.Marshal(view)
		if err != nil {
			return 1, fmt.Errorf("encode status JSON: %w", err)
		}
		_, err = fmt.Fprintln(destination, string(encoded))
		return 0, err
	}
	available, disk, cpu := unavailableValue, unavailableValue, unavailableValue
	availableBytes := view.AvailableMemoryBytes
	if availableBytes == nil {
		availableBytes = view.AvailableNonCompressedEstimateBytes
	}

	if availableBytes != nil {
		available = fmt.Sprintf("%.2f", float64(*availableBytes)/float64(policy.GiB))
	}
	if view.DiskFreeBytes != nil {
		disk = fmt.Sprintf("%.2f", float64(*view.DiskFreeBytes)/float64(policy.GiB))
	}
	if view.CPUUtilizationPercent != nil {
		cpu = fmt.Sprintf("%.1f%%", *view.CPUUtilizationPercent)
	}

	totals := view.Coordination
	var err error

	_, err = fmt.Fprintf(
		destination,
		"state=%s reason=%s profile=%s concurrency=%d swap=%s availableGiB=%s diskFreeGiB=%s cpu=%s owners=%d waiters=%d ownerLimit=%d promotion=%s\n",
		view.Resource.State,
		view.Resource.Reason,
		view.Profile.ResolvedProfile,
		view.Profile.Concurrency,
		view.SwapState,
		available,
		disk,
		cpu,
		totals.ActiveOwners,
		totals.WaitingOwners,
		view.Promotion.EffectiveOwners,
		view.Promotion.Reason,
	)
	if err != nil {
		return 1, err
	}
	for _, entry := range append(totals.Owners, totals.Waiters...) {
		if _, err = fmt.Fprintf(
			destination,
			"%s run=%s position=%d source=%s class=%s tier=%s cpu=%d memoryMiB=%d deadline=%s\n",
			entry.State, entry.RunID, entry.Position, entry.Source, entry.Class, entry.Tier,
			max(entry.Allocated.CPU, entry.Requested.CPU),
			max(entry.Allocated.MemoryBytes, entry.Requested.MemoryBytes)/policy.MiB,
			entry.Deadline,
		); err != nil {
			return 1, err
		}
	}

	return 0, nil
}

func (boundary Application) monitor(ctx context.Context, options monitorOptions) (int, error) {
	return boundary.Observer.Monitor(ctx, application.MonitorRequest{ConfigPath: options.configPath, DiskPath: options.diskPath, RequestedProfile: options.requestedProfile(), Interval: options.interval}, func(value application.MonitorTransition) error {
		return writeMonitorTransition(boundary.Stdout, options.jsonOutput, value)
	})
}

func writeMonitorTransition(destination io.Writer, jsonOutput bool, value application.MonitorTransition) error {
	if !jsonOutput {
		_, err := fmt.Fprintf(destination, "%s state=%s reason=%s profile=%s swap=%s\n", value.MeasuredAt, value.State, value.Reason, value.Profile, value.SwapState)
		return err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode monitor transition JSON: %w", err)
	}
	_, err = fmt.Fprintln(destination, string(encoded))
	return err
}
