// Package systemtest provides in-memory doubles for the operating-system ports in package system.
//
// Its doubles let a unit test drive an adapter without touching the file system, the clock, the environment, a
// process, or the network. Only _test.go files and the tests tree may import it; production code never does.
package systemtest
