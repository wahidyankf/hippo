//go:build linux

package host

import "context"

func readFileContext(ctx context.Context, path string) ([]byte, error) {
	return readUnixFileContext(ctx, path)
}
