package system

import (
	"context"
	"crypto/rand"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

// FileSystem is the file-system port: whole-file reads and writes, directory and link operations, and the
// metadata calls. Each field carries the signature of the standard-library function it is bound to.
type FileSystem struct {
	ReadFile  func(name string) ([]byte, error)
	WriteFile func(name string, data []byte, perm fs.FileMode) error
	ReadDir   func(name string) ([]fs.DirEntry, error)
	Stat      func(name string) (fs.FileInfo, error)
	Lstat     func(name string) (fs.FileInfo, error)
	Readlink  func(name string) (string, error)
	Mkdir     func(name string, perm fs.FileMode) error
	MkdirAll  func(path string, perm fs.FileMode) error
	MkdirTemp func(dir, pattern string) (string, error)
	Remove    func(name string) error
	RemoveAll func(path string) error
	Rename    func(oldpath, newpath string) error
	Link      func(oldname, newname string) error
	Symlink   func(oldname, newname string) error
	Chmod     func(name string, mode fs.FileMode) error
}

// Clock is the time port. Timers and tickers are not in it yet: their methods need a handle a double can
// control, which the adapter that first needs them adds with its own binding.
type Clock struct {
	Now   func() time.Time
	Since func(start time.Time) time.Duration
	Sleep func(duration time.Duration)
	After func(duration time.Duration) <-chan time.Time
}

// SignalSource is the operating-system signal port. A double delivers signals to the channels it was given.
type SignalSource struct {
	Notify func(channel chan<- os.Signal, signals ...os.Signal)
	Stop   func(channel chan<- os.Signal)
	Ignore func(signals ...os.Signal)
	Reset  func(signals ...os.Signal)
}

// ProcessTable is the process-identity and signal port: the calling process, its process group, and group or
// process signals. Errno classification stays in the adapter that reads the returned error.
type ProcessTable struct {
	Getpid  func() int
	Getpgrp func() int
	Getpgid func(pid int) (int, error)
	Kill    func(pid int, signal syscall.Signal) error
}

// ProcessRunner is the port for locating and identifying executables and for capturing the standard output of a
// command that runs to completion. Starting a child and driving its pipes step by step is still bound by the adapter
// that first needs one, because it needs a handle a double can control.
type ProcessRunner struct {
	LookPath      func(file string) (string, error)
	Executable    func() (string, error)
	CaptureOutput func(ctx context.Context, name string, arguments ...string) ([]byte, error)
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
	},
	Clock: Clock{Now: time.Now, Since: time.Since, Sleep: time.Sleep, After: time.After},
	Signals: SignalSource{
		Notify: signal.Notify, Stop: signal.Stop, Ignore: signal.Ignore, Reset: signal.Reset,
	},
	Processes: ProcessTable{Getpid: os.Getpid, Getpgrp: syscall.Getpgrp, Getpgid: syscall.Getpgid, Kill: syscall.Kill},
	Runner:    ProcessRunner{LookPath: exec.LookPath, Executable: os.Executable, CaptureOutput: captureOutput},
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
