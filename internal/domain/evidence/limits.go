package evidence

const (
	defaultChunkBytes int64 = 400 * 1024
	defaultChunkCount       = 5
)

// Limits bounds one live raw evidence stream.
type Limits struct {
	ChunkBytes int64
	Chunks     int
}

// DefaultLimits retains five recent 400 KiB chunks per live session.
func DefaultLimits() Limits {
	return Limits{ChunkBytes: defaultChunkBytes, Chunks: defaultChunkCount}
}
