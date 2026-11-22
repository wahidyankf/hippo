package evidence //nolint:testpackage // Host-seam tests drive the unexported functions that take the system port.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/policy"
)

// redStart is the fixed clock every host-seam test reads.
var redStart = time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)

// redRoot is the evidence root the host-seam tests write under.
const redRoot = "/hippo/evidence"

// errRed is the failure a test injects into one port call.
var errRed = errors.New("injected port failure")

func redHost() *systemtest.Host { return systemtest.NewHost(redStart) }

// redDirs registers directories in the host so that its double lists them.
func redDirs(host *systemtest.Host, dirs ...string) {
	for _, dir := range dirs {
		host.Files.Dirs[dir] = true
	}
}

// redPlace stores data under name and stamps its modification time in the double.
func redPlace(host *systemtest.Host, name string, data []byte, modified time.Time) {
	host.Files.Contents[name] = data
	host.Files.Modified[name] = modified
}

// redMarshal encodes value as JSON or fails the test.
func redMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

// redGzip compresses the lines, one record per line, the way an archive stores them.
func redGzip(t *testing.T, lines ...[]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	for _, line := range lines {
		if _, err := writer.Write(append(append([]byte{}, line...), '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return buffer.Bytes()
}

// redSummary is a passed run that finished at the given instant.
func redSummary(runID string, finished time.Time) Summary {
	return Summary{
		SchemaVersion: 5, RunID: runID, Source: "hippo", ResourceTier: "standard",
		TaskClass: policy.RecordedClass(policy.TaskEphemeral), Outcome: Recorded(OutcomePassed),
		FinishedAt: finished.UTC().Format(time.RFC3339Nano),
	}
}

// redFileFaults names the calls on one file handle that fail. A zero field passes the call to the wrapped file.
type redFileFaults struct {
	read, sync, chmod, closeErr, stat error
	// writeFrom is the 1-based number of the first write that fails, and every write after it. Zero fails none.
	writeFrom int
	// info, when set, is what Stat reports in place of the wrapped file's information.
	info fs.FileInfo
}

// redFile applies redFileFaults to a file the host double opened or created.
type redFile struct {
	system.File
	faults redFileFaults
	writes int
}

func (file *redFile) Read(buffer []byte) (int, error) {
	if file.faults.read != nil {
		return 0, file.faults.read
	}

	return file.File.Read(buffer)
}

func (file *redFile) Write(data []byte) (int, error) {
	file.writes++
	if file.faults.writeFrom != 0 && file.writes >= file.faults.writeFrom {
		return 0, errRed
	}

	return file.File.Write(data)
}

func (file *redFile) Sync() error {
	if file.faults.sync != nil {
		return file.faults.sync
	}

	return file.File.Sync()
}

func (file *redFile) Chmod(mode fs.FileMode) error {
	if file.faults.chmod != nil {
		return file.faults.chmod
	}

	return file.File.Chmod(mode)
}

func (file *redFile) Close() error {
	err := file.File.Close()
	if file.faults.closeErr != nil {
		return file.faults.closeErr
	}

	return err
}

func (file *redFile) Stat() (fs.FileInfo, error) {
	if file.faults.stat != nil {
		return nil, file.faults.stat
	}
	if file.faults.info != nil {
		return file.faults.info, nil
	}

	return file.File.Stat()
}

// redStatFunc is the Stat of a directory handle as the double answers it, passed to a fault so that the fault
// can answer some names itself and delegate the rest.
type redStatFunc func(name string) (fs.FileInfo, error)

// redDirFaults names the calls on one directory handle that fail. When only is set, the name-based calls
// fail only for that entry. Files and directories the handle opens inherit the faults, and the file faults
// apply to the files it opens.
type redDirFaults struct {
	only                                                     string
	openFile, openDir, mkdir, rename, remove, chmod, readDir error
	closeErr                                                 error
	stat                                                     func(name string, real redStatFunc) (fs.FileInfo, error)
	file                                                     redFileFaults
}

// redDirectory applies redDirFaults to a directory handle the host double opened.
type redDirectory struct {
	system.Directory
	faults redDirFaults
}

func (dir redDirectory) hits(name string) bool {
	return dir.faults.only == "" || dir.faults.only == name
}

func (dir redDirectory) OpenFile(name string, flag int, perm fs.FileMode) (system.File, error) {
	if dir.faults.openFile != nil && dir.hits(name) {
		return nil, dir.faults.openFile
	}
	file, err := dir.Directory.OpenFile(name, flag, perm)
	if err != nil || !dir.hits(name) {
		return file, err
	}

	return &redFile{File: file, faults: dir.faults.file}, nil
}

func (dir redDirectory) OpenDirectory(name string) (system.Directory, error) {
	if dir.faults.openDir != nil && dir.hits(name) {
		return nil, dir.faults.openDir
	}
	child, err := dir.Directory.OpenDirectory(name)
	if err != nil {
		return nil, err
	}

	return redDirectory{Directory: child, faults: dir.faults}, nil
}

func (dir redDirectory) Mkdir(name string, perm fs.FileMode) error {
	if dir.faults.mkdir != nil && dir.hits(name) {
		return dir.faults.mkdir
	}

	return dir.Directory.Mkdir(name, perm)
}

func (dir redDirectory) Rename(oldname, newname string) error {
	if dir.faults.rename != nil && dir.hits(oldname) {
		return dir.faults.rename
	}

	return dir.Directory.Rename(oldname, newname)
}

func (dir redDirectory) Remove(name string) error {
	if dir.faults.remove != nil && dir.hits(name) {
		return dir.faults.remove
	}

	return dir.Directory.Remove(name)
}

func (dir redDirectory) Stat(name string) (fs.FileInfo, error) {
	if dir.faults.stat != nil {
		return dir.faults.stat(name, dir.Directory.Stat)
	}

	return dir.Directory.Stat(name)
}

func (dir redDirectory) ReadDir(count int) ([]fs.DirEntry, error) {
	if dir.faults.readDir != nil {
		return nil, dir.faults.readDir
	}

	return dir.Directory.ReadDir(count)
}

func (dir redDirectory) Chmod(mode fs.FileMode) error {
	if dir.faults.chmod != nil {
		return dir.faults.chmod
	}

	return dir.Directory.Chmod(mode)
}

func (dir redDirectory) Close() error {
	err := dir.Directory.Close()
	if dir.faults.closeErr != nil {
		return dir.faults.closeErr
	}

	return err
}

// redWrapOpenFiles makes every file the host opens or creates carry the faults.
func redWrapOpenFiles(port *system.System, faults redFileFaults) {
	open := port.Files.OpenFile
	port.Files.OpenFile = func(name string, flag int, perm fs.FileMode) (system.File, error) {
		file, err := open(name, flag, perm)
		if err != nil {
			return nil, err
		}

		return &redFile{File: file, faults: faults}, nil
	}
	create := port.Files.CreateTemp
	port.Files.CreateTemp = func(dir, pattern string) (system.File, error) {
		file, err := create(dir, pattern)
		if err != nil {
			return nil, err
		}

		return &redFile{File: file, faults: faults}, nil
	}
}

// redWrapOpenDirectory makes every directory the host opens carry the faults.
func redWrapOpenDirectory(port *system.System, faults redDirFaults) {
	open := port.Files.OpenDirectory
	port.Files.OpenDirectory = func(name string) (system.Directory, error) {
		dir, err := open(name)
		if err != nil {
			return nil, err
		}

		return redDirectory{Directory: dir, faults: faults}, nil
	}
}

// redListing runs change on the nth listing of dir, or on every listing when nth is zero.
// Other listings pass through the double unchanged.
func redListing(port *system.System, dir string, nth int, change func([]fs.DirEntry) ([]fs.DirEntry, error)) {
	base := port.Files.ReadDir
	calls := 0
	port.Files.ReadDir = func(name string) ([]fs.DirEntry, error) {
		if name != dir {
			return base(name)
		}
		calls++
		entries, err := base(name)
		if err != nil || nth != 0 && calls != nth {
			return entries, err
		}

		return change(entries)
	}
}

// redEntry is a listed entry whose Info is computed by a test.
type redEntry struct {
	fs.DirEntry
	info func(fs.DirEntry) (fs.FileInfo, error)
}

func (entry redEntry) Info() (fs.FileInfo, error) { return entry.info(entry.DirEntry) }

// redEntryInfo makes the listing of the entry's directory describe entryPath through info.
func redEntryInfo(port *system.System, entryPath string, info func(fs.DirEntry) (fs.FileInfo, error)) {
	dir := path.Dir(entryPath)
	redListing(port, dir, 0, func(entries []fs.DirEntry) ([]fs.DirEntry, error) {
		for index, entry := range entries {
			if path.Join(dir, entry.Name()) == entryPath {
				entries[index] = redEntry{DirEntry: entry, info: info}
			}
		}

		return entries, nil
	})
}

// redSized reports the real entry's identity with a size chosen by a test.
type redSized struct {
	fs.FileInfo
	size int64
}

func (info redSized) Size() int64 { return info.size }

// redEntrySize makes the entry at entryPath report the given size in every listing.
func redEntrySize(port *system.System, entryPath string, size int64) {
	redEntryInfo(port, entryPath, func(entry fs.DirEntry) (fs.FileInfo, error) {
		real, err := entry.Info()
		if err != nil {
			return nil, err
		}

		return redSized{FileInfo: real, size: size}, nil
	})
}

// redReadDirFails makes every listing of dir fail with err.
func redReadDirFails(port *system.System, dir string, err error) {
	base := port.Files.ReadDir
	port.Files.ReadDir = func(name string) ([]fs.DirEntry, error) {
		if name == dir {
			return nil, err
		}

		return base(name)
	}
}

// redReadFile makes the nth read of file return data and err. Zero for nth fails every read.
func redReadFile(port *system.System, file string, nth int, data []byte, err error) {
	base := port.Files.ReadFile
	calls := 0
	port.Files.ReadFile = func(name string) ([]byte, error) {
		if name != file {
			return base(name)
		}
		calls++
		if nth != 0 && calls != nth {
			return base(name)
		}

		return data, err
	}
}
