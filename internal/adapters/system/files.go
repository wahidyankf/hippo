package system

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// openFile opens name with os.OpenFile. It returns a nil File on failure, so a failed open never yields an interface
// that holds a nil *os.File.
func openFile(name string, flag int, perm fs.FileMode) (File, error) { //nolint:ireturn // Callers share the File interface across host and double trees.
	file, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// createTemp creates a temporary file with os.CreateTemp, with the same nil-on-failure rule as openFile.
func createTemp(dir, pattern string) (File, error) { //nolint:ireturn // Callers share the File interface across host and double trees.
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// flock applies or releases the advisory lock how on file. Only an operating-system file has a descriptor to lock.
func flock(file File, how int) error {
	handle, ok := file.(*os.File)
	if !ok {
		return errors.New("system: only an operating-system file can be locked")
	}

	return unix.Flock(int(handle.Fd()), how)
}

// openDirectory opens the named directory, following links on its path as the open call does. Every name the
// returned Directory takes is then resolved against its descriptor, and none of them follows a link.
func openDirectory(name string) (Directory, error) { //nolint:ireturn // Callers share the Directory interface across host and double trees.
	descriptor, err := unix.Open(name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}

	return directory{file: os.NewFile(uintptr(descriptor), name)}, nil
}

// directory is an open directory whose entries are reached through its descriptor with the *at calls.
type directory struct {
	file *os.File
}

func (dir directory) descriptor() int { return int(dir.file.Fd()) }

func (dir directory) OpenFile(name string, flag int, perm fs.FileMode) (File, error) { //nolint:ireturn // The Directory interface dictates this signature.
	descriptor, err := unix.Openat(dir.descriptor(), name, flag|unix.O_CLOEXEC, uint32(perm.Perm()))
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(descriptor), name), nil
}

func (dir directory) OpenDirectory(name string) (Directory, error) { //nolint:ireturn // The Directory interface dictates this signature.
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	descriptor, err := unix.Openat(dir.descriptor(), name, flags, 0)
	if err != nil {
		return nil, err
	}

	return directory{file: os.NewFile(uintptr(descriptor), name)}, nil
}

func (dir directory) Mkdir(name string, perm fs.FileMode) error {
	return unix.Mkdirat(dir.descriptor(), name, uint32(perm.Perm()))
}

func (dir directory) Rename(oldname, newname string) error {
	return unix.Renameat(dir.descriptor(), oldname, dir.descriptor(), newname)
}

// Remove unlinks a name. It never removes a directory, so a name that is one fails.
func (dir directory) Remove(name string) error {
	return unix.Unlinkat(dir.descriptor(), name, 0)
}

// Stat describes the named entry without following a link, so a link is described as a link.
func (dir directory) Stat(name string) (fs.FileInfo, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(dir.descriptor(), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, err
	}

	return statInfo{name: filepath.Base(name), stat: stat}, nil
}

// statInfo is the fs.FileInfo of an entry, as fstatat reports it.
type statInfo struct {
	name string
	stat unix.Stat_t
}

func (info statInfo) Name() string { return info.name }
func (info statInfo) Size() int64  { return info.stat.Size }
func (info statInfo) Mode() fs.FileMode {
	return fileMode(uint32(info.stat.Mode)) //nolint:unconvert // The uint32 conversion is required on darwin where Mode is uint16.
}
func (info statInfo) ModTime() time.Time { return modTime(&info.stat) }
func (info statInfo) IsDir() bool        { return info.Mode().IsDir() }
func (info statInfo) Sys() any           { return &info.stat }

// fileMode converts the type and permission bits of a raw mode to the fs.FileMode that describes them.
func fileMode(raw uint32) fs.FileMode {
	mode := fs.FileMode(raw & 0o777)
	switch raw & unix.S_IFMT {
	case unix.S_IFDIR:
		mode |= fs.ModeDir
	case unix.S_IFLNK:
		mode |= fs.ModeSymlink
	case unix.S_IFIFO:
		mode |= fs.ModeNamedPipe
	case unix.S_IFSOCK:
		mode |= fs.ModeSocket
	case unix.S_IFCHR:
		mode |= fs.ModeDevice | fs.ModeCharDevice
	case unix.S_IFBLK:
		mode |= fs.ModeDevice
	}

	return mode
}

func (dir directory) ReadDir(count int) ([]fs.DirEntry, error) { return dir.file.ReadDir(count) }

func (dir directory) Chmod(mode fs.FileMode) error { return dir.file.Chmod(mode) }

func (dir directory) Close() error { return dir.file.Close() }
