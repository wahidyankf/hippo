package system_test

import (
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix" //nolint:depguard // The host assertion must name the same flock constants as the production call.

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

// TestProductionFlockLocksARealFile proves the flock binding grants and
// releases an exclusive lock on an operating-system file.
func TestProductionFlockLocksARealFile(t *testing.T) {
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "locked"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err = system.Production.Files.Flock(file, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("flock exclusive: %v", err)
	}
	if err = system.Production.Files.Flock(file, unix.LOCK_UN); err != nil {
		t.Fatalf("flock unlock: %v", err)
	}
}

// nonOperatingSystemFile is a File no descriptor backs, so the flock binding
// refuses it without touching the kernel.
type nonOperatingSystemFile struct {
	system.File
}

// TestProductionFlockRefusesADoubleWithoutADescriptor proves the flock binding
// locks only an operating-system file.
func TestProductionFlockRefusesADoubleWithoutADescriptor(t *testing.T) {
	if err := system.Production.Files.Flock(nonOperatingSystemFile{}, unix.LOCK_EX); err == nil {
		t.Fatal("flock double without descriptor = nil, want the refusal")
	}
}

// TestProductionOpenDirectoryRefusesAMissingRoot proves the directory binding
// reports a missing root instead of a handle.
func TestProductionOpenDirectoryRefusesAMissingRoot(t *testing.T) {
	if _, err := system.Production.Files.OpenDirectory(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("open missing directory = nil, want the kernel refusal")
	}
}

// TestProductionDirectoryReadsRefuseAMissingEntry proves the directory-relative
// open and stat report a missing entry instead of a handle.
func TestProductionDirectoryReadsRefuseAMissingEntry(t *testing.T) {
	directory, err := system.Production.Files.OpenDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = directory.Close() }()
	if _, err = directory.OpenFile("missing", os.O_RDONLY, 0); err == nil {
		t.Fatal("open missing entry = nil, want the kernel refusal")
	}
	if _, err = directory.Stat("missing"); err == nil {
		t.Fatal("stat missing entry = nil, want the kernel refusal")
	}
}

// TestProductionDirectoryStatDescribesEveryFileKind proves one stat reports
// the name, size, mode, modification time, directory flag, and system value
// for each file kind the status layer distinguishes.
func TestProductionDirectoryStatDescribesEveryFileKind(t *testing.T) {
	root := t.TempDir()
	regular := filepath.Join(root, "regular")
	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("regular", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(root, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	directory, err := system.Production.Files.OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = directory.Close() }()
	for name, kind := range map[string]fs.FileMode{
		"regular":   0,
		"directory": fs.ModeDir,
		"link":      fs.ModeSymlink,
		"pipe":      fs.ModeNamedPipe,
		"socket":    fs.ModeSocket,
	} {
		info, statError := directory.Stat(name)
		if statError != nil {
			t.Fatal(statError)
		}
		if info.Name() != name {
			t.Errorf("stat name = %q, want %q", info.Name(), name)
		}
		if info.IsDir() != (kind == fs.ModeDir) {
			t.Errorf("stat %s is dir = %v, want %v", name, info.IsDir(), kind == fs.ModeDir)
		}
		if info.Mode().Type() != kind {
			t.Errorf("stat %s type = %v, want %v", name, info.Mode().Type(), kind)
		}
		if info.Sys() == nil {
			t.Errorf("stat %s system value = nil, want the raw status", name)
		}
	}
	before := directory // Chmod exercises the handle; Stat confirms the mode changed.
	info, err := before.Stat("regular")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 1 {
		t.Fatalf("stat size = %d, want one byte", info.Size())
	}
	if info.ModTime().IsZero() {
		t.Fatal("stat modification time is zero, want the write time")
	}
}

// TestProductionStatReportsACharacterDevice proves the mode mapping keeps the
// device and character-device flags a character device reports.
func TestProductionStatReportsACharacterDevice(t *testing.T) {
	directory, err := system.Production.Files.OpenDirectory("/dev")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = directory.Close() }()
	info, err := directory.Stat("null")
	if err != nil {
		t.Fatal(err)
	}
	if want := fs.ModeDevice | fs.ModeCharDevice; info.Mode().Type() != want {
		t.Fatalf("null device type = %v, want %v", info.Mode().Type(), want)
	}
}
