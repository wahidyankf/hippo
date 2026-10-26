package support

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"syscall"

	"github.com/wahidyankf/hippo/tests/contract"
)

// PreparePublicRoot explicitly acquires a root registered to this E2E driver.
// Its owning temporary directory is registered before the next fallible effect.
func (driver *Driver) PreparePublicRoot() (string, string, error) {
	if driver.mode != contract.E2E {
		return "", "", errors.New("public root requires the E2E boundary")
	}
	directory, err := driver.temporaryRoot()
	if err != nil {
		return "", "", err
	}
	root := filepath.Join(directory, "public-state")
	if err = os.Mkdir(root, 0o700); err != nil {
		return "", "", err
	}
	return root, directory, nil
}

func publicOwnedDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return errors.New("public directory is not private and owned")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	owner := os.Getuid()
	if !ok || owner < 0 || owner > math.MaxUint32 || stat.Uid != uint32(owner) {
		return errors.New("public directory has a foreign owner")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if resolved != path {
		return errors.New("public directory contains a symlink")
	}
	return nil
}
