package systemtest_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

var start = time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)

// NewHost builds the double host the tests share.
func NewHost(start time.Time) *systemtest.Host {
	return systemtest.NewHost(start)
}

func TestHostPortBindsEveryDouble(t *testing.T) {
	port := NewHost(start).Port()
	if port.Files.ReadFile == nil || port.Files.Rename == nil || port.Clock.After == nil ||
		port.Signals.Notify == nil || port.Processes.Kill == nil || port.Runner.LookPath == nil ||
		port.Environment.Stdout == nil || port.Random.Read == nil || port.Disk.CPUCount == nil {
		t.Fatal("a host port left a function field nil")
	}
}

func TestClockMovesOnlyWhenSlept(t *testing.T) {
	host := NewHost(start)
	port := host.Port()
	if !port.Clock.Now().Equal(start) {
		t.Fatalf("now %v, want %v", port.Clock.Now(), start)
	}
	port.Clock.Sleep(3 * time.Second)
	if got := port.Clock.Since(start); got != 3*time.Second || len(host.Clock.Slept) != 1 {
		t.Fatalf("since %v after one sleep of %d", got, len(host.Clock.Slept))
	}
	if fired := <-port.Clock.After(time.Hour); !fired.Equal(start.Add(3 * time.Second)) {
		t.Fatalf("after fired at %v", fired)
	}
}

func TestEnvironmentRecordsStreamsAndExit(t *testing.T) {
	host := NewHost(start)
	host.Environment.Values["HOME"] = "/home/fixture"
	host.Environment.Values["A"] = "1"
	host.Environment.Home = "/home/fixture"
	host.Environment.Arguments = []string{"hippo", "status"}
	host.Environment.Platform = "linux"
	host.Environment.Working = "/work"
	host.Environment.Temp = "/tmp/fixture"
	port := host.Port()

	if got := strings.Join(port.Environment.Environ(), ","); got != "A=1,HOME=/home/fixture" {
		t.Fatalf("environ %q", got)
	}
	if value, ok := port.Environment.LookupEnv("A"); !ok || value != "1" || port.Environment.Getenv("missing") != "" {
		t.Fatalf("lookup %q %t", value, ok)
	}
	if home, err := port.Environment.UserHomeDir(); err != nil || home != "/home/fixture" {
		t.Fatalf("home %q %v", home, err)
	}
	host.Environment.Home = ""
	if _, err := host.Port().Environment.UserHomeDir(); err == nil {
		t.Fatal("empty home directory accepted")
	}
	if port.Environment.TempDir() != "/tmp/fixture" || port.Environment.GOOS != "linux" ||
		len(port.Environment.Args) != 2 {
		t.Fatal("platform, temporary directory, or arguments not bound")
	}
	if dir, err := port.Environment.Getwd(); err != nil || dir != "/work" {
		t.Fatalf("working directory %q %v", dir, err)
	}
	if _, err := port.Environment.Stdout.Write([]byte("out")); err != nil {
		t.Fatal(err)
	}
	if _, err := port.Environment.Stderr.Write([]byte("err")); err != nil {
		t.Fatal(err)
	}
	port.Environment.Exit(3)
	if host.Environment.Output.String() != "out" || host.Environment.Errors.String() != "err" ||
		host.Environment.Exits[0] != 3 {
		t.Fatal("streams or exit not recorded")
	}
	if _, err := port.Environment.Stdin.Read(make([]byte, 1)); err == nil {
		t.Fatal("empty input returned data")
	}
}

func TestSignalsDeliverToRegisteredChannelsOnly(t *testing.T) {
	host := NewHost(start)
	port := host.Port()
	first, second := make(chan os.Signal, 1), make(chan os.Signal, 1)
	port.Signals.Notify(first, os.Interrupt)
	port.Signals.Notify(second, syscall.SIGTERM)
	port.Signals.Stop(first)
	port.Signals.Ignore(syscall.SIGPIPE)
	port.Signals.Reset(syscall.SIGPIPE)
	host.Signals.Deliver(syscall.SIGTERM)
	if got := <-second; got != syscall.SIGTERM || len(first) != 0 {
		t.Fatalf("delivered %v to second, %d to first", got, len(first))
	}
	if host.Signals.Stopped != 1 || len(host.Signals.Ignored) != 1 || len(host.Signals.Restored) != 1 {
		t.Fatal("stop, ignore, or reset not recorded")
	}
}

func TestFilesReadWriteAndStat(t *testing.T) {
	host := NewHost(start)
	port := host.Port()
	if _, err := port.Files.ReadFile("missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing read: %v", err)
	}
	if err := port.Files.WriteFile("dir/a.txt", []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := port.Files.ReadFile("dir/a.txt")
	if err != nil || string(data) != "hello" {
		t.Fatalf("read %q %v", data, err)
	}
	data[0] = 'j'
	if again, _ := port.Files.ReadFile("dir/a.txt"); string(again) != "hello" {
		t.Fatal("read returned the stored slice")
	}
	info, err := port.Files.Stat("dir/a.txt")
	if err != nil || info.Size() != 5 || info.Mode() != 0o600 || info.IsDir() || info.Name() != "a.txt" ||
		!info.ModTime().Equal(start) || info.Sys() != nil {
		t.Fatalf("stat %+v %v", info, err)
	}
	if _, err := port.Files.Lstat("missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing lstat: %v", err)
	}
	if err := port.Files.Chmod("dir/a.txt", 0o644); err != nil || host.Files.Modes["dir/a.txt"] != 0o644 {
		t.Fatalf("chmod: %v", err)
	}
	if err := port.Files.Chmod("missing", 0o644); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing chmod: %v", err)
	}
}

func TestFilesDirectoriesAndNames(t *testing.T) {
	host := NewHost(start)
	port := host.Port()
	if err := port.Files.Mkdir("work", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := port.Files.Mkdir("work", 0o755); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("second mkdir: %v", err)
	}
	if err := port.Files.MkdirAll("work/nested", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := port.Files.WriteFile("work/b.txt", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := port.Files.WriteFile("work/a.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := port.Files.ReadDir("work")
	if err != nil || len(entries) != 3 {
		t.Fatalf("read dir %v %d", err, len(entries))
	}
	if entries[0].Name() != "a.txt" || entries[1].Name() != "b.txt" || entries[2].Name() != "nested" {
		t.Fatalf("entries out of order: %s %s %s", entries[0].Name(), entries[1].Name(), entries[2].Name())
	}
	if !entries[2].IsDir() || entries[2].Type() != fs.ModeDir || entries[0].Type() != 0 {
		t.Fatal("entry type wrong")
	}
	if info, err := entries[0].Info(); err != nil || info.Name() != "a.txt" {
		t.Fatalf("entry info %v", err)
	}
	if _, err := port.Files.ReadDir("missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing read dir: %v", err)
	}
	temp, err := port.Files.MkdirTemp("work", "run-")
	if err != nil || temp != "work/run-1" || !host.Files.Dirs[temp] {
		t.Fatalf("temp %q %v", temp, err)
	}
}

func TestFilesRenameRemoveAndRemoveAll(t *testing.T) {
	host := NewHost(start)
	port := host.Port()
	if err := port.Files.MkdirAll("work/nested", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := port.Files.WriteFile("work/a.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := port.Files.Rename("work/a.txt", "work/c.txt"); err != nil || host.Files.Contents["work/c.txt"] == nil {
		t.Fatalf("rename: %v", err)
	}
	if err := port.Files.Rename("missing", "work/d.txt"); err == nil {
		t.Fatal("rename of a missing file accepted")
	}
	if err := port.Files.Remove("work/c.txt"); err != nil {
		t.Fatal(err)
	}
	if err := port.Files.Remove("work/c.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("second remove: %v", err)
	}
	if err := port.Files.RemoveAll("work"); err != nil {
		t.Fatal(err)
	}
	if len(host.Files.Contents) != 0 || len(host.Files.Dirs) != 0 {
		t.Fatalf("remove all left %d files and %d directories", len(host.Files.Contents), len(host.Files.Dirs))
	}
}

func TestFilesRejectLinksAndReadlink(t *testing.T) {
	port := NewHost(start).Port()
	if _, err := port.Files.Readlink("link"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("readlink: %v", err)
	}
	if err := port.Files.Symlink("a", "b"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("symlink: %v", err)
	}
}

func TestFilesLinkCopiesAnEntryAndRefusesATakenName(t *testing.T) {
	host := NewHost(start)
	port := host.Port()
	if err := port.Files.WriteFile("a", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := port.Files.Link("a", "b"); err != nil || string(host.Files.Contents["b"]) != "x" {
		t.Fatalf("link: %v", err)
	}
	if err := port.Files.Link("a", "b"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("link over a taken name: %v", err)
	}
	if err := port.Files.Link("missing", "c"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("link of a missing file: %v", err)
	}
}

func TestFilesOpenFollowsTheFlagsAndHandlesAddressTheirOwnOffset(t *testing.T) {
	host := NewHost(start)
	port := host.Port()
	if _, err := port.Files.OpenFile("a", os.O_RDONLY, 0); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("open without create: %v", err)
	}
	file, err := port.Files.OpenFile("a", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = port.Files.OpenFile("a", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o640); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("exclusive create over a file: %v", err)
	}
	if _, err = file.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write([]byte("d")); err != nil {
		t.Fatal(err)
	}
	reader, err := port.Files.OpenFile("a", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "abcd" || host.Files.Modes["a"] != 0o640 || file.Name() != "a" {
		t.Fatalf("read %q with %v, mode %o", data, err, host.Files.Modes["a"])
	}
	if info, err := reader.Stat(); err != nil || info.Size() != 4 {
		t.Fatalf("handle stat %+v with %v", info, err)
	}
	if _, err := port.Files.OpenFile("a", os.O_RDWR|os.O_TRUNC, 0); err != nil || len(host.Files.Contents["a"]) != 0 {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := port.Files.CreateTemp("missing-dir", ".tmp-*.x"); err != nil {
		t.Fatalf("temporary: %v", err)
	}
	temp, err := port.Files.CreateTemp("dir", ".history-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if temp.Name() != "dir/.history-2.tmp" {
		t.Fatalf("temporary name %q", temp.Name())
	}
}

func TestDirectoriesResolveNamesInsideTheirOwnTree(t *testing.T) {
	host := NewHost(start)
	host.Files.Dirs["/root"] = true
	host.Files.Dirs["/root/sub"] = true
	port := host.Port()
	if _, err := port.Files.OpenDirectory("/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing directory: %v", err)
	}
	root, err := port.Files.OpenDirectory("/root")
	if err != nil {
		t.Fatal(err)
	}
	if err = root.Mkdir("sub", 0o700); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("mkdir over a directory: %v", err)
	}
	if err = root.Mkdir("new", 0o700); err != nil || !host.Files.Dirs["/root/new"] {
		t.Fatalf("mkdir: %v", err)
	}
	if err = root.Rename("new", "renamed"); err != nil || !host.Files.Dirs["/root/renamed"] {
		t.Fatalf("rename: %v", err)
	}
	if err = root.Remove("renamed"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err = root.Chmod(0o750); err != nil || host.Files.Modes["/root"] != 0o750 {
		t.Fatalf("chmod: %v", err)
	}
	if _, err = root.Stat("sub"); err != nil {
		t.Fatalf("stat: %v", err)
	}
	entries, err := root.ReadDir(-1)
	if err != nil || len(entries) != 1 || entries[0].Name() != "sub" {
		t.Fatalf("entries %v with %v", entries, err)
	}
	if _, err = root.OpenFile("sub", os.O_RDONLY, 0); !errors.Is(err, syscall.EISDIR) {
		t.Fatalf("open of a directory as a file: %v", err)
	}
	if err = root.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestChtimesSetsTheModificationTime(t *testing.T) {
	host := NewHost(start)
	port := host.Port()
	if err := port.Files.Chtimes("missing", start, start); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("chtimes of a missing file: %v", err)
	}
	host.Files.Contents["a"] = []byte("x")
	modified := start.Add(-time.Hour)
	if err := port.Files.Chtimes("a", modified, modified); err != nil {
		t.Fatal(err)
	}
	info, err := port.Files.Stat("a")
	if err != nil || !info.ModTime().Equal(modified) {
		t.Fatalf("modification time of %+v with %v", info, err)
	}
	if err = port.Files.Flock(nil, 0); err != nil {
		t.Fatalf("lock: %v", err)
	}
}

// otherIdentity reports the same entry as the wrapped FileInfo, under a different Sys value.
type otherIdentity struct{ fs.FileInfo }

func (otherIdentity) Sys() any { return "other" }

func TestFilesResolvePathsAndIdentity(t *testing.T) {
	host := NewHost(start)
	host.Files.Dirs["/hippo"] = true
	host.Files.Contents["/hippo/link"] = []byte("/hippo")
	host.Files.Modes["/hippo/link"] = fs.ModeSymlink | 0o777
	port := host.Port()
	if resolved, err := port.Files.EvalSymlinks("/hippo"); err != nil || resolved != "/hippo" {
		t.Fatalf("resolve %q %v", resolved, err)
	}
	if _, err := port.Files.EvalSymlinks("/hippo/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing path: %v", err)
	}
	if _, err := port.Files.EvalSymlinks("/hippo/link"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("link: %v", err)
	}
	first, err := port.Files.Stat("/hippo")
	if err != nil {
		t.Fatal(err)
	}
	again, err := port.Files.Stat("/hippo")
	if err != nil {
		t.Fatal(err)
	}
	if !port.Files.SameFile(first, again) {
		t.Fatal("the same entry was not the same file")
	}
	if port.Files.SameFile(first, otherIdentity{first}) {
		t.Fatal("an entry with another identity was the same file")
	}
	if port.Files.SameFile(first, nil) {
		t.Fatal("a missing file info was the same file")
	}
}

func TestProcessesAnswerLivenessAndRecordSignals(t *testing.T) {
	host := NewHost(start)
	host.Processes.Pid = 42
	host.Processes.Alive[42] = true
	host.Processes.Groups[42] = 40
	port := host.Port()
	if port.Processes.Getpid() != 42 || port.Processes.Getpgrp() != 40 {
		t.Fatal("own identity not reported")
	}
	if group, err := port.Processes.Getpgid(42); err != nil || group != 40 {
		t.Fatalf("group %d %v", group, err)
	}
	if _, err := port.Processes.Getpgid(7); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("missing group: %v", err)
	}
	if err := port.Processes.Kill(42, 0); err != nil {
		t.Fatalf("liveness probe: %v", err)
	}
	if err := port.Processes.Kill(7, syscall.SIGTERM); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("kill of a dead process: %v", err)
	}
	if len(host.Processes.Sent) != 2 || host.Processes.Sent[1].Pid != 7 {
		t.Fatalf("signals recorded %+v", host.Processes.Sent)
	}
}

func TestRunnerResolvesExecutables(t *testing.T) {
	host := NewHost(start)
	host.Runner.Executables["git"] = "/usr/bin/git"
	host.Runner.Self = "/opt/hippo"
	port := host.Port()
	if found, err := port.Runner.LookPath("git"); err != nil || found != "/usr/bin/git" {
		t.Fatalf("lookup %q %v", found, err)
	}
	if _, err := port.Runner.LookPath("absent"); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("absent lookup: %v", err)
	}
	if self, err := port.Runner.Executable(); err != nil || self != "/opt/hippo" {
		t.Fatalf("self %q %v", self, err)
	}
}

func TestRunnerAnswersCapturesAndRecordsEveryCall(t *testing.T) {
	host := NewHost(start)
	host.Runner.Stdout["ps"] = []byte("1024\n")
	host.Runner.Failures["lsof"] = errors.New("lsof failed")
	port := host.Port()
	out, err := port.Runner.CaptureOutput(context.Background(), "ps", "-o", "rss=")
	if err != nil || string(out) != "1024\n" {
		t.Fatalf("capture %q %v", out, err)
	}
	if _, err := port.Runner.CaptureOutput(context.Background(), "lsof", "-t"); err == nil ||
		err.Error() != "lsof failed" {
		t.Fatalf("configured failure not returned: %v", err)
	}
	if _, err := port.Runner.CaptureOutput(context.Background(), "absent"); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("absent capture: %v", err)
	}
	captures := host.Runner.Captures
	if len(captures) != 3 || captures[0].Name != "ps" || captures[2].Name != "absent" ||
		!slices.Equal(captures[0].Arguments, []string{"-o", "rss="}) {
		t.Fatalf("captures %+v", captures)
	}
}

func TestRunnerRefusesToStartOrWaitForProcesses(t *testing.T) {
	port := NewHost(start).Port()
	command := &exec.Cmd{Path: "true"}
	if err := port.Runner.Start(command); err == nil {
		t.Fatal("the double started a process")
	}
	if err := port.Runner.Wait(command); err == nil {
		t.Fatal("the double waited for a process")
	}
}

func TestRandomAndDiskDoubles(t *testing.T) {
	host := NewHost(start)
	port := host.Port()
	if _, err := port.Random.Read(make([]byte, 2)); err == nil {
		t.Fatal("unconfigured random source returned bytes")
	}
	host.Random.Bytes = []byte{1, 2}
	buffer := make([]byte, 5)
	if n, err := host.Port().Random.Read(buffer); err != nil || n != 5 || buffer[4] != 1 || buffer[3] != 2 {
		t.Fatalf("random %v %d %v", buffer, n, err)
	}
	host.Disk.PageSize, host.Disk.CPUs = 4096, 8
	if host.Port().Disk.PageSize() != 4096 || host.Port().Disk.CPUCount() != 8 {
		t.Fatal("disk doubles not bound")
	}
}
