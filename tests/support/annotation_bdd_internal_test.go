package support

import (
	"encoding/json"
	"testing"

	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestAnnotationExclusiveSchemaRejectsDrift(t *testing.T) {
	owner := runtimeadapter.NewExclusiveSessionRecord(7, annotationBDDToken, policy.TaskEphemeral, 13, 17)
	session, err := owner.Encode()
	if err != nil {
		t.Fatal(err)
	}
	marker, err := runtimeadapter.NewExclusiveCoordinationMarker().Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err = requireUnitExclusiveWire(session, marker, annotationBDDToken); err != nil {
		t.Fatalf("actual production encoding rejected: %v", err)
	}
	for _, test := range []struct {
		name         string
		marker       bool
		field, value string
	}{
		{name: "session schema drift", field: "schemaVersion", value: "2"},
		{name: "session token drift", field: "token", value: `"different-owner"`},
		{name: "session PID drift", field: "pid", value: "0"},
		{name: "session class drift", field: "class", value: `"service"`},
		{name: "session identity device drift", field: "identityDevice", value: "0"},
		{name: "session identity inode drift", field: "identityInode", value: "19"},
		{name: "session identity omission", field: "identityDevice"},
		{name: "session annotation field added", field: "label", value: `"build-web"`},
		{name: "marker schema drift", marker: true, field: "schemaVersion", value: "2"},
		{name: "marker mode drift", marker: true, field: "mode", value: `"reservation"`},
		{name: "marker annotation field added", marker: true, field: "label", value: `"build-web"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := session
			if test.marker {
				data = marker
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if test.value == "" {
				delete(fields, test.field)
			} else {
				fields[test.field] = json.RawMessage(test.value)
			}
			changed, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			changedSession, changedMarker := session, marker
			if test.marker {
				changedMarker = changed
			} else {
				changedSession = changed
			}
			if err = requireUnitExclusiveWire(changedSession, changedMarker, annotationBDDToken); err == nil {
				t.Fatal("schema drift passed the canonical Unit Then assertion")
			}
		})
	}
}
