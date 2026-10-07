package coordination

import (
	"errors"
	"fmt"
	"regexp"
)

var portOwnerPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidatePortLeaseRequest checks the complete port range and safe owner name.
func ValidatePortLeaseRequest(port int, ownerName string, minimum, maximum int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and %d", 65535)
	}
	if port < minimum || port > maximum {
		return fmt.Errorf("port %d must be between the lease minimum %d and maximum %d", port, minimum, maximum)
	}
	if !portOwnerPattern.MatchString(ownerName) {
		return errors.New("port lease owner is invalid")
	}

	return nil
}
