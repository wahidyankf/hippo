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
	Files       *Files
	Processes   *Processes
	Runner      *Runner
	Random      *Random
	Disk        *Disk
}

// NewHost returns a host whose clock reads start, whose environment and file tree are
// empty, and whose input is empty.
func NewHost(start time.Time) *Host {
	return &Host{
		Clock:       &Clock{Current: start},
		Environment: &Environment{Values: map[string]string{}, Input: strings.NewReader("")},
		Signals:     &Signals{},
		Files:       &Files{Contents: map[string][]byte{}, Modes: map[string]fs.FileMode{}, Dirs: map[string]bool{}},
		Processes:   &Processes{Alive: map[int]bool{}, Groups: map[int]int{}},
		Runner:      &Runner{Executables: map[string]string{}, Stdout: map[string][]byte{}, Failures: map[string]error{}},
		Random:      &Random{},
		Disk:        &Disk{},
	}
}

// Port assembles the system.System value over the doubles.
func (host *Host) Port() system.System {
	return system.System{
		Files:       host.Files.port(),
		Clock:       host.Clock.port(),
		Signals:     host.Signals.port(),
		Processes:   host.Processes.port(),
		Runner:      host.Runner.port(),
		Environment: host.Environment.port(),
		Random:      host.Random.port(),
		Disk:        host.Disk.port(),
	}
}

// Clock moves only when a test sets Current or an adapter sleeps.
type Clock struct {
	Current time.Time
	Slept   []time.Duration
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
	}
}

// Files is an in-memory file tree. Parents are not checked on create, and links are
// not modelled: Link, Symlink, and Readlink fail with fs.ErrInvalid, and EvalSymlinks
// fails for a link entry. SameFile compares the name and the Sys value of two entries,
// the only identity a double carries, so a test replaces an entry by giving it another Sys.
type Files struct {
	Contents map[string][]byte
	Modes    map[string]fs.FileMode
	Dirs     map[string]bool
	temps    int
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
		Mkdir: func(name string, _ fs.FileMode) error {
			if _, exists := files.info(name); exists {
				return pathError("mkdir", name, fs.ErrExist)
			}
			files.Dirs[name] = true

			return nil
		},
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
		Remove: func(name string) error {
			if _, exists := files.info(name); !exists {
				return pathError("remove", name, fs.ErrNotExist)
			}
			files.forget(name)

			return nil
		},
		RemoveAll: func(name string) error {
			for _, key := range files.keys() {
				if key == name || strings.HasPrefix(key, name+"/") {
					files.forget(key)
				}
			}

			return nil
		},
		Rename:  files.rename,
		Link:    unsupported("link"),
		Symlink: unsupported("symlink"),
		Chmod: func(name string, mode fs.FileMode) error {
			if _, exists := files.info(name); !exists {
				return pathError("chmod", name, fs.ErrNotExist)
			}
			files.Modes[name] = mode

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
	}
}

func (files *Files) info(name string) (fileInfo, bool) {
	if data, ok := files.Contents[name]; ok {
		return fileInfo{name: path.Base(name), size: int64(len(data)), mode: files.Modes[name]}, true
	}
	if files.Dirs[name] {
		return fileInfo{name: path.Base(name), mode: fs.ModeDir | 0o755}, true
	}

	return fileInfo{}, false
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

// fileInfo is the fs.FileInfo of an in-memory entry; its modification time is the zero time.
type fileInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (info fileInfo) Name() string       { return info.name }
func (info fileInfo) Size() int64        { return info.size }
func (info fileInfo) Mode() fs.FileMode  { return info.mode }
func (info fileInfo) ModTime() time.Time { return time.Time{} }
func (info fileInfo) IsDir() bool        { return info.mode.IsDir() }
func (info fileInfo) Sys() any           { return nil }

// dirEntry is the fs.DirEntry of an in-memory entry.
type dirEntry struct{ info fileInfo }

func (entry dirEntry) Name() string               { return entry.info.Name() }
func (entry dirEntry) IsDir() bool                { return entry.info.IsDir() }
func (entry dirEntry) Type() fs.FileMode          { return entry.info.Mode().Type() }
func (entry dirEntry) Info() (fs.FileInfo, error) { return entry.info, nil }

// Processes reports the liveness and group of each process ID a test registers, and
// records every signal sent to them. Kill fails with ESRCH for an unregistered process.
type Processes struct {
	Pid    int
	Alive  map[int]bool
	Groups map[int]int
	Sent   []SentSignal
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
