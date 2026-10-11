package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/wahidyankf/hippo/internal/application"
)

func (state *model) supportedSize() bool { return state.width >= 40 && state.height >= 16 }

func (state *model) render() string {
	if state.width <= 0 || state.height <= 0 {
		return ""
	}
	if !state.supportedSize() {
		return state.renderSmall()
	}
	lines := []string{state.viewTitle()}
	lines = append(lines, state.summaryLines()...)
	if state.view == dashboardView {
		lines = append(lines, state.dashboardLines()...)
	} else {
		lines = append(lines, strings.Split(state.body.View(), "\n")...)
	}
	lines = append(lines, state.stateLines()...)
	lines = append(lines, state.footer())
	return state.fitFrame(lines)
}

func (state *model) renderSmall() string {
	if state.width < 6 {
		return "q?"[:min(2, state.width)]
	}
	if state.height == 1 {
		return fitText("q ? help", state.width)
	}
	lines := []string{fmt.Sprintf("%dx%d", state.width, state.height)}
	if state.height > 1 {
		lines = append(lines, "q ? help | q quits")
	}
	if state.height > 2 {
		lines = append(lines, "Terminal too small: need 40x16")
	}
	return state.fitFrame(lines)
}

func (state *model) fitFrame(lines []string) string {
	lines = lines[:min(len(lines), state.height)]
	for index, line := range lines {
		lines[index] = state.styleLine(fitText(line, state.width))
	}
	return strings.Join(lines, "\n")
}

func (state *model) viewTitle() string {
	switch state.view {
	case jobView:
		return "HIPPO | Job detail"
	case capacityView:
		return "HIPPO | Capacity detail"
	case helpView:
		return "HIPPO | Help"
	case dashboardView:
		return "HIPPO | Running and Queue"
	}
	return "HIPPO | Running and Queue"
}

func (state *model) summaryLines() []string {
	sample := state.snapshot.Status.Sample
	memory := sample.AvailableMemoryBytes
	if memory == nil {
		memory = sample.AvailableNonCompressedEstimateBytes
	}
	totals := state.snapshot.Status.Coordination
	return []string{
		fmt.Sprintf("CPU %s | RAM %s | Disk %s", state.cpuText(), optionalBytes(memory), optionalBytes(sample.DiskFreeBytes)),
		"Pressure " + nonemptyText(string(state.snapshot.Status.Resource.State)) + " | " + nonemptyText(totals.Mode),
		"Reservation " + vectorText(totals.Capacity, state.snapshot.CapacityKnown) + " / used " + vectorText(totals.Allocated, state.snapshot.UsageKnown),
	}
}

func (state *model) stateLine() string {
	parts := []string{"LIVE"}
	if !state.hasSnapshot {
		parts[0] = "No observation"
	}
	if state.stale {
		parts[0] = "STALE"
	}
	if state.busy {
		parts = append(parts, "refreshing")
	}
	if age, known := state.sampleAge(); known {
		parts = append(parts, "age "+age.Truncate(time.Second).String())
	} else {
		parts = append(parts, "age unavailable")
	}
	if state.snapshot.AnnotationAdvisory != "" {
		parts = append(parts, "Display labels unavailable")
	}
	return strings.Join(parts, " | ")
}

// Keep the safe error summary on its own row so compact frames retain both
// observation state and the complete message instead of truncating either.
func (state *model) stateLines() []string {
	lines := []string{state.stateLine()}
	if state.errorSummary != "" {
		lines = append(lines, "ERROR "+state.errorSummary)
	}
	return lines
}

func (state *model) footer() string {
	if state.view == dashboardView {
		return "q quit ? help r refresh | Enter detail c capacity"
	}
	return "q quit b back ? help r refresh j/k scroll"
}

func (state *model) dashboardLines() []string {
	extraStateRows := len(state.stateLines()) - 1
	if state.width >= 80 && state.height >= 24 {
		budget := state.height - 8 - extraStateRows
		first := budget / 2
		lines := state.panelLines(runningSection, first)
		return append(lines, state.panelLines(queueSection, budget-first)...)
	}
	lines := make([]string, 0, state.height-6)
	if !state.hasSnapshot {
		lines = append(lines, "Running unavailable | Queue unavailable")
	} else {
		lines = append(lines, fmt.Sprintf("Running %d | Queue %d [%s]", len(state.snapshot.Running), len(state.snapshot.Queue), state.sectionName(state.section)))
	}
	lines = append(lines, state.panelLines(state.section, state.height-7-extraStateRows)...)
	return lines
}

func (*model) sectionName(active section) string {
	if active == queueSection {
		return "Queue"
	}
	return "Running"
}

func (state *model) panelLines(active section, budget int) []string {
	if !state.hasSnapshot {
		return []string{state.sectionName(active) + " unavailable", "No observation available"}
	}
	rows := state.rows(active)
	selected := state.selected[active]
	position := 0
	if len(rows) > 0 {
		position = selected.index + 1
	}
	name := state.sectionName(active)
	header := fmt.Sprintf("%s %d/%d", name, position, len(rows))
	if active == queueSection && !state.snapshot.QueueRegistered {
		header += " unregistered"
	}
	lines := []string{header}
	if len(rows) == 0 {
		return append(lines, "No "+strings.ToLower(name)+" jobs")
	}
	count := max(0, budget-1)
	start := max(0, selected.index-count+1)
	for index := start; index < min(len(rows), start+count); index++ {
		lines = append(lines, state.rowLine(rows[index], active, index == selected.index))
	}
	return lines
}

func (state *model) rowLine(row application.ObservationRow, active section, selected bool) string {
	marker := " "
	if selected && active == state.section {
		marker = ">"
	}
	label := unavailableText
	if row.Label.Availability == application.DisplayAvailable {
		label = row.Label.Label.String()
	}
	kind, resources := "ACTIVE", vectorText(row.Entry.Allocated, row.AllocatedKnown)
	if active == queueSection {
		kind = fmt.Sprintf("WAIT #%d", row.Entry.Position)
		resources = vectorText(row.Entry.Requested, row.RequestedKnown)
	}
	id := row.Entry.RunID[:min(8, len(row.Entry.RunID))]
	return fmt.Sprintf("%s %s %s %s #%s %s", marker, kind, label, nonemptyText(row.Entry.Source), id, resources)
}

// Semantic roles use standard terminal palette slots; ASCII text carries meaning
// when invocation styling is disabled or the terminal remaps its palette.
func (state *model) styleLine(line string) string {
	if !state.styling {
		return line
	}
	style := lipgloss.NewStyle()
	switch {
	case strings.Contains(line, "ERROR") || strings.Contains(line, "critical"):
		style = style.Foreground(lipgloss.Color("1"))
	case strings.Contains(line, "STALE") || strings.Contains(line, "WAIT") || strings.Contains(line, "warning"):
		style = style.Foreground(lipgloss.Color("3"))
	case strings.HasPrefix(line, ">"):
		style = style.Bold(true).Foreground(lipgloss.Color("6"))
	case strings.HasPrefix(line, "HIPPO"):
		style = style.Bold(true)
	}
	return style.Render(line)
}
