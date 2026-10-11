package application

import "github.com/wahidyankf/hippo/internal/identity"

// DisplayOwnership describes authenticated ownership relative to one invocation.
type DisplayOwnership uint8

const (
	// DisplayOwnershipUnknown is unconfirmed ownership and grants no mutation.
	DisplayOwnershipUnknown DisplayOwnership = iota
	// DisplayOwnershipOwned is this invocation's authenticated token.
	DisplayOwnershipOwned
	// DisplayOwnershipInherited belongs to the authenticated enclosing owner.
	DisplayOwnershipInherited
	// DisplayOwnershipForeign belongs to another invocation.
	DisplayOwnershipForeign
)

// DisplayRegistrationKind preserves reservation and exclusive compensation rules.
type DisplayRegistrationKind uint8

const (
	// DisplayReservationRegistration publishes a reservation identity.
	DisplayReservationRegistration DisplayRegistrationKind = iota
	// DisplayWaiterRegistration publishes a registered FIFO waiter.
	DisplayWaiterRegistration
	// DisplayExclusiveRegistration publishes an exclusive compatibility session.
	DisplayExclusiveRegistration
)

// DisplayRegistration carries safe identity and authenticated registration facts.
type DisplayRegistration struct {
	RunID      string
	Label      identity.DisplayLabel
	Kind       DisplayRegistrationKind
	Ownership  DisplayOwnership
	Registered bool
}

// DisplayRegistrationEffects performs mechanics inside one existing native lock.
type DisplayRegistrationEffects struct {
	Publish func(string, identity.DisplayLabel) error
	Commit  func() error
	Remove  func(string) error
}

// RegisterDisplay publishes new owned context before the ownership transaction.
// Annotation failures remain advisory; exclusive failed commits compensate their publication.
func RegisterDisplay(request DisplayRegistration, effects DisplayRegistrationEffects, advisories *DisplayAdvisories) error {
	publish := request.Ownership == DisplayOwnershipOwned && !request.Registered
	if publish {
		if err := effects.Publish(request.RunID, request.Label); err != nil && advisories != nil {
			advisories.RecordPublication(err)
		}
	}
	err := effects.Commit()
	if err != nil && publish && request.Kind == DisplayExclusiveRegistration {
		if removeError := effects.Remove(request.RunID); removeError != nil && advisories != nil {
			advisories.RecordRemoval(removeError)
		}
	}
	return err
}

// DisplayRetirement identifies the invocation whose ownership is being removed.
type DisplayRetirement struct {
	RunID     string
	Ownership DisplayOwnership
}

// DisplayRetirementResult records positive removal or retained ownership facts.
type DisplayRetirementResult struct{ Confirmed, Retained bool }

// DisplayRetirementEffects keeps ownership and annotation removal in one lock.
type DisplayRetirementEffects struct {
	RemoveOwnership func() (DisplayRetirementResult, error)
	Remove          func(string) error
}

// RetireDisplay removes matching context only after confirmed owned-token retirement.
// Foreign, inherited, unknown, retained, and failed ownership transitions cannot retire context.
func RetireDisplay(request DisplayRetirement, effects DisplayRetirementEffects, advisories *DisplayAdvisories) error {
	if request.Ownership != DisplayOwnershipOwned {
		return nil
	}
	result, err := effects.RemoveOwnership()
	if err != nil {
		return err
	}
	if result.Confirmed && !result.Retained {
		if removeError := effects.Remove(request.RunID); removeError != nil && advisories != nil {
			advisories.RecordRemoval(removeError)
		}
	}
	return nil
}

// DiscardUnregisteredDisplay removes this invocation's authenticated unpublished context.
// It is distinct from retirement: the failed registration never committed an owned token.
func DiscardUnregisteredDisplay(request DisplayRetirement, remove func(string) error, advisories *DisplayAdvisories) {
	if request.Ownership != DisplayOwnershipOwned {
		return
	}
	if err := remove(request.RunID); err != nil && advisories != nil {
		advisories.RecordRemoval(err)
	}
}

// ResolveDisplayLabel preserves validated explicit context or the safe basename.
func ResolveDisplayLabel(command string, label identity.DisplayLabel) identity.DisplayLabel {
	if label.String() != "" {
		return label
	}
	return identity.ExecutableDisplayLabel(command)
}
