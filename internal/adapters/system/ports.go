package system

import (
	"context"
	"crypto/rand"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// FileSystem is the file-system port: whole-file reads and writes, handles, directory and link operations, the
// metadata calls, path resolution, locks, and file identity. Most fields carry the signature of the standard-library
// function they are bound to. OpenFile, CreateTemp, OpenDirectory, and Flock return or take the File and Directory
// handles below, so their production bindings are adapters in this package.
type FileSystem struct {
	ReadFile      func(name string) ([]byte, error)
	WriteFile     func(name string, data []byte, perm fs.FileMode) error
	ReadDir       func(name string) ([]fs.DirEntry, error)
	Stat          func(name string) (fs.FileInfo, error)
	Lstat         func(name string) (fs.FileInfo, error)
	Readlink      func(name string) (string, error)
	Mkdir         func(name string, perm fs.FileMode) error
	MkdirAll      func(path string, perm fs.FileMode) error
	MkdirTemp     func(dir, pattern string) (string, error)
	Remove        func(name string) error
	RemoveAll     func(path string) error
	Rename        func(oldpath, newpath string) error
	Link          func(oldname, newname string) error
	Symlink       func(oldname, newname string) error
	Chmod         func(name string, mode fs.FileMode) error
	Chtimes       func(name string, accessed, modified time.Time) error
	EvalSymlinks  func(path string) (string, error)
	SameFile      func(first, second fs.FileInfo) bool
	OpenFile      func(name string, flag int, perm fs.FileMode) (File, error)
	CreateTemp    func(dir, pattern string) (File, error)
	OpenDirectory func(name string) (Directory, error)
	Flock         func(file File, how int) error
	// Pipe creates the launcher handshake channels. The ends stay *os.File
	// because they cross the exec boundary as inherited descriptors, which no
	// in-memory double can stand in for: the port exists so tests inject a
	// failure and observe the wiring, while kernel behavior stays in
	// integration coverage (R-6).
	Pipe func() (reader, writer *os.File, err error)
}

// File is an open file. The operating-system *os.File satisfies it, and so does a double.
type File interface {
	Read(buffer []byte) (int, error)
	Write(data []byte) (int, error)
	Close() error
	Name() string
	Stat() (fs.FileInfo, error)
	Sync() error
	Chmod(mode fs.FileMode) error
}

// Directory is an open directory. Each name it takes is relative to the directory, and no name follows a link: the
// operations are the directory-relative calls a caller uses to stay inside the directory it opened.
type Directory interface {
	OpenFile(name string, flag int, perm fs.FileMode) (File, error)
	OpenDirectory(name string) (Directory, error)
	Mkdir(name string, perm fs.FileMode) error
	Rename(oldname, newname string) error
	Remove(name string) error
	Stat(name string) (fs.FileInfo, error)
	ReadDir(count int) ([]fs.DirEntry, error)
	Chmod(mode fs.FileMode) error
	Close() error
}

// Ticker is a stoppable sampling timer. Ticks carries the samples and Stop
// releases the timer, so a double can control both ends.
type Ticker struct {
	Ticks <-chan time.Time
	Stop  func()
}

// Clock is the time port. After fires once without a handle; NewTicker returns
// a stoppable timer whose ticks and stop a double controls.
type Clock struct {
	Now       func() time.Time
	Since     func(start time.Time) time.Duration
	Sleep     func(duration time.Duration)
	After     func(duration time.Duration) <-chan time.Time
	NewTicker func(duration time.Duration) Ticker
}

// SignalSource is the operating-system signal port. A double delivers signals to the channels it was given.
type SignalSource struct {
	Notify  func(channel chan<- os.Signal, signals ...os.Signal)
	Stop    func(channel chan<- os.Signal)
	Ignore  func(signals ...os.Signal)
	Reset   func(signals ...os.Signal)
	Ignored func(signal os.Signal) bool
}

// Terminal is the controlling-terminal port: foreground-group queries and
// placement. Only the production binding touches the kernel; a double answers
// from a table so adapter logic is unit-testable while kernel behavior stays in
// integration coverage (R-6).
type Terminal struct {
	ForegroundGroup    func(file *os.File) (int, error)
	SetForegroundGroup func(file *os.File, group int) error
}

// ProcessTable is the process-identity and signal port: the calling process, its process group, and group or
// process signals. Errno classification stays in the adapter that reads the returned error.
type ProcessTable struct {
	Getpid  func() int
	Getpgrp func() int
	Getpgid func(pid int) (int, error)
	Kill    func(pid int, signal syscall.Signal) error
}

// ProcessRunner is the port for locating and identifying executables, for capturing the standard output of a command
// that runs to completion, and for starting and waiting on a command value the caller built. Start and Wait take the
// command as their handle, so a double can answer them. Driving the command's pipes step by step is still bound by the
// adapter that first needs one.
type ProcessRunner struct {
	LookPath      func(file string) (string, error)
	Executable    func() (string, error)
	CaptureOutput func(ctx context.Context, name string, arguments ...string) ([]byte, error)
	Start         func(command *exec.Cmd) error
	Wait          func(command *exec.Cmd) error
}

// Environment is the process environment port: variables, the working and home directories, the platform, the
// standard streams, the arguments, and the process exit.
type Environment struct {
	Environ     func() []string
	Getenv      func(key string) string
	LookupEnv   func(key string) (string, bool)
	UserHomeDir func() (string, error)
	TempDir     func() string
	Getwd       func() (string, error)
	Exit        func(code int)
	GOOS        string
	Args        []string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
}

// TokenSource is the cryptographic random-byte port.
type TokenSource struct {
	Read func(buffer []byte) (int, error)
}

// DiskStats is the host-capacity port for page size and processor count. Filesystem capacity (statfs) is bound
// by the host adapter, because its result type differs between the supported platforms.
type DiskStats struct {
	PageSize func() int
	CPUCount func() int
}

// System groups every port one composition root supplies to the adapters.
type System struct {
	Files       FileSystem
	Clock       Clock
	Signals     SignalSource
	Terminal    Terminal
	Processes   ProcessTable
	Runner      ProcessRunner
	Environment Environment
	Random      TokenSource
	Disk        DiskStats
}

// Production binds every port to the standard library. Each field is a function value or a plain value, so this
// literal holds no executable statement.
var Production = System{
	Files: FileSystem{
		ReadFile: os.ReadFile, WriteFile: os.WriteFile, ReadDir: os.ReadDir, Stat: os.Stat, Lstat: os.Lstat,
		Readlink: os.Readlink, Mkdir: os.Mkdir, MkdirAll: os.MkdirAll, MkdirTemp: os.MkdirTemp, Remove: os.Remove,
		RemoveAll: os.RemoveAll, Rename: os.Rename, Link: os.Link, Symlink: os.Symlink, Chmod: os.Chmod,
		Chtimes: os.Chtimes, EvalSymlinks: filepath.EvalSymlinks, SameFile: os.SameFile,
		OpenFile: openFile, CreateTemp: createTemp, OpenDirectory: openDirectory, Flock: flock, Pipe: os.Pipe,
	},
	Clock: Clock{
		Now: time.Now, Since: time.Since, Sleep: time.Sleep, After: time.After,
		NewTicker: func(duration time.Duration) Ticker {
			ticker := time.NewTicker(duration)
			return Ticker{Ticks: ticker.C, Stop: ticker.Stop}
		},
	},
	Signals: SignalSource{
		Notify: signal.Notify, Stop: signal.Stop, Ignore: signal.Ignore, Reset: signal.Reset,
		Ignored: signal.Ignored,
	},
	Terminal:  Terminal{ForegroundGroup: foregroundGroup, SetForegroundGroup: setForegroundGroup},
	Processes: ProcessTable{Getpid: os.Getpid, Getpgrp: syscall.Getpgrp, Getpgid: syscall.Getpgid, Kill: syscall.Kill},
	Runner: ProcessRunner{
		LookPath: exec.LookPath, Executable: os.Executable, CaptureOutput: captureOutput,
		Start: (*exec.Cmd).Start, Wait: (*exec.Cmd).Wait,
	},
	Environment: Environment{
		Environ: os.Environ, Getenv: os.Getenv, LookupEnv: os.LookupEnv, UserHomeDir: os.UserHomeDir,
		TempDir: os.TempDir, Getwd: os.Getwd, Exit: os.Exit, GOOS: runtime.GOOS, Args: os.Args,
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
	},
	Random: TokenSource{Read: rand.Read},
	Disk:   DiskStats{PageSize: os.Getpagesize, CPUCount: runtime.NumCPU},
}

// captureOutput runs the named command to completion and returns what it wrote to standard output. It is not a
// standard-library function: os/exec returns the output from a method of its command value, so this adapter is the
// one statement the runner port adds.
func captureOutput(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, arguments...).Output()
}
