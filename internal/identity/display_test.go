package identity_test

import (
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/identity"
)

func TestDisplayLabelExplicitValidation(t *testing.T) {
	for _, value := range []string{"check-docs", "Build 2._:@+-", strings.Repeat("a", 64)} {
		label, err := identity.ParseDisplayLabel(value)
		if err != nil || label.String() != value {
			t.Fatalf("valid label %q changed: %q %v", value, label.String(), err)
		}
	}
	for _, value := range []string{"", strings.Repeat("a", 65), "../build", "a..b", "/private/build", `private\build`, "_build", " build", "build\nnext", "build\x1b[31m", "build\x00", "build\u754c"} {
		t.Run(strings.ReplaceAll(value, "\x00", "NUL"), func(t *testing.T) {
			label, err := identity.ParseDisplayLabel(value)
			if err == nil || label.String() != "" || strings.Contains(err.Error(), value) && value != "" {
				t.Fatalf("invalid input did not fail without echoing payload: label=%q error=%v", label.String(), err)
			}
		})
	}
	if (identity.DisplayLabel{}).String() != "" {
		t.Fatal("zero label invented available context")
	}
}

func TestDisplayLabelDerivationUsesOnlySanitizedBasename(t *testing.T) {
	cases := []struct{ command, want string }{
		{"/usr/bin/printf", "printf"},
		{`C:\private\printf`, "printf"},
		{"/private/_@-build", "build"},
		{"/private/a..b", "ab"},
		{"/private/a...b", "a.b"},
		{"/private/a\x1b.b", "a.b"},
		{"/private/\u754c", "command"},
		{"", "command"},
		{"/", "command"},
		{"/private/" + strings.Repeat("x", 65), strings.Repeat("x", 64)},
	}
	for _, test := range cases {
		label := identity.ExecutableDisplayLabel(test.command)
		validated, err := identity.ParseDisplayLabel(label.String())
		if label.String() != test.want || err != nil || validated != label {
			t.Fatalf("basename %q => %q want=%q validation=%v", test.command, label.String(), test.want, err)
		}
	}
}
