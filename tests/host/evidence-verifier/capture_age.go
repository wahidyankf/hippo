package main

import (
	"errors"
	"io/fs"
	"regexp"
	"strconv"
	"time"
)

type captureClock struct{ beforeStart, beforeEnd, afterStart, afterEnd time.Duration }

var visibleAge = regexp.MustCompile(`(?m)^(?:LIVE|STALE|No observation)(?: \| refreshing)? \| age ([^ |\n]+)(?: \||$)`)

// A completed manual refresh uses the observation readiness bound. Ordinary
// input and blocked-worker responsiveness retain their separate 500 ms bound.
func completedManualRefresh(step recordedStep) bool {
	for _, planned := range sequence(hostTmux) {
		if planned.id == step.ID {
			return planned.input == "r" && step.Input == planned.input &&
				step.Width == planned.width && step.Height == planned.height
		}
	}
	return false
}

func parseCaptureClock(fields map[string]string) (captureClock, error) {
	return parseCaptureClockForStep(fields, recordedStep{})
}

func parseCaptureClockForStep(fields map[string]string, step recordedStep) (captureClock, error) {
	values := make([]time.Duration, 0, 4)
	for _, key := range []string{"beforeCaptureStartMonotonicNS", "beforeCaptureEndMonotonicNS", "captureStartMonotonicNS", "captureEndMonotonicNS"} {
		value, err := strconv.ParseInt(fields[key], 10, 64)
		if err != nil || value <= 0 {
			return captureClock{}, errors.New("native capture requires actual finite monotonic observation windows")
		}
		values = append(values, time.Duration(value))
	}
	clock := captureClock{values[0], values[1], values[2], values[3]}
	transitionLimit := 500 * time.Millisecond
	if completedManualRefresh(step) {
		transitionLimit = 3 * time.Second
	}
	if clock.beforeEnd < clock.beforeStart || clock.afterStart < clock.beforeEnd || clock.afterEnd < clock.afterStart ||
		clock.beforeEnd-clock.beforeStart > 3*time.Second || clock.afterEnd-clock.afterStart > 3*time.Second ||
		clock.afterEnd-clock.beforeEnd > transitionLimit {
		return captureClock{}, errors.New("native capture clock order or action/completion bounds differ")
	}
	return clock, nil
}

func readCaptureClock(step recordedStep, artifacts fs.FS) (captureClock, error) {
	data, err := readArtifact(artifacts, step.InputReceipt)
	if err != nil {
		return captureClock{}, err
	}
	fields, err := receiptFields(data)
	if err != nil {
		return captureClock{}, err
	}
	return parseCaptureClockForStep(fields, step)
}

func verifyDisplayedAge(data []byte, age int) error {
	matches := visibleAge.FindAllSubmatch(data, -1)
	if len(matches) != 1 || age < 0 {
		return errors.New("capture omits one actual displayed sample age")
	}
	duration, err := time.ParseDuration(string(matches[0][1]))
	if err != nil || duration < 0 || duration%time.Second != 0 || duration/time.Second != time.Duration(age) {
		return errors.New("inspection age differs from the unmodified actual native capture")
	}
	return nil
}

func verifyAgeAdvance(before, after int, start, end time.Duration) error {
	elapsed := end - start
	if before < 0 || after < before || elapsed < 0 || elapsed > 180*time.Second {
		return errors.New("observed sample age reset or actual elapsed clock differs")
	}
	// Whole seconds and a one-second display clock can differ by one tick.
	// Use the complete measured capture windows; do not reset or ignore age.
	maximum := int(elapsed/time.Second) + 1
	if elapsed%time.Second != 0 {
		maximum++
	}
	if after-before > maximum {
		return errors.New("observed sample age exceeds finite actual elapsed/display tick bound")
	}
	return nil
}

func verifyCaptureAge(step recordedStep, artifacts fs.FS) error {
	clock, err := readCaptureClock(step, artifacts)
	if err != nil {
		return err
	}
	for _, value := range []struct {
		reference artifact
		age       int
	}{{step.BeforeCapture, step.Before.SampleAgeSeconds}, {step.Capture, step.After.SampleAgeSeconds}} {
		data, err := readArtifact(artifacts, value.reference)
		if err != nil {
			return err
		}
		if err := verifyDisplayedAge(data, value.age); err != nil {
			return err
		}
	}
	if completedManualRefresh(step) {
		if step.After.SampleAgeSeconds != 0 {
			return errors.New("successful refresh did not reset actual displayed age")
		}
		return nil
	}
	return verifyAgeAdvance(step.Before.SampleAgeSeconds, step.After.SampleAgeSeconds, clock.beforeStart, clock.afterEnd)
}

func verifyAgeContinuity(prior, current recordedStep, artifacts fs.FS) error {
	if prior.InputReceipt.Path == "" && current.InputReceipt.Path == "" {
		// Old pure fixtures remain strict; clock changes require actual receipts.
		if prior.After.SampleAgeSeconds != current.Before.SampleAgeSeconds {
			return errors.New("age changed without actual capture clocks")
		}
		return nil
	}
	before, err := readCaptureClock(prior, artifacts)
	if err != nil {
		return err
	}
	after, err := readCaptureClock(current, artifacts)
	if err != nil {
		return err
	}
	if after.beforeStart < before.afterEnd {
		return errors.New("ordered actual capture clocks overlap or reverse")
	}
	return verifyAgeAdvance(prior.After.SampleAgeSeconds, current.Before.SampleAgeSeconds, before.afterStart, after.beforeEnd)
}
