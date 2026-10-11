package main

import (
	"strings"
	"testing"
)

func TestOldIdentityParsesStructuredReceiptAndRejectsMissingOrWrongFields(t *testing.T) {
	valid := `{"schemaVersion":1,"version":"v0.8.4","commit":"abde2ecbeff7513549cfd1c0805fd20b5d232c01"}`
	for _, input := range []string{valid, " " + valid + "\n", `{"commit":"abde2ecbeff7513549cfd1c0805fd20b5d232c01", "version":"v0.8.4", "schemaVersion":1}`} {
		if err := verifyOldReceipt(strings.NewReader(input)); err != nil {
			t.Errorf("conforming structured receipt rejected: %v", err)
		}
	}
	for _, row := range []struct{ name, input string }{
		{"invalid JSON", "{"},
		{"empty", ""},
		{"null", "null"},
		{"array", "[]"},
		{"missing schema", strings.Replace(valid, `"schemaVersion":1,`, "", 1)},
		{"missing version", strings.Replace(valid, `"version":"v0.8.4",`, "", 1)},
		{"missing commit", `{"schemaVersion":1,"version":"v0.8.4"}`},
		{"wrong schema", strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":2`, 1)},
		{"schema type", strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":"1"`, 1)},
		{"version type", strings.Replace(valid, `"version":"v0.8.4"`, `"version":1`, 1)},
		{"wrong version", strings.Replace(valid, "v0.8.4", "v0.8.5", 1)},
		{"wrong commit", strings.Replace(valid, "abde2ecbeff7513549cfd1c0805fd20b5d232c01", strings.Repeat("0", 40), 1)},
		{"unknown field", strings.Replace(valid, "{", `{"unexpected":true,`, 1)},
		{"two documents", valid + valid},
		{"trailing invalid bytes", valid + "garbage"},
	} {
		t.Run(row.name, func(t *testing.T) {
			if err := verifyOldReceipt(strings.NewReader(row.input)); err == nil {
				t.Fatal("invalid structured identity accepted")
			}
		})
	}
}

// The new explicit mode accepts only the immutable v0.8.5 tuple; the existing
// one-argument verifier continues rejecting v0.8.5 as the v0.8.4 negatives prove.
func TestV085IdentityRejectsForeignReleaseAndMalformedActualReceipt(t *testing.T) {
	valid := `{"schemaVersion":1,"version":"v0.8.5","commit":"456d24bc2a20ee23a7b81746c1789133cd5c3a97"}`
	if err := verifyV085Receipt(strings.NewReader(valid)); err != nil {
		t.Fatalf("actual v0.8.5 tuple rejected: %v", err)
	}
	for _, row := range []struct{ name, input string }{
		{"old release", `{"schemaVersion":1,"version":"v0.8.4","commit":"abde2ecbeff7513549cfd1c0805fd20b5d232c01"}`},
		{"foreign commit", strings.Replace(valid, "456d24bc2a20ee23a7b81746c1789133cd5c3a97", strings.Repeat("0", 40), 1)},
		{"missing schema", strings.Replace(valid, `"schemaVersion":1,`, "", 1)},
		{"future schema", strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":2`, 1)},
		{"unknown field", strings.Replace(valid, "{", `{"unexpected":true,`, 1)},
		{"invalid JSON", "{"},
		{"trailing document", valid + valid},
	} {
		t.Run(row.name, func(t *testing.T) {
			if err := verifyV085Receipt(strings.NewReader(row.input)); err == nil {
				t.Fatal("unqualified v0.8.5 identity accepted")
			}
		})
	}
}
