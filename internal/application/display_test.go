package application_test

import (
	"errors"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
)

func TestDisplayAvailabilityHasSafeClosedWords(t *testing.T) {
	cases := []struct {
		availability application.DisplayAvailability
		word         string
	}{
		{application.DisplayUnavailable, "unavailable"},
		{application.DisplayAvailable, "available"},
		{application.DisplayMissing, "missing"},
		{application.DisplayInvalid, "invalid"},
		{application.DisplayUnsupported, "unsupported"},
		{application.DisplayUnreadable, "unreadable"},
		{application.DisplayAvailability(255), "unavailable"},
	}
	for _, test := range cases {
		if test.availability.String() != test.word {
			t.Fatalf("availability %d exposes %q want=%q", test.availability, test.availability.String(), test.word)
		}
	}
}

func TestRunDisplayInvalidExplicitLabelPrecedesEveryEffect(t *testing.T) {
	for _, label := range []string{"", "../private", "bad\x1b[31m"} {
		code, err := (application.RunEntryServices{}).Execute(t.Context(), application.RunRequest{Command: []string{"payload"}, Label: label, LabelSet: true})
		var failure status.Failure
		if code != 0 || !errors.As(err, &failure) || failure.Code != status.CodeArgsInvalid || failure.Field != "--label" {
			t.Fatalf("invalid explicit label reached dependencies or lost usage failure: %d %v", code, err)
		}
	}
}

func TestRunDisplayPreparationKeepsNamingIndependentOfIdentityAndArguments(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		fixture := newRunEntryContractFixture()
		request := runEntryContractRequest()
		request.Command = []string{"/usr/bin/printf", "private argument"}
		request.WorkingDir = "/private/working-directory"
		request.Label, request.LabelSet = "build-web", explicit
		request.Tags = []string{"role=test"}
		code, err := fixture.entry().Execute(t.Context(), request)
		want := "printf"
		if explicit {
			want = "build-web"
		}
		if err != nil || code != 0 || fixture.config.Label.String() != want || fixture.config.ReservationMetadata.Source != "hippo" || fixture.config.ReservationMetadata.Tags["role"] != "test" {
			t.Fatalf("prepared context mixed display label with identity: %d %v %+v", code, err, fixture.config)
		}
	}
}
