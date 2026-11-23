package bootstrap_test

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/bootstrap"
)

func TestWithDefaultsTakesUnsetOutputStreamsFromTheHost(t *testing.T) {
	doubles := systemtest.NewHost(time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC))
	application := bootstrap.WithDefaults(doubles.Port(), cli.Application{})

	if application.Stdout != io.Writer(&doubles.Environment.Output) {
		t.Fatalf("unset stdout did not take the host's stream: got %T", application.Stdout)
	}
	if application.Stderr != io.Writer(&doubles.Environment.Errors) {
		t.Fatalf("unset stderr did not take the host's stream: got %T", application.Stderr)
	}
}

func TestExecuteRunsTheCommandOnTheHostStreams(t *testing.T) {
	doubles := systemtest.NewHost(time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC))
	code := bootstrap.Execute(t.Context(), doubles.Port(), []string{"version"})

	if want := fmt.Sprintf("%s (%s)\n", cli.Version, cli.Commit); code != 0 || doubles.Environment.Output.String() != want {
		t.Fatalf("execute: code=%d output=%q, want 0 and %q", code, doubles.Environment.Output.String(), want)
	}
}

func TestConformanceServiceReadsTheClockPort(t *testing.T) {
	doubles := systemtest.NewHost(time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC))
	service := bootstrap.ConformanceService(doubles.Port().Clock)
	doubles.Clock.Current = doubles.Clock.Current.Add(time.Hour)

	if got := service.Now(); !got.Equal(doubles.Clock.Current) {
		t.Fatalf("conformance clock = %s, want the port's %s", got, doubles.Clock.Current)
	}
}
