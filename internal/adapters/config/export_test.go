package config

// The aliases below let the external config_test package bind the host-taking seams through the in-memory
// doubles, so the production package exports nothing it does not already export.

// LoadWithHost is load, reading the local configuration through the given host.
var LoadWithHost = load

// LoadIdentityWithHost is loadIdentity, reading the identity file through the given host.
var LoadIdentityWithHost = loadIdentity

// IdentityPathWithHost is identityPath, resolving the identity file through the given host.
var IdentityPathWithHost = identityPath

// IdentityPresentWithHost is identityPresent, observing the identity file through the given host.
var IdentityPresentWithHost = identityPresent

// AbsolutePathWithHost is absolutePath, resolving a path against the given host's working directory.
var AbsolutePathWithHost = absolutePath

// DisplayPathWithHost is displayPath, formatting a path against the given host's working directory.
var DisplayPathWithHost = displayPath

// PortLeaseRootWithHost is portLeaseRoot, resolving the port-lease root through the given host.
var PortLeaseRootWithHost = portLeaseRoot

// EvidenceRootWithHost is evidenceRoot, resolving the evidence root through the given host.
var EvidenceRootWithHost = evidenceRoot

// BuildCoordination is buildCoordination, validating a decoded configuration against a catalog.
var BuildCoordination = buildCoordination

// File is the decoded configuration document.
type File = file

// ProfileOverride is one profile block of a decoded configuration document.
type ProfileOverride = profileOverride
