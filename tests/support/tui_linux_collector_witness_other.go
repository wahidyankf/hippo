//go:build !linux

package support

import (
	"context"
	"errors"
	"os/exec"
)

// RunPublicLinuxCollectorWitness observes the exact released Linux status child.
// Other platforms retain their existing host-query witness and cannot use this route.
func RunPublicLinuxCollectorWitness(_ context.Context, _ *exec.Cmd, _ string) (PublicTUIHostProbeTrace, error) {
	return PublicTUIHostProbeTrace{}, errors.New("linux syscall collector witness is unavailable on this platform")
}
