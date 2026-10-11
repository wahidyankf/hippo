package support

import (
	"os"
	"syscall"
	"testing"
)

func TestPublicReadOnlyOutputRequiresOwnedTerminal(t *testing.T) {
	master, slave, err := openNativeTUIPTY()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := master.Close(); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() {
		if err := slave.Close(); err != nil {
			t.Error(err)
		}
	})
	var identity syscall.Stat_t
	if err := syscall.Fstat(int(slave.Fd()), &identity); err != nil {
		t.Fatal(err)
	}
	readonly, err := os.OpenFile(slave.Name(), os.O_RDONLY|syscall.O_NOCTTY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := readonly.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := validatePublicReadOnlyOutput(readonly, identity); err != nil {
		t.Fatal(err)
	}
	if err := validatePublicReadOnlyOutput(slave, identity); err == nil {
		t.Fatal("writable slave was accepted as readonly")
	}
	foreign := identity
	foreign.Ino++
	if err := validatePublicReadOnlyOutput(readonly, foreign); err == nil {
		t.Fatal("foreign inode accepted")
	}
	foreign = identity
	foreign.Uid++
	if err := validatePublicReadOnlyOutput(readonly, foreign); err == nil {
		t.Fatal("foreign UID accepted")
	}
	ordinary, err := os.CreateTemp(t.TempDir(), "ordinary")
	if err != nil {
		t.Fatal(err)
	}
	if err := ordinary.Close(); err != nil {
		t.Fatal(err)
	}
	regular, err := os.Open(ordinary.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := regular.Close(); err != nil {
			t.Error(err)
		}
	})
	var regularIdentity syscall.Stat_t
	if err := syscall.Fstat(int(regular.Fd()), &regularIdentity); err != nil {
		t.Fatal(err)
	}
	if err := validatePublicReadOnlyOutput(regular, regularIdentity); err == nil {
		t.Fatal("regular readonly file was accepted as TTY")
	}
}
