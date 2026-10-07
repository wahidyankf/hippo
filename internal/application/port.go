package application

import "fmt"

// AcquirePort owns the bounded stale-owner retry policy for one service port.
func (services RunServices) AcquirePort(config RunConfig) (*PortLease, error) {
	admission, err := services.Ports.BeginPort(config)
	if err != nil {
		return nil, err
	}
	for range 2 {
		result, attemptError := admission.Attempt()
		if attemptError != nil || !result.Retry {
			return result.Lease, attemptError
		}
	}
	return nil, fmt.Errorf("port %d could not be leased", config.LeasePort)
}
