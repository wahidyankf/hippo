package tui_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/tests/support"
)

func (fixture *publicBDDObservation) neutralHandler(pattern string) any {
	prepare := func(variant string) func(context.Context) (context.Context, error) {
		return func(ctx context.Context) (context.Context, error) { return fixture.prepareCase(ctx, variant, "") }
	}
	switch pattern {
	case `^two running jobs and three FIFO waiters with only the third waiter matching source and tag filters$`:
		return prepare("filtered")
	case `^the filtered TUI observation completes$`, `^current observation reads the owners and their queued work$`:
		return fixture.observe
	case `^only the matching row is visible at position 3 and global capacity and usage remain unfiltered$`:
		return fixture.requireRows
	case `^a host collector with two readings and a shared root whose active mode differs from configuration$`:
		return prepare("mode-disagreement")
	case `^two host readings one second apart and the active coordination mode determine the displayed snapshot$`:
		return fixture.requireWorkflow
	case `^an empty reservation root with no active epoch$`:
		return prepare("idle")
	case `^Running and Queue are empty with zero known demand and unavailable epoch capacity$`:
		return fixture.checkRows
	case `^the observation creates no owner waiter annotation or coordination epoch$`:
		return fixture.requireReadOnly
	case `^an exclusive compatibility root with a live owner and no registered FIFO queue$`:
		return prepare("exclusive")
	case `^the owner is visible and reservation vectors and unregistered waiting demand are unavailable$`:
		return fixture.requireRows
	case `^a reservation owner whose optional metadata and display annotation are missing$`:
		return prepare("legacy")
	case `^its live run ID and ledger allocation remain known while its label is unavailable$`:
		return fixture.requireRows
	case `^live job IDs sharing a short prefix and annotations for one live ID and one absent ID$`:
		return prepare("identity")
	case `^only the exact matching live ID receives its label without adding or reordering jobs$`:
		return fixture.requireRows
	case `^a live job whose display annotation is (missing|malformed|mismatched run ID|unsupported schema|larger than 512 bytes|unreadable|a symlink)$`:
		return func(ctx context.Context, fault string) (context.Context, error) {
			return fixture.prepareCase(ctx, "annotation-fault", fault)
		}
	case `^its label is unavailable and the live identity vectors and global accounting remain unchanged$`:
		return fixture.checkRows
	case `^the annotation advisory exposes no unchecked payload or private path$`:
		return fixture.requirePrivateAdvisory
	case `^old and current owners sharing unchanged coordination schemas$`:
		return prepare("mixed")
	case `^owners without display annotations remain visible with unavailable labels$`:
		return fixture.checkRows
	case `^admission accounting and FIFO ordering remain unchanged$`:
		return fixture.finishObservation
	default:
		return nil
	}
}

func (fixture *publicBDDObservation) visibleRows(queue bool) []coordination.ReservationEntry {
	expected := fixture.ledger.Expected()
	if fixture.variant == "filtered" {
		if queue {
			return expected.Waiters[2:3]
		}
		return nil
	}
	if queue {
		return expected.Waiters
	}
	return expected.Owners
}

func (fixture *publicBDDObservation) panelHeader(name string, rows []coordination.ReservationEntry, index int) string {
	position := 0
	if len(rows) != 0 {
		position = index + 1
	}
	return fmt.Sprintf("%s %d/%d", name, position, len(rows))
}

func publicBDDVector(vector coordination.ReservationVector) string {
	memory := fmt.Sprintf("%dB", vector.MemoryBytes)
	if vector.MemoryBytes >= 1<<30 {
		memory = fmt.Sprintf("%.1fGiB", float64(vector.MemoryBytes)/(1<<30))
	}
	return fmt.Sprintf("%dCPU %s", vector.CPU, memory)
}

func (fixture *publicBDDObservation) labelFragment(id string) string {
	if label := fixture.ledger.Labels()[id]; label != "" {
		return "Label: " + label
	}
	availability := "missing"
	if fixture.variant == "annotation-fault" {
		switch fixture.annotationFault {
		case "malformed", "mismatched run ID", "larger than 512 bytes":
			availability = "invalid"
		case "unsupported schema":
			availability = "unsupported"
		case "unreadable", "a symlink":
			availability = "unreadable"
		}
	}
	return "Label: unavailable (" + availability + ")"
}

// checkRows compares public status to independently declared authentic input
// and reads every visible full identity through the real selected child's UI.
// It never constructs an application snapshot or invokes the native model.
func (fixture *publicBDDObservation) checkRows(ctx context.Context) error {
	if fixture.transport == nil {
		return errors.New("public observation has no actual terminal")
	}
	if err := fixture.ledger.RequirePublicTotals(fixture.status.Coordination); err != nil {
		return err
	}
	expected := fixture.ledger.Expected()
	for _, panel := range []struct {
		name string
		rows []coordination.ReservationEntry
	}{{"Running", fixture.visibleRows(false)}, {"Queue", fixture.visibleRows(true)}} {
		if err := fixture.checkPublicPanelRows(ctx, panel.name, panel.rows, expected.Mode == "exclusive"); err != nil {
			return err
		}
	}
	if err := fixture.checkPublicCapacity(ctx, expected); err != nil {
		return err
	}
	return fixture.ledger.VerifyLiveness(ctx)
}

func (fixture *publicBDDObservation) checkPublicPanelRows(ctx context.Context, name string, rows []coordination.ReservationEntry, exclusive bool) error {
	if name == "Queue" {
		if err := fixture.key(ctx, "n", "HIPPO | Running and Queue", fixture.panelHeader(name, rows, 0)); err != nil {
			return err
		}
	}
	if len(rows) == 0 {
		if _, err := fixture.await(ctx, []string{fixture.panelHeader(name, rows, 0), "No " + strings.ToLower(name) + " jobs"}, nil); err != nil {
			return err
		}
	}
	for index, row := range rows {
		if err := fixture.checkPublicDetailRow(ctx, name, rows, index, row, exclusive); err != nil {
			return err
		}
	}
	return nil
}

func (fixture *publicBDDObservation) checkPublicDetailRow(ctx context.Context, name string, rows []coordination.ReservationEntry, index int, row coordination.ReservationEntry, exclusive bool) error {
	if index != 0 {
		if err := fixture.key(ctx, "j", fixture.panelHeader(name, rows, index), "#"+row.RunID[:8]); err != nil {
			return err
		}
	}
	source := row.Source
	if source == "" {
		source = "unavailable"
	}
	present := []string{"HIPPO | Job detail", "Run ID: " + row.RunID, "Source: " + source, "Class: " + string(row.Class), fixture.labelFragment(row.RunID)}
	if exclusive {
		present = append(present, "Requested: unavailable", "Allocated: unavailable", "Registered: unavailable")
	} else {
		present = append(present, "Requested: "+publicBDDVector(row.Requested), "Profile: "+string(row.Profile))
		if name == "Running" {
			present = append(present, "Allocated: "+publicBDDVector(row.Allocated))
		} else {
			present = append(present, "Allocated: unavailable", "FIFO position: "+strconv.Itoa(row.Position))
		}
		if row.Legacy {
			present = append(present, "Registered: unavailable")
		}
	}
	for key, value := range row.Tags {
		present = append(present, "Tag "+key+": "+value)
	}
	if err := fixture.key(ctx, "\r", present...); err != nil {
		return err
	}
	absent := []string{}
	if !row.Legacy && (fixture.variant != "mixed" || row.RegisteredAt != "") {
		absent = append(absent, "Registered: unavailable")
	}
	if _, err := fixture.await(ctx, present, absent); err != nil {
		return err
	}
	if err := fixture.key(ctx, "b", "HIPPO | Running and Queue", fixture.panelHeader(name, rows, index)); err != nil {
		return err
	}
	return nil
}

func (fixture *publicBDDObservation) checkPublicCapacity(ctx context.Context, expected coordination.ReservationTotals) error {
	if err := fixture.key(ctx, "c", "HIPPO | Capacity detail", "Host capacity and pressure"); err != nil {
		return err
	}
	for range 12 {
		bound, stop := context.WithTimeout(ctx, 500*time.Millisecond)
		err := fixture.transport.SendKey(bound, []byte("j"))
		stop()
		if err != nil {
			return err
		}
	}
	capacity, allocated, waiting := publicBDDVector(expected.Capacity), publicBDDVector(expected.Allocated), publicBDDVector(expected.Waiting)
	if expected.Capacity.CPU == 0 || expected.Capacity.MemoryBytes == 0 {
		capacity = "unavailable"
	}
	if expected.Mode == "exclusive" {
		allocated, waiting = "unavailable", "unavailable"
	}
	present := []string{"HIPPO | Capacity detail", "Mode: " + expected.Mode, "Capacity: " + capacity, "Allocated: " + allocated, "Waiting request: " + waiting, "Global owners: " + strconv.Itoa(expected.ActiveOwners), "Global waiting owners: " + strconv.Itoa(expected.WaitingOwners)}
	if expected.Mode == "exclusive" {
		present = append(present, "Queue: unregistered")
	}
	if _, err := fixture.await(ctx, present, nil); err != nil {
		return err
	}
	return nil
}

func (fixture *publicBDDObservation) finishObservation(ctx context.Context) error {
	if err := fixture.ledger.RequirePublicTotals(fixture.status.Coordination); err != nil {
		return err
	}
	if fixture.variant == "mixed" {
		schemas, err := fixture.ledger.ProtocolSchemas()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(fixture.mixedSchemas, schemas) {
			return errors.New("public observation changed shared coordination schemas")
		}
	}
	if err := fixture.finish(ctx); err != nil {
		return err
	}
	if err := fixture.ledger.VerifyLiveness(ctx); err != nil {
		return err
	}
	if fixture.variant == "mixed" {
		// Retire genuine mixed children before Godog cancels the scenario context.
		// All schema, accounting, FIFO, frame, natural-q and live-owner checks precede release.
		cleanup, stop := context.WithTimeout(ctx, 3*time.Second)
		defer stop()
		return fixture.close(cleanup)
	}
	return nil
}

func requirePublicReadingGap(trace support.PublicTUIHostProbeTrace) error {
	if trace.Readings != 2 || trace.Active != 0 || trace.MaximumActive != 1 {
		return errors.New("public observation did not settle two nonoverlapping readings")
	}
	var lastFirst, firstSecond int64
	foundFirst, foundSecond := false, false
	for _, event := range trace.Events {
		if event.Reading == 1 && (event.Phase == "output-ack" || event.Phase == "output") {
			lastFirst, foundFirst = event.ElapsedNanoseconds, true
		}
		if event.Reading == 2 && !foundSecond {
			firstSecond, foundSecond = event.ElapsedNanoseconds, true
		}
	}
	// Both timestamps have the SAME probe origin. A final output ACK precedes
	// collection completion; this lower bound never calls ACK an actual Wait.
	if !foundFirst || !foundSecond || firstSecond-lastFirst < int64(time.Second) {
		return errors.New("public readings lack the real one-second sample wait")
	}
	return nil
}

func (fixture *publicBDDObservation) requireWorkflow(ctx context.Context) error {
	if fixture.status.Coordination.Mode != "reservation" {
		return errors.New("configured exclusive mode displaced live reservation mode")
	}
	if err := requirePublicReadingGap(fixture.statusTrace); err != nil {
		return err
	}
	if err := requirePublicReadingGap(fixture.probe.Trace()); err != nil {
		return err
	}
	return fixture.requireRows(ctx)
}

func (fixture *publicBDDObservation) requireReadOnly(ctx context.Context) error {
	if err := fixture.finishObservation(ctx); err != nil {
		return err
	}
	return fixture.ledger.RequireReadOnly(fixture.idleBefore)
}

func (fixture *publicBDDObservation) requirePrivateAdvisory(ctx context.Context) error {
	if err := fixture.finishObservation(ctx); err != nil {
		return err
	}
	result := fixture.transport.Result()
	for _, raw := range [][]byte{result.Stdout, result.Stderr, result.HolderStderr, fixture.statusOutput, fixture.statusDiagnostics} {
		for _, secret := range []string{"unchecked-payload", fixture.root, fixture.directory} {
			if bytes.Contains(raw, []byte(secret)) {
				return errors.New("annotation emitted unchecked payload or an owned private path")
			}
		}
	}
	for _, frame := range fixture.frames {
		text := strings.Join(frame.Rows, "\n")
		if strings.Contains(text, "unchecked-payload") || strings.Contains(text, fixture.root) || strings.Contains(text, fixture.directory) {
			return errors.New("annotation current frame exposed unchecked data or private path")
		}
	}
	return nil
}
