package runtime

import (
	"encoding/json"

	"github.com/wahidyankf/hippo/internal/policy"
)

// CompatibilityOwnerRecord is the existing lease and session wire record. Its
// class is decoded tolerantly before each reader applies its ownership rules.
type CompatibilityOwnerRecord struct {
	SchemaVersion  int                      `json:"schemaVersion"`
	PID            int                      `json:"pid"`
	Token          string                   `json:"token,omitempty"`
	Port           int                      `json:"port,omitempty"`
	Owner          string                   `json:"owner,omitempty"`
	Class          policy.RecordedTaskClass `json:"class,omitzero"`
	IdentityDevice uint64                   `json:"identityDevice,omitempty"`
	IdentityInode  uint64                   `json:"identityInode,omitempty"`
}

type leaseOwner = CompatibilityOwnerRecord

// NewExclusiveSessionRecord prepares the owned compatibility session's wire
// fields from the authenticated identity supplied by registration.
func NewExclusiveSessionRecord(pid int, token string, class policy.TaskClass, device, inode uint64) CompatibilityOwnerRecord {
	return CompatibilityOwnerRecord{
		SchemaVersion:  1,
		PID:            pid,
		Token:          token,
		Class:          policy.RecordedClass(class),
		IdentityDevice: device,
		IdentityInode:  inode,
	}
}

// Encode returns exactly the newline-terminated bytes persisted by lease and
// session writers; it performs no host or filesystem operation.
func (record CompatibilityOwnerRecord) Encode() ([]byte, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	return append(data, '\n'), nil
}

// CoordinationMarkerRecord is the existing shared-root mode wire record.
type CoordinationMarkerRecord struct {
	SchemaVersion int    `json:"schemaVersion"`
	Mode          string `json:"mode"`
}

type coordinationMarker = CoordinationMarkerRecord

// NewExclusiveCoordinationMarker prepares the compatibility mode wire record.
func NewExclusiveCoordinationMarker() CoordinationMarkerRecord {
	return CoordinationMarkerRecord{
		SchemaVersion: coordinationSchemaVersion,
		Mode:          coordinationModeExclusive,
	}
}

// Encode returns exactly the newline-terminated bytes persisted by the shared
// mode writer; it performs no host or filesystem operation.
func (record CoordinationMarkerRecord) Encode() ([]byte, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	return append(data, '\n'), nil
}
