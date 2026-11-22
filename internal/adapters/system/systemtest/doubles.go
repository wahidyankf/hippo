package systemtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

// Host is a complete in-memory system. Each field is one double, and Port assembles
// the system.System value an adapter or a composition root receives. Call Port after
// the doubles are configured: the environment's arguments, platform, and input are
// copied into the port when it is built.
type Host struct {
	Clock       *Clock
	Environment *Environment
	Signals     *Signals
	Terminal    *Terminal
	Files       *Files
	Processes   *Processes
	Runner      *Runner
	Random      *Random
	Disk        *Disk
}

// NewHost returns a host whose clock reads start, whose environment and file tree are
// empty, and whose input is empty.
func NewHost(start time.Time) *Host {
	clock := &Clock{Current: start}

	return &Host{
		Clock:       clock,
		Environment: &Environment{Values: map[string]string{}, Input: strings.NewReader("")},
		Signals:     &Signals{},
		Terminal:    &Terminal{Groups: map[string]int{}},
		Files: &Files{
			Contents: map[string][]byte{}, Modes: map[string]fs.FileMode{}, Dirs: map[string]bool{},
			Modified: map[string]time.Time{}, clock: clock,
		},
		Processes: &Processes{Alive: map[int]bool{}, NoAccess: map[int]bool{}, Groups: map[int]int{}},
		Runner:    &Runner{Executables: map[string]string{}, Stdout: map[string][]byte{}, Failures: map[string]error{}},
		Random:    &Random{},
		Disk:      &Disk{},
	}
}

// Port assembles the system.System value over the doubles.
func (host *Host) Port() system.System {
	return system.System{
		Files:       host.Files.port(),
		Clock:       host.Clock.port(),
		Signals:     host.Signals.port(),
		Terminal:    host.Terminal.port(),
		Processes:   host.Processes.port(),
		Runner:      host.Runner.port(),
		Environment: host.Environment.port(),
		Random:      host.Random.port(),
		Disk:        host.Disk.port(),
	}
}

// Clock moves only when a test sets Current or an adapter sleeps. NewTicker
// hands out one buffered tick at the current time and records the stop, so a
// test observes exactly one sample per timer without waiting.
type Clock struct {
	Current time.Time
	Slept   []time.Duration
	Ticks   []chan time.Time
	Stops   int
}

func (clock *Clock) port() system.Clock {
	return system.Clock{
		Now:   func() time.Time { return clock.Current },
		Since: func(start time.Time) time.Duration { return clock.Current.Sub(start) },
		Sleep: func(duration time.Duration) {
			clock.Slept = append(clock.Slept, duration)
			clock.Current = clock.Current.Add(duration)
		},
		After: func(time.Duration) <-chan time.Time {
			channel := make(chan time.Time, 1)
			channel <- clock.Current

			return channel
		},
		NewTicker: func(time.Duration) system.Ticker {
			channel := make(chan time.Time, 1)
			channel <- clock.Current
			clock.Ticks = append(clock.Ticks, channel)

			return system.Ticker{Ticks: channel, Stop: func() { clock.Stops++ }}
		},
	}
}

// Environment is the process environment. Exit records the code and returns, so a
// test observes the exit status without ending the test binary.
type Environment struct {
	Values    map[string]string
	Arguments []string
	Home      string
	Temp      string
	Working   string
	Platform  string
	Input     io.Reader
	Output    bytes.Buffer
	Errors    bytes.Buffer
	Exits     []int
}

func (environment *Environment) port() system.Environment {
	return system.Environment{
		Environ: func() []string {
			entries := make([]string, 0, len(environment.Values))
			for key, value := range environment.Values {
				entries = append(entries, key+"="+value)
			}
			sort.Strings(entries)

			return entries
		},
		Getenv: func(key string) string { return environment.Values[key] },
		LookupEnv: func(key string) (string, bool) {
			value, ok := environment.Values[key]

			return value, ok
		},
		UserHomeDir: func() (string, error) {
			if environment.Home == "" {
				return "", errors.New("systemtest: no home directory configured")
			}

			return environment.Home, nil
		},
		TempDir: func() string { return environment.Temp },
		Getwd:   func() (string, error) { return environment.Working, nil },
		Exit:    func(code int) { environment.Exits = append(environment.Exits, code) },
		GOOS:    environment.Platform,
		Args:    environment.Arguments,
		Stdin:   environment.Input,
		Stdout:  &environment.Output,
		Stderr:  &environment.Errors,
	}
}

// Signals delivers an operating-system signal to every channel registered with Notify.
type Signals struct {
	channels []chan<- os.Signal
	Ignored  []os.Signal
	Restored []os.Signal
	Stopped  int
}

// Deliver sends the signal to every registered channel. The channel must have room.
func (signals *Signals) Deliver(signal os.Signal) {
	for _, channel := range signals.channels {
		channel <- signal
	}
}

func (signals *Signals) port() system.SignalSource {
	return system.SignalSource{
		Notify: func(channel chan<- os.Signal, _ ...os.Signal) {
			signals.channels = append(signals.channels, channel)
		},
		Stop: func(channel chan<- os.Signal) {
			signals.Stopped++
			remaining := signals.channels[:0]
			for _, registered := range signals.channels {
				if registered != channel {
					remaining = append(remaining, registered)
				}
			}
			signals.channels = remaining
		},
		Ignore: func(list ...os.Signal) { signals.Ignored = append(signals.Ignored, list...) },
		Reset:  func(list ...os.Signal) { signals.Restored = append(signals.Restored, list...) },
		Ignored: func(signal os.Signal) bool {
			return slices.Contains(signals.Ignored, signal)
		},
	}
}

// Terminal answers foreground queries from a table keyed by file name, so
// adapter logic is testable without a terminal. A name absent from Groups
// reports no foreground group, as an ioctl on a non-terminal file fails.
type Terminal struct {
	Groups        map[string]int
	ForegroundErr error
	SetErr        error
	Set           []ForegroundSet
}

// ForegroundSet records one placement of group on the named file.
type ForegroundSet struct {
	Name  string
	Group int
}

func (terminal *Terminal) port() system.Terminal {
	return system.Terminal{
		ForegroundGroup: func(file *os.File) (int, error) {
			if terminal.ForegroundErr != nil {
				return 0, terminal.ForegroundErr
			}
			group, ok := terminal.Groups[file.Name()]
			if !ok {
				return 0, errNoForeground
			}

			return group, nil
		},
		SetForegroundGroup: func(file *os.File, group int) error {
			terminal.Set = append(terminal.Set, ForegroundSet{Name: file.Name(), Group: group})

			return terminal.SetErr
		},
	}
}

// errNoForeground is what the double answers for a file with no configured
// foreground group.
var errNoForeground = errors.New("systemtest: the file has no foreground group")

// Files is an in-memory file tree. Contents holds the bytes of each file, Dirs each directory, and Modes the mode of
// each entry. Modified holds the modification time a test or Chtimes sets. An entry without one reads the host
// clock's current time, as an entry written now would. Parents are not checked on create.
//
// Links are not modelled, except that Link copies an entry: the double has no inodes, so a write to one name does not
// show under the other. Symlink and Readlink fail with fs.ErrInvalid, and EvalSymlinks fails for a link entry. SameFile
// compares the name and the Sys value of two entries, the only identity a double carries, so a test replaces an entry
// by giving it another Sys.
type Files struct {
	Contents map[string][]byte
	Modes    map[string]fs.FileMode
	Dirs     map[string]bool
	Modified map[string]time.Time
	clock    *Clock
	temps    int
	// Pipes counts the pipes handed out. PipeErr, when set, fails every pipe
	// instead of opening kernel descriptors.
	Pipes   int
	PipeErr error
	// FlockErr, when set, fails every lock instead of granting it, so a test
	// observes contention without racing another holder.
	FlockErr error
}

func (files *Files) port() system.FileSystem {
	return system.FileSystem{
		ReadFile: func(name string) ([]byte, error) {
			data, ok := files.Contents[name]
			if !ok {
				return nil, pathError("open", name, fs.ErrNotExist)
			}

			return bytes.Clone(data), nil
		},
		WriteFile: func(name string, data []byte, perm fs.FileMode) error {
			files.Contents[name] = bytes.Clone(data)
			files.Modes[name] = perm

			return nil
		},
		ReadDir: func(name string) ([]fs.DirEntry, error) {
			if !files.Dirs[name] {
				return nil, pathError("open", name, fs.ErrNotExist)
			}

			return files.children(name), nil
		},
		Stat:  files.stat,
		Lstat: files.stat,
		Readlink: func(name string) (string, error) {
			return "", pathError("readlink", name, fs.ErrInvalid)
		},
		Mkdir: files.mkdir,
		MkdirAll: func(name string, _ fs.FileMode) error {
			files.Dirs[name] = true

			return nil
		},
		MkdirTemp: func(dir, pattern string) (string, error) {
			files.temps++
			name := path.Join(dir, fmt.Sprintf("%s%d", pattern, files.temps))
			files.Dirs[name] = true

			return name, nil
		},
		Remove:    files.remove,
		RemoveAll: files.removeAll,
		Rename:    files.rename,
		Link:      files.link,
		Symlink:   unsupported("symlink"),
		Chmod:     files.chmod,
		Chtimes: func(name string, _, modified time.Time) error {
			if _, exists := files.info(name); !exists {
				return pathError("chtimes", name, fs.ErrNotExist)
			}
			files.Modified[name] = modified

			return nil
		},
		EvalSymlinks: func(name string) (string, error) {
			info, exists := files.info(name)
			if !exists {
				return "", pathError("lstat", name, fs.ErrNotExist)
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return "", pathError("readlink", name, fs.ErrInvalid)
			}

			return path.Clean(name), nil
		},
		SameFile: func(first, second fs.FileInfo) bool {
			if first == nil || second == nil {
				return false
			}

			return first.Name() == second.Name() && first.Sys() == second.Sys()
		},
		OpenFile:      files.open,
		CreateTemp:    files.createTemp,
		OpenDirectory: files.openDirectory,
		// The double has no other holder of a lock, so every lock is granted
		// unless FlockErr names a failure.
		Flock: func(system.File, int) error { return files.FlockErr },
		Pipe: func() (*os.File, *os.File, error) {
			if files.PipeErr != nil {
				return nil, nil, files.PipeErr
			}
			files.Pipes++
			// Pipe descriptors cross the exec boundary into the launcher
			// child, which no in-memory double can stand in for, so the
			// double opens kernel descriptors and counts them.
			return os.Pipe()
		},
	}
}

func (files *Files) mkdir(name string, _ fs.FileMode) error {
	if _, exists := files.info(name); exists {
		return pathError("mkdir", name, fs.ErrExist)
	}
	files.Dirs[name] = true

	return nil
}

func (files *Files) remove(name string) error {
	if _, exists := files.info(name); !exists {
		return pathError("remove", name, fs.ErrNotExist)
	}
	files.forget(name)

	return nil
}

func (files *Files) removeAll(name string) error {
	for _, key := range files.keys() {
		if key == name || strings.HasPrefix(key, name+"/") {
			files.forget(key)
		}
	}

	return nil
}

func (files *Files) chmod(name string, mode fs.FileMode) error {
	if _, exists := files.info(name); !exists {
		return pathError("chmod", name, fs.ErrNotExist)
	}
	files.Modes[name] = mode

	return nil
}

// open opens name with the os.O_* flags it is given. A missing name is created only under os.O_CREATE, an existing
// one fails with fs.ErrExist under os.O_EXCL, and os.O_TRUNC empties a file. Each handle writes at its own offset.
func (files *Files) open(name string, flag int, perm fs.FileMode) (system.File, error) {
	_, exists := files.info(name)
	switch {
	case exists && flag&os.O_CREATE != 0 && flag&os.O_EXCL != 0:
		return nil, pathError("open", name, fs.ErrExist)
	case files.Dirs[name]:
		return nil, pathError("open", name, syscall.EISDIR)
	case !exists && flag&os.O_CREATE == 0:
		return nil, pathError("open", name, fs.ErrNotExist)
	case !exists:
		files.Contents[name] = []byte{}
		files.Modes[name] = perm
	case flag&os.O_TRUNC != 0:
		files.Contents[name] = []byte{}
	}

	return &fileHandle{files: files, name: name}, nil
}

// createTemp creates a file in dir named by pattern, whose last "*" a counter replaces, as os.CreateTemp replaces it
// with a random string.
func (files *Files) createTemp(dir, pattern string) (system.File, error) {
	files.temps++

	return files.open(path.Join(dir, tempName(pattern, files.temps)), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
}

func tempName(pattern string, counter int) string {
	number := strconv.Itoa(counter)
	index := strings.LastIndex(pattern, "*")
	if index < 0 {
		return pattern + number
	}

	return pattern[:index] + number + pattern[index+1:]
}

// openDirectory opens the directory named by name. A name that is not a directory of the tree is not found.
func (files *Files) openDirectory(name string) (system.Directory, error) {
	if !files.Dirs[name] {
		return nil, pathError("open", name, fs.ErrNotExist)
	}

	return directory{files: files, name: name}, nil
}

// link gives newname a copy of oldname. It fails with fs.ErrNotExist when
// oldname is not a file and with fs.ErrExist when newname is taken.
func (files *Files) link(oldname, newname string) error {
	data, exists := files.Contents[oldname]
	if !exists {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: fs.ErrNotExist}
	}
	if _, taken := files.info(newname); taken {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: fs.ErrExist}
	}
	files.Contents[newname] = bytes.Clone(data)
	files.Modes[newname] = files.Modes[oldname]

	return nil
}

// fileHandle is an open file of the tree. It reads and writes the entry it names, at its own offset.
type fileHandle struct {
	files  *Files
	name   string
	offset int64
}

func (handle *fileHandle) Read(buffer []byte) (int, error) {
	data := handle.files.Contents[handle.name]
	if handle.offset >= int64(len(data)) {
		return 0, io.EOF
	}
	count := copy(buffer, data[handle.offset:])
	handle.offset += int64(count)

	return count, nil
}

func (handle *fileHandle) Write(data []byte) (int, error) {
	content := handle.files.Contents[handle.name]
	end := handle.offset + int64(len(data))
	if end > int64(len(content)) {
		content = append(content, make([]byte, end-int64(len(content)))...)
	}
	copy(content[handle.offset:], data)
	handle.files.Contents[handle.name] = content
	handle.offset = end

	return len(data), nil
}

func (handle *fileHandle) Close() error { return nil }

func (handle *fileHandle) Name() string { return handle.name }

func (handle *fileHandle) Stat() (fs.FileInfo, error) { return handle.files.stat(handle.name) }

func (handle *fileHandle) Sync() error { return nil }

func (handle *fileHandle) Chmod(mode fs.FileMode) error { return handle.files.chmod(handle.name, mode) }

// directory is an open directory of the tree, named by its full path. The names it takes are relative to it.
type directory struct {
	files *Files
	name  string
}

func (dir directory) full(name string) string { return path.Join(dir.name, name) }

func (dir directory) OpenFile(name string, flag int, perm fs.FileMode) (system.File, error) {
	return dir.files.open(dir.full(name), flag, perm)
}

func (dir directory) OpenDirectory(name string) (system.Directory, error) {
	return dir.files.openDirectory(dir.full(name))
}

func (dir directory) Mkdir(name string, perm fs.FileMode) error {
	return dir.files.mkdir(dir.full(name), perm)
}

func (dir directory) Rename(oldname, newname string) error {
	return dir.files.rename(dir.full(oldname), dir.full(newname))
}

func (dir directory) Remove(name string) error { return dir.files.remove(dir.full(name)) }

func (dir directory) Stat(name string) (fs.FileInfo, error) { return dir.files.stat(dir.full(name)) }

// ReadDir lists every entry of the directory, whatever count the caller asks for, as ReadDir(-1) does.
func (dir directory) ReadDir(int) ([]fs.DirEntry, error) { return dir.files.children(dir.name), nil }

func (dir directory) Chmod(mode fs.FileMode) error { return dir.files.chmod(dir.name, mode) }

func (dir directory) Close() error { return nil }

func (files *Files) info(name string) (fileInfo, bool) {
	if data, ok := files.Contents[name]; ok {
		return fileInfo{
			name: path.Base(name), size: int64(len(data)), mode: files.Modes[name], modified: files.modified(name),
		}, true
	}
	if files.Dirs[name] {
		return fileInfo{name: path.Base(name), mode: fs.ModeDir | 0o755, modified: files.modified(name)}, true
	}

	return fileInfo{}, false
}

// modified is the modification time an entry reports: the one a test or Chtimes set, or else the host clock's current
// time.
func (files *Files) modified(name string) time.Time {
	if modified, ok := files.Modified[name]; ok {
		return modified
	}

	return files.clock.Current
}

func (files *Files) stat(name string) (fs.FileInfo, error) {
	info, exists := files.info(name)
	if !exists {
		return nil, pathError("stat", name, fs.ErrNotExist)
	}

	return info, nil
}

func (files *Files) children(name string) []fs.DirEntry {
	var entries []fs.DirEntry
	for _, key := range files.keys() {
		if key != name && path.Dir(key) == name {
			if info, exists := files.info(key); exists {
				entries = append(entries, dirEntry{info: info})
			}
		}
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })

	return entries
}

func (files *Files) keys() []string {
	keys := make([]string, 0, len(files.Contents)+len(files.Dirs))
	for key := range files.Contents {
		keys = append(keys, key)
	}
	for key := range files.Dirs {
		keys = append(keys, key)
	}

	return keys
}

func (files *Files) forget(name string) {
	delete(files.Contents, name)
	delete(files.Dirs, name)
	delete(files.Modes, name)
	delete(files.Modified, name)
}

func (files *Files) rename(oldpath, newpath string) error {
	if _, exists := files.info(oldpath); !exists {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: fs.ErrNotExist}
	}
	if data, ok := files.Contents[oldpath]; ok {
		files.Contents[newpath] = data
	}
	if mode, ok := files.Modes[oldpath]; ok {
		files.Modes[newpath] = mode
	}
	if modified, ok := files.Modified[oldpath]; ok {
		files.Modified[newpath] = modified
	}
	if files.Dirs[oldpath] {
		files.Dirs[newpath] = true
	}
	files.forget(oldpath)

	return nil
}

func unsupported(operation string) func(oldname, newname string) error {
	return func(oldname, newname string) error {
		return &os.LinkError{Op: operation, Old: oldname, New: newname, Err: fs.ErrInvalid}
	}
}

func pathError(operation, name string, err error) error {
	return &fs.PathError{Op: operation, Path: name, Err: err}
}

// fileInfo is the fs.FileInfo of an in-memory entry, with the modification time the tree reports for it.
type fileInfo struct {
	name     string
	size     int64
	mode     fs.FileMode
	modified time.Time
}

func (info fileInfo) Name() string       { return info.name }
func (info fileInfo) Size() int64        { return info.size }
func (info fileInfo) Mode() fs.FileMode  { return info.mode }
func (info fileInfo) ModTime() time.Time { return info.modified }
func (info fileInfo) IsDir() bool        { return info.mode.IsDir() }
func (info fileInfo) Sys() any           { return nil }

// dirEntry is the fs.DirEntry of an in-memory entry.
type dirEntry struct{ info fileInfo }

func (entry dirEntry) Name() string               { return entry.info.Name() }
func (entry dirEntry) IsDir() bool                { return entry.info.IsDir() }
func (entry dirEntry) Type() fs.FileMode          { return entry.info.Mode().Type() }
func (entry dirEntry) Info() (fs.FileInfo, error) { return entry.info, nil }

// Processes reports the liveness and group of each process ID a test registers, and
// records every signal sent to them. Kill fails with ESRCH for an unregistered
// process, and with EPERM for a process in NoAccess, which reads as live under
// another owner.
type Processes struct {
	Pid      int
	Alive    map[int]bool
	NoAccess map[int]bool
	Groups   map[int]int
	Sent     []SentSignal
}

// SentSignal is one call to Kill.
type SentSignal struct {
	Pid    int
	Signal syscall.Signal
}

func (processes *Processes) port() system.ProcessTable {
	return system.ProcessTable{
		Getpid:  func() int { return processes.Pid },
		Getpgrp: func() int { return processes.Groups[processes.Pid] },
		Getpgid: func(pid int) (int, error) {
			if !processes.Alive[pid] {
				return 0, syscall.ESRCH
			}

			return processes.Groups[pid], nil
		},
		Kill: func(pid int, signal syscall.Signal) error {
			processes.Sent = append(processes.Sent, SentSignal{Pid: pid, Signal: signal})
			if processes.NoAccess[pid] {
				return syscall.EPERM
			}
			if !processes.Alive[pid] {
				return syscall.ESRCH
			}

			return nil
		},
	}
}

// Runner resolves executables from a fixed table, reports a fixed own path, and answers
// captured runs from fixed standard output or a fixed failure, keyed by command name. A
// failure wins over output. A command with neither fails as not found. Every capture is
// recorded in Captures in call order. Start and Wait fail: a double runs no process.
type Runner struct {
	Executables map[string]string
	Self        string
	Stdout      map[string][]byte
	Failures    map[string]error
	Captures    []Capture
}

// Capture is one call to CaptureOutput: the command name and its arguments.
type Capture struct {
	Name      string
	Arguments []string
}

func (runner *Runner) capture(_ context.Context, name string, arguments ...string) ([]byte, error) {
	runner.Captures = append(runner.Captures, Capture{Name: name, Arguments: slices.Clone(arguments)})
	if err, failed := runner.Failures[name]; failed {
		return nil, err
	}
	if data, ok := runner.Stdout[name]; ok {
		return bytes.Clone(data), nil
	}

	return nil, &exec.Error{Name: name, Err: exec.ErrNotFound}
}

func (runner *Runner) port() system.ProcessRunner {
	return system.ProcessRunner{
		LookPath: func(file string) (string, error) {
			if found, ok := runner.Executables[file]; ok {
				return found, nil
			}

			return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
		},
		Executable:    func() (string, error) { return runner.Self, nil },
		CaptureOutput: runner.capture,
		Start:         func(*exec.Cmd) error { return errNoProcesses },
		Wait:          func(*exec.Cmd) error { return errNoProcesses },
	}
}

// errNoProcesses is what the double returns for a process it was asked to start or wait for.
var errNoProcesses = errors.New("systemtest: the double does not run processes")

// Random yields the configured bytes, repeated, and fails when none are configured.
type Random struct {
	Bytes []byte
}

func (random *Random) port() system.TokenSource {
	return system.TokenSource{
		Read: func(buffer []byte) (int, error) {
			if len(random.Bytes) == 0 {
				return 0, errors.New("systemtest: no random bytes configured")
			}
			for index := range buffer {
				buffer[index] = random.Bytes[index%len(random.Bytes)]
			}

			return len(buffer), nil
		},
	}
}

// Disk reports a fixed page size and processor count.
type Disk struct {
	PageSize int
	CPUs     int
}

func (disk *Disk) port() system.DiskStats {
	return system.DiskStats{
		PageSize: func() int { return disk.PageSize },
		CPUCount: func() int { return disk.CPUs },
	}
}
