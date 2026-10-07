package identity_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/identity"
)

func TestIdentityContractDecodeRefusesMalformedAndAmbiguousJSON(t *testing.T) {
	cases := []struct {
		name, data string
		contains   string
	}{
		{"empty", "", ""},
		{"unfinished key", `{"schemaVersion":1,"source`, ""},
		{"invalid key", `{1:"repo"}`, ""},
		{"invalid value", `{"schemaVersion":1,"source":!}`, ""},
		{"unclosed object", `{"schemaVersion":1,"source":"repo"`, ""},
		{"unclosed array", `[{"source":"repo"}`, ""},
		{"bad array element", `[!]`, ""},
		{"array instead of document", `[{},[1,true,null]]`, ""},
		{"scalar instead of document", `true`, ""},
		{"duplicate source", `{"schemaVersion":1,"source":"one","source":"two"}`, "duplicate identity field"},
		{"duplicate tag", `{"schemaVersion":1,"source":"repo","tags":{"role":"one","role":"two"}}`, "duplicate identity field"},
		{"duplicate nested in array", `[{"source":"one","source":"two"}]`, "duplicate identity field"},
		{"multiple JSON values", `{"schemaVersion":1,"source":"repo"} {}`, "one JSON value"},
		{"junk after document", `{"schemaVersion":1,"source":"repo"}!`, "one JSON value"},
		{"unknown field", `{"schemaVersion":1,"source":"repo","private":"hidden"}`, "unknown field"},
		{"wrong tag type", `{"schemaVersion":1,"source":"repo","tags":{"role":1}}`, ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := identity.Decode([]byte(test.data))
			if err == nil || !reflect.DeepEqual(got, identity.Value{}) {
				t.Fatalf("malformed document value=%+v error=%v", got, err)
			}
			if test.contains != "" && !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error=%v, want %q", err, test.contains)
			}
		})
	}
}

func TestIdentityContractDecodeAndResolveOwnIndependentMergedValues(t *testing.T) {
	data := []byte(`{"schemaVersion":1,"source":"repo","tags":{"group":"local","role":"old"}}`)
	decoded, err := identity.Decode(data)
	if err != nil || decoded.SchemaVersion != identity.SchemaVersion || decoded.Source != "repo" || decoded.Tags["group"] != "local" {
		t.Fatalf("decoded=%+v error=%v", decoded, err)
	}
	resolved, err := identity.Resolve(data, true, "override", []string{"role=new", "runner=first", "runner=last"})
	want := identity.Value{SchemaVersion: identity.SchemaVersion, Source: "override", Tags: map[string]string{"group": "local", "role": "new", "runner": "last"}}
	if err != nil || !reflect.DeepEqual(resolved, want) {
		t.Fatalf("resolved=%+v error=%v", resolved, err)
	}
	resolved.Tags["group"] = "changed"
	if decoded.Tags["group"] != "local" {
		t.Fatal("resolution aliased a previously decoded identity")
	}
	withoutTags, err := identity.Resolve([]byte(`{"schemaVersion":1,"source":"repo"}`), true, "", []string{"role=build"})
	if err != nil || withoutTags.Tags["role"] != "build" {
		t.Fatalf("absent document tags=%+v error=%v", withoutTags, err)
	}
	withoutFile, err := identity.Resolve([]byte("ignored malformed bytes"), false, "repo", nil)
	if err != nil || withoutFile.Tags == nil || withoutFile.Source != "repo" {
		t.Fatalf("absent file value=%+v error=%v", withoutFile, err)
	}
}

func TestIdentityContractValidateEnforcesPrivacyAndSizeBounds(t *testing.T) {
	many := map[string]string{}
	for index := range identity.MaximumTags + 1 {
		many[fmt.Sprintf("tag%d", index)] = "valid"
	}
	oversized := map[string]string{}
	for index := range identity.MaximumTags {
		oversized[fmt.Sprintf("tag%d", index)] = strings.Repeat("x", 64)
	}
	cases := []struct {
		name     string
		value    identity.Value
		contains string
	}{
		{"unsupported schema", identity.Value{SchemaVersion: 2, Source: "repo"}, "unsupported identity schema"},
		{"missing source", identity.Value{SchemaVersion: identity.SchemaVersion}, "source is required"},
		{"source path traversal", identity.Value{SchemaVersion: identity.SchemaVersion, Source: "repo..private"}, "source"},
		{"source escape", identity.Value{SchemaVersion: identity.SchemaVersion, Source: "repo\x1b[31m"}, "source"},
		{"invalid tag key", identity.Value{SchemaVersion: identity.SchemaVersion, Source: "repo", Tags: map[string]string{"Role": "build"}}, "tag key"},
		{"invalid tag value", identity.Value{SchemaVersion: identity.SchemaVersion, Source: "repo", Tags: map[string]string{"role": "/private"}}, "tag value"},
		{"tag traversal", identity.Value{SchemaVersion: identity.SchemaVersion, Source: "repo", Tags: map[string]string{"role": "one..two"}}, "tag value"},
		{"too many document tags", identity.Value{SchemaVersion: identity.SchemaVersion, Source: "repo", Tags: many}, "at most"},
		{"encoded document bound", identity.Value{SchemaVersion: identity.SchemaVersion, Source: "repo", Tags: oversized}, "encoded bytes"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := identity.Validate(test.value)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error=%v, want %q", err, test.contains)
			}
			if test.name == "missing source" && !errors.Is(err, identity.ErrSourceRequired) {
				t.Fatalf("missing-source sentinel lost: %v", err)
			}
		})
	}
	valid := identity.Value{SchemaVersion: identity.SchemaVersion, Source: strings.Repeat("a", 64), Tags: map[string]string{"role": strings.Repeat("b", 64)}}
	if err := identity.Validate(valid); err != nil {
		t.Fatalf("valid maximum-length labels rejected: %v", err)
	}
	valid.Source += "a"
	if err := identity.Validate(valid); err == nil {
		t.Fatal("65-character source accepted")
	}
}

func TestIdentityContractOverridesAndResolutionFailuresKeepTheirMeaning(t *testing.T) {
	cases := []struct {
		name     string
		data     string
		present  bool
		source   string
		tags     []string
		contains string
	}{
		{name: "decode failure", data: `{`, present: true, source: "override", contains: "decode identity"},
		{name: "unknown field", data: `{"schemaVersion":1,"source":"repo","unknown":1}`, present: true, contains: "decode identity"},
		{name: "unsupported document schema", data: `{"schemaVersion":2,"source":"repo"}`, present: true, contains: "unsupported identity schema"},
		{name: "invalid document tag", data: `{"schemaVersion":1,"source":"repo","tags":{"role":"/private"}}`, present: true, contains: "tag value"},
		{name: "invalid override", source: "repo", tags: []string{"role=../private"}, contains: "tag value"},
		{name: "no source", contains: "source is required"},
		{name: "malformed tag", source: "repo", tags: []string{"role"}, contains: "key=value"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := identity.Resolve([]byte(test.data), test.present, test.source, test.tags)
			if err == nil || !strings.Contains(err.Error(), test.contains) || !reflect.DeepEqual(got, identity.Value{}) {
				t.Fatalf("value=%+v error=%v", got, err)
			}
		})
	}
	if err := identity.ValidateOverrides("bad source", []string{"bad"}); err == nil || !strings.Contains(err.Error(), "source") {
		t.Fatalf("override precedence error=%v", err)
	}
	if err := identity.ValidateOverrides("", []string{"role=build"}); err != nil {
		t.Fatalf("optional source override error=%v", err)
	}
	if err := identity.ValidateOverrides("repo", []string{"Bad=value"}); err == nil {
		t.Fatal("invalid override key accepted")
	}
	parsed, err := identity.ParseTags([]string{"role=first", "role=last"})
	if err != nil || !reflect.DeepEqual(parsed, map[string]string{"role": "last"}) {
		t.Fatalf("repeat tags=%v error=%v", parsed, err)
	}
	many := []string{"a=1", "b=2", "c=3", "d=4", "e=5", "f=6", "g=7", "h=8", "i=9"}
	if _, err := identity.ParseTags(many); err == nil {
		t.Fatal("too many override tags accepted")
	}
}
