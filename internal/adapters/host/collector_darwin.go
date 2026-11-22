//go:build darwin

package host

import (
	"context"
	"os/exec"

	"golang.org/x/sys/unix"
)

func command(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, arguments...).Output()
}

// collect gathers one macOS reading from the operating system's sysctl and the supervised probes.
func (collector SystemCollector) collect(ctx context.Context, previous CPUState, diskPath string) (Reading, error) {
	return collector.collectDarwin(ctx, previous, diskPath, darwinCalls{Physical: unix.SysctlUint64, Command: command})
}
