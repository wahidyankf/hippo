package hostprobe_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/support/hostprobe"
)

// This exact entry emits only collector output. Ordinary unmarked tests do
// nothing; marked success exits naturally without testing progress on stdout.
func TestBDDPublicTUIHostProbeHelper(_ *testing.T) {
	if os.Getenv(hostprobe.HelperEnvironment) != "1" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := hostprobe.Run(ctx)
	cancel()
	if err != nil {
		diagnostic := err.Error()
		if len(diagnostic) > 4096 {
			diagnostic = diagnostic[:4096]
		}
		fmt.Fprintln(os.Stderr, diagnostic)
		os.Exit(1)
	}
	os.Exit(0)
}
