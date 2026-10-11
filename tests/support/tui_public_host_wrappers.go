package support

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

const (
	publicHostAddressVariable = "PUBLIC_TUI_HOST_PROBE_ADDRESS"
	publicHostNonceVariable   = "PUBLIC_TUI_HOST_PROBE_NONCE"
	publicHostRootVariable    = "PUBLIC_TUI_HOST_PROBE_ROOT"
)

// Only RunIsolated installs this actual suite owner; inherited markers do not.
var publicTUISuiteWrapperOwner *publicTUIHostWrapperOwner

type publicTUIHostWrapperSet struct {
	directory string
	identity  os.FileInfo
	files     map[string]os.FileInfo
	borrowers int
}

type publicTUIHostWrapperOwner struct {
	mu      sync.Mutex
	root    string
	sets    map[[sha256.Size]byte]*publicTUIHostWrapperSet
	retired bool
}

func newPublicTUIHostWrapperOwner(root string) (*publicTUIHostWrapperOwner, error) {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	if err := publicTUIOwnedDirectory(canonical); err != nil {
		return nil, err
	}
	return &publicTUIHostWrapperOwner{root: canonical, sets: map[[sha256.Size]byte]*publicTUIHostWrapperSet{}}, nil
}

func publicHostProbeEnvironmentKey(entry string) bool {
	name, _, _ := strings.Cut(entry, "=")
	return name == linuxEvidenceRootEnvironment || name == publicHostAddressVariable || name == publicHostNonceVariable || name == isolatedRootVariable || name == publicHostRootVariable
}

func publicHostWrapperKey(executable string, scripts map[string]string) ([sha256.Size]byte, error) {
	canonical, err := filepath.EvalSymlinks(executable)
	if err != nil || canonical != executable {
		return [sha256.Size]byte{}, errors.Join(err, errors.New("shared wrapper helper path is not canonical"))
	}
	body, err := os.ReadFile(executable)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	encoded, err := json.Marshal(struct {
		Executable string            `json:"Executable"`
		Helper     [sha256.Size]byte `json:"Helper"`
		Scripts    map[string]string `json:"Scripts"`
	}{Executable: executable, Helper: sha256.Sum256(body), Scripts: scripts})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func (owner *publicTUIHostWrapperOwner) acquire(executable string, scripts map[string]string) (*publicTUIHostWrapperSet, error) {
	key, err := publicHostWrapperKey(executable, scripts)
	if err != nil {
		return nil, err
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.retired {
		return nil, errors.New("shared host wrapper owner has retired")
	}
	if err := publicTUIOwnedDirectory(owner.root); err != nil {
		return nil, err
	}
	set, found := owner.sets[key]
	if !found {
		set, err = owner.create(scripts)
		if err != nil {
			return nil, err
		}
		owner.sets[key] = set
	}
	if err := validatePublicHostWrapperSet(set, scripts); err != nil {
		return nil, err
	}
	set.borrowers++
	return set, nil
}

func (owner *publicTUIHostWrapperOwner) create(scripts map[string]string) (*publicTUIHostWrapperSet, error) {
	directory, err := os.MkdirTemp(owner.root, "host-wrappers-")
	if err != nil {
		return nil, err
	}
	if err := writePublicHostWrappers(directory, scripts); err != nil {
		return nil, errors.Join(err, os.RemoveAll(directory))
	}
	identity, err := os.Lstat(directory)
	if err != nil {
		return nil, errors.Join(err, os.RemoveAll(directory))
	}
	set := &publicTUIHostWrapperSet{directory: directory, identity: identity, files: map[string]os.FileInfo{}}
	for name := range scripts {
		info, err := os.Lstat(filepath.Join(directory, name))
		if err != nil {
			return nil, errors.Join(err, os.RemoveAll(directory))
		}
		set.files[name] = info
	}
	return set, nil
}

func writePublicHostWrappers(directory string, scripts map[string]string) error {
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func validatePublicHostWrapperSet(set *publicTUIHostWrapperSet, scripts map[string]string) error {
	if err := publicTUIOwnedDirectory(set.directory); err != nil {
		return err
	}
	identity, err := os.Lstat(set.directory)
	if err != nil || !os.SameFile(identity, set.identity) {
		return errors.Join(err, errors.New("shared wrapper directory identity changed"))
	}
	for name, expected := range scripts {
		path := filepath.Join(set.directory, name)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o700 || !ok || int64(stat.Uid) != int64(os.Getuid()) || !os.SameFile(info, set.files[name]) {
			return fmt.Errorf("shared wrapper identity or ownership changed: %s", name)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(body) != expected {
			return fmt.Errorf("shared wrapper bytes changed: %s", name)
		}
	}
	return nil
}

func (owner *publicTUIHostWrapperOwner) release(set *publicTUIHostWrapperSet) {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	set.borrowers--
}

func (owner *publicTUIHostWrapperOwner) close() error {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	owner.retired = true
	for _, set := range owner.sets {
		if set.borrowers != 0 {
			return fmt.Errorf("shared host wrappers retain %d unjoined borrowers", set.borrowers)
		}
	}
	for _, set := range owner.sets {
		if err := publicTUIOwnedDirectory(set.directory); err != nil {
			return err
		}
		identity, err := os.Lstat(set.directory)
		if err != nil || !os.SameFile(identity, set.identity) {
			return errors.Join(err, errors.New("refuse retirement of replaced wrapper directory"))
		}
		if err := os.RemoveAll(set.directory); err != nil {
			return err
		}
	}
	clear(owner.sets)
	return nil
}
