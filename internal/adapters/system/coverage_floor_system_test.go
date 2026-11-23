package system_test

import (
	"io/fs"
	"os"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

func TestFloorFileModeBlockDevice(t *testing.T) {
	if _, err := os.Stat("/dev/loop0"); err != nil {
		t.Skipf("no loop block device: %v", err)
	}
	directory, err := system.Production.Files.OpenDirectory("/dev")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = directory.Close() }()
	info, err := directory.Stat("loop0")
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode(); mode&fs.ModeDevice == 0 || mode&fs.ModeCharDevice != 0 {
		t.Fatalf("loop0 mode = %v, want a block device", mode)
	}
}
