package host

import (
	"context"
	"io/fs"
	"os"
)

// fileRoute holds the calls a file read makes before it chooses a reader: the absolute path, the metadata, and the
// two readers. The darwin file binds the real ones, and a test binds canned ones.
type fileRoute struct {
	Abs     func(path string) (string, error)
	Stat    func(name string) (fs.FileInfo, error)
	Regular ContextFileReader
	FIFO    ContextFileReader
}

// readRouted reads path with the reader its kind needs: a named pipe goes to FIFO, and anything else to Regular.
func readRouted(ctx context.Context, path string, route fileRoute) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absolute, err := route.Abs(path)
	if cancelled := ctx.Err(); cancelled != nil {
		return nil, cancelled
	}
	if err != nil {
		return nil, &os.PathError{Op: "abs", Path: path, Err: err}
	}
	metadata, err := route.Stat(absolute)
	if cancelled := ctx.Err(); cancelled != nil {
		return nil, cancelled
	}
	if err != nil {
		return nil, err
	}
	if metadata.Mode()&os.ModeNamedPipe == 0 {
		return route.Regular(ctx, absolute)
	}
	return route.FIFO(ctx, absolute)
}
