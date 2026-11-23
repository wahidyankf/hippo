//go:build darwin

package host

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func readFileContext(ctx context.Context, path string) ([]byte, error) {
	return readRouted(ctx, path, fileRoute{
		Abs:     filepath.Abs,
		Stat:    os.Stat,
		Regular: readUnixFileContext,
		FIFO:    readDarwinFIFO,
	})
}

func newDarwinFIFOCommand(ctx context.Context, path string, output *bytes.Buffer) *exec.Cmd {
	command := exec.CommandContext(ctx, "/bin/cat", "--", path)
	command.Stdout = output
	command.Stderr = io.Discard
	return command
}

func readDarwinFIFO(ctx context.Context, path string) ([]byte, error) {
	var output bytes.Buffer
	command := newDarwinFIFOCommand(ctx, path, &output)
	return runDarwinFIFOCommand(ctx, path, fifoCommandOperations{start: command.Start, wait: command.Wait, output: output.Bytes})
}
