package tui

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
)

func (state *model) syncBody() {
	if state.view == dashboardView || !state.supportedSize() {
		return
	}
	state.body.SetWidth(state.width)
	state.body.SetHeight(max(0, state.height-5-len(state.stateLines())))
	lines := state.bodyLines()
	for index, line := range lines {
		lines[index] = wrapText(line, state.width)
	}
	state.body.SetContent(strings.Join(lines, "\n"))
}

func (state *model) bodyLines() []string {
	switch state.view {
	case jobView:
		return state.jobLines()
	case capacityView:
		return state.capacityLines()
	case helpView:
		return state.helpLines()
	case dashboardView:
		return nil
	}
	return nil
}

func (state *model) detailRow() (application.ObservationRow, section, bool) {
	for _, active := range []section{runningSection, queueSection} {
		for _, row := range state.rows(active) {
			if row.Entry.RunID == state.detailID {
				return row, active, true
			}
		}
	}
	return application.ObservationRow{}, runningSection, false
}

func (state *model) jobLines() []string {
	row, active, found := state.detailRow()
	if !found {
		return []string{"job no longer active", "Run ID: " + state.detailID, "b back to current dashboard"}
	}
	label := "unavailable (" + row.Label.Availability.String() + ")"
	if row.Label.Availability == application.DisplayAvailable {
		label = row.Label.Label.String()
	}
	lines := []string{
		"Run ID: " + row.Entry.RunID,
		"Label: " + label,
		"Source: " + nonemptyText(row.Entry.Source),
		"State: " + state.sectionName(active) + " " + nonemptyText(row.Entry.State),
		"Class: " + nonemptyText(string(row.Entry.Class)),
		"Profile: " + nonemptyText(string(row.Entry.Profile)),
		"Tier: " + nonemptyText(row.Entry.Tier),
		"Requested: " + vectorText(row.Entry.Requested, row.RequestedKnown),
		"Allocated: " + vectorText(row.Entry.Allocated, row.AllocatedKnown),
		"Minimum: " + vectorText(row.Entry.Minimum, row.MinimumKnown),
		"Maximum: " + vectorText(row.Entry.Maximum, row.MaximumKnown),
		"Registered: " + observationDate(row.RegisteredAt),
		"Deadline: " + observationDate(row.Deadline),
	}
	if active == queueSection {
		lines = append(lines, fmt.Sprintf("FIFO position: %d", row.Entry.Position))
	}
	if row.Entry.Legacy {
		lines = append(lines, "Legacy owner: resource allocation unavailable")
	}
	lines = append(lines, state.rowElapsed(row))
	keys := make([]string, 0, len(row.Entry.Tags))
	for key := range row.Entry.Tags {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		lines = append(lines, "Tag "+key+": "+row.Entry.Tags[key])
	}
	return lines
}

func observationDate(value *time.Time) string {
	if value == nil || value.IsZero() {
		return unavailableText
	}
	return value.UTC().Format(time.RFC3339)
}

func (state *model) rowElapsed(row application.ObservationRow) string {
	if row.RegisteredAt == nil || row.RegisteredAt.IsZero() || state.now.IsZero() {
		return "Elapsed: unavailable"
	}
	return "Elapsed: " + max(0, state.now.Sub(*row.RegisteredAt)).Truncate(time.Second).String()
}

func (state *model) capacityLines() []string {
	sample := state.snapshot.Status.Sample
	totals := state.snapshot.Status.Coordination
	queue := "unregistered"
	if state.snapshot.QueueRegistered {
		queue = fmt.Sprintf("%d registered waiters", len(state.snapshot.Queue))
	}
	return []string{
		"Host capacity and pressure",
		"Host parallelism: " + positiveText(sample.AvailableParallelism),
		"Host CPU utilization: " + state.cpuText(),
		"Physical RAM: " + positiveBytes(sample.PhysicalMemoryBytes),
		"Effective RAM limit: " + positiveBytes(sample.EffectiveMemoryLimitBytes),
		"Available RAM: " + optionalBytes(sample.AvailableMemoryBytes),
		"Non-compressed estimate: " + optionalBytes(sample.AvailableNonCompressedEstimateBytes),
		"Swap total: " + optionalBytes(sample.SwapTotalBytes),
		"Swap used: " + optionalBytes(sample.SwapUsedBytes),
		"Swap free: " + optionalBytes(sample.SwapFreeBytes),
		"Swap-ins: " + optionalCount(sample.SwapIns),
		"Swap-outs: " + optionalCount(sample.SwapOuts),
		"Swap state: " + nonemptyText(sample.SwapState),
		"Disk free: " + optionalBytes(sample.DiskFreeBytes),
		"Disk total: " + optionalBytes(sample.DiskTotalBytes),
		"Pressure: " + nonemptyText(string(state.snapshot.Status.Resource.State)),
		"Pressure reason: " + nonemptyText(state.snapshot.Status.Resource.Reason),
		"Reservation accounting",
		"Mode: " + nonemptyText(totals.Mode),
		"Capacity: " + vectorText(totals.Capacity, state.snapshot.CapacityKnown),
		"Allocated: " + vectorText(totals.Allocated, state.snapshot.UsageKnown),
		"Waiting request: " + vectorText(totals.Waiting, state.snapshot.UsageKnown),
		fmt.Sprintf("Running: %d registered rows", len(state.snapshot.Running)),
		"Queue: " + queue,
		fmt.Sprintf("Global owners: %d", totals.ActiveOwners),
		fmt.Sprintf("Global waiting owners: %d", totals.WaitingOwners),
		"Base owners: " + positiveText(state.snapshot.Status.Promotion.BaseOwners),
		"Effective owners: " + positiveText(state.snapshot.Status.Promotion.EffectiveOwners),
		"Maximum owners: " + positiveText(state.snapshot.Status.Promotion.MaximumOwners),
		"Promotion reason: " + nonemptyText(state.snapshot.Status.Promotion.Reason),
		fmt.Sprintf("Class ephemeral: %d", totals.Ephemeral),
		fmt.Sprintf("Class service: %d", totals.Service),
		fmt.Sprintf("Class transactional: %d", totals.Transactional),
		"Allocations describe reservations, not process RSS or CPU usage.",
	}
}

func optionalCount(value *int64) string {
	if value == nil {
		return unavailableText
	}
	return strconv.FormatInt(*value, 10)
}

func positiveBytes(value int64) string {
	if value <= 0 {
		return unavailableText
	}
	return bytesText(value)
}

func (state *model) cpuText() string {
	value := state.snapshot.Status.CPUUtilizationPercent
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return unavailableText
	}
	return fmt.Sprintf("%.0f%%", *value)
}

func (state *model) helpLines() []string {
	return []string{
		"Help: keyboard actions",
		"Available in every view:",
		"r refresh (one queued request while busy)",
		"? help; q quit; Ctrl-C interrupt",
		"b back from this help to the previous view",
		"Dashboard actions:",
		"Tab or n: next section",
		"Shift-Tab or p: previous section",
		"Down or j: select next job",
		"Up or k: select previous job",
		"Enter: open selected job detail",
		"c: open capacity detail",
		"Detail, capacity and help:",
		"Down/j and Up/k: scroll one row",
		"b: back to dashboard (help restores prior view)",
		"Running and Queue selection is retained by complete run ID.",
		"Resize preserves selection; minimum dashboard size is 40x16.",
		"Letters work without modifiers on mobile terminals.",
		"Host prefixes and double Escape remain available to the terminal host.",
	}
}
