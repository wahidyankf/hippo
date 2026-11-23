// Package system declares the operating-system ports that the adapters reach the host through.
//
// Each port is a struct of function fields, and the Production value binds every field directly to the standard
// library. A binding that is a plain function value holds no statement, so it adds nothing to the unit coverage
// measure, and a unit test substitutes an in-memory double from the systemtest package instead of touching the host.
package system
