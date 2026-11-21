package support

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// holdCoordinationLock takes the root's coordination lock through a file
// description of its own, the way another admitting process holds it, and
// keeps it until the scenario ends.
func (driver *Driver) holdCoordinationLock(root string) error {
	lock, err := os.OpenFile(filepath.Join(root, "coordination.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()

		return fmt.Errorf("hold the coordination lock: %w", err)
	}
	driver.stops = append(driver.stops, func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	})

	return nil
}
