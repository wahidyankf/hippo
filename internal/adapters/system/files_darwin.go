package system

import (
	"time"

	"golang.org/x/sys/unix"
)

// modTime is the last modification time of a stat result. Darwin names the field Mtimespec.
func modTime(stat *unix.Stat_t) time.Time {
	return time.Unix(stat.Mtimespec.Unix())
}
