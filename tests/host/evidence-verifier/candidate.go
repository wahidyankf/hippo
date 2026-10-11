package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func observeCandidate(ctx context.Context, host, binary, sourceRoot string, configRoots []string) (Candidate, error) {
	current := Candidate{Host: host, Now: time.Now().UTC(), DirtyPaths: map[string]string{}, ConfigPaths: map[string]string{}}
	if !filepath.IsAbs(sourceRoot) || len(configRoots) == 0 {
		return current, errors.New("source checkout and actual imported configuration roots are required")
	}
	revision, err := runCommand(ctx, sourceRoot, "git", "rev-parse", "HEAD")
	if err != nil {
		return current, err
	}
	current.Revision = strings.TrimSpace(string(revision))
	if !hexadecimal(current.Revision, 40) {
		return current, errors.New("source revision is not a full commit")
	}
	changed, err := runCommand(ctx, sourceRoot, "git", "diff", "--name-only", "-z", "HEAD", "--")
	if err != nil {
		return current, err
	}
	untracked, err := runCommand(ctx, sourceRoot, "git", "ls-files", "--others", "--exclude-standard", "-z", "--")
	if err != nil {
		return current, err
	}
	for name := range strings.SplitSeq(string(append(changed, untracked...)), "\x00") {
		if name == "" {
			continue
		}
		if !fs.ValidPath(filepath.ToSlash(name)) {
			return current, errors.New("invalid source path from Git")
		}
		digest, err := sourceFingerprint(filepath.Join(sourceRoot, name))
		if err != nil {
			return current, err
		}
		current.DirtyPaths[filepath.ToSlash(name)] = digest
	}
	digest, err := fileFingerprint(binary)
	if err != nil {
		return current, fmt.Errorf("digest candidate binary: %w", err)
	}
	current.BinarySHA256 = digest
	encoded, err := runCommand(ctx, sourceRoot, binary, "version", "--json")
	if err != nil {
		return current, fmt.Errorf("read embedded binary identity: %w", err)
	}
	var identity struct {
		SchemaVersion int    `json:"schemaVersion"`
		Version       string `json:"version"`
		Commit        string `json:"commit"`
	}
	if err := json.Unmarshal(encoded, &identity); err != nil {
		return current, fmt.Errorf("decode embedded binary identity: %w", err)
	}
	if identity.SchemaVersion != 1 || identity.Version == "" || !hexadecimal(identity.Commit, 40) {
		return current, errors.New("binary is missing its full embedded identity")
	}
	current.BinaryVersion, current.BinaryCommit = identity.Version, identity.Commit
	for _, root := range configRoots {
		if !filepath.IsAbs(root) {
			return current, errors.New("imported configuration roots must be absolute")
		}
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			return current, fmt.Errorf("resolve configuration root: %w", err)
		}
		if err := fingerprintConfig(resolved, current.ConfigPaths); err != nil {
			return current, err
		}
	}
	if len(current.ConfigPaths) == 0 {
		return current, errors.New("actual imported configuration inventory is empty")
	}
	return current, nil
}

func runCommand(ctx context.Context, directory, binary string, arguments ...string) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(bounded, binary, arguments...)
	command.Dir = directory
	command.Env = cleanGitEnvironment()
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", filepath.Base(binary), err)
	}
	return output, nil
}

func cleanGitEnvironment() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			environment = append(environment, value)
		}
	}
	return environment
}

func fileFingerprint(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, readError := io.Copy(hash, file)
	if closeError := file.Close(); readError != nil || closeError != nil {
		return "", errors.Join(readError, closeError)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sourceFingerprint(name string) (string, error) {
	info, err := os.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		sum := sha256.Sum256([]byte("deleted source path"))
		return hex.EncodeToString(sum[:]), nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(name)
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256([]byte("symlink:" + target))
		return hex.EncodeToString(sum[:]), nil
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("dirty source path is not a regular file or symlink")
	}
	return fileFingerprint(name)
}

func fingerprintConfig(root string, fingerprints map[string]string) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return errors.New("imported configuration symlink needs its own resolved --config-root")
		}
		if !entry.Type().IsRegular() {
			return errors.New("imported configuration is not a regular file")
		}
		digest, err := fileFingerprint(name)
		if err != nil {
			return err
		}
		fingerprints[name] = digest
		return nil
	})
}
