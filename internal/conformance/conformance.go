// Package conformance runs manifest-driven checks without consumer-specific defaults.
package conformance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	manifestSchemaVersion = 1
	commandStopGrace      = 100 * time.Millisecond
	// After SIGKILL the only remaining delay is the operating system retiring
	// and reaping the group, including descendants it reparents first. That
	// latency belongs to the host, not to the grace a cooperating child is
	// given, so it gets its own window.
	commandReapGrace    = 2 * time.Second
	commandGroupPoll    = 5 * time.Millisecond
	reconciliationLimit = 5 * time.Second
	commandCancelled    = "cancelled"
	commandReapTimedOut = "force-stop reap timed out"
)

// Command describes an argv-safe command; no shell interpolation is performed.
type Command struct {
	Arguments []string `json:"arguments"`
}

// Consumer contains one runtime-supplied checkout and its own commands.
type Consumer struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Bootstrap []Command `json:"bootstrap,omitempty"`
	Gates     []Command `json:"gates"`
}

// Check is an additional generic coordination check executed from a named consumer.
type Check struct {
	Consumer          string  `json:"consumer"`
	Command           Command `json:"command"`
	AllowCapacitySkip bool    `json:"allowCapacitySkip,omitempty"`
}

// Manifest is the complete generic four-consumer conformance input.
type Manifest struct {
	SchemaVersion      int        `json:"schemaVersion"`
	HIPPOBinary        string     `json:"hippoBinary"`
	HIPPOSHA256        string     `json:"hippoSha256"`
	SharedRoot         string     `json:"sharedRoot"`
	Consumers          []Consumer `json:"consumers"`
	CoordinationChecks []Check    `json:"coordinationChecks"`
}

type checkoutState struct {
	head  string
	dirty string
}

type checkoutIdentity struct {
	info os.FileInfo
}

type binaryIdentity struct {
	path           string
	executablePath string
	checksum       string
	info           os.FileInfo
	executableInfo os.FileInfo
	temporaryRoot  string
	content        []byte
}

type commandBinaryIdentity struct {
	executablePath string
	checksum       string
	info           os.FileInfo
	temporaryRoot  string
}

type commandError struct {
	category string
	exitCode int
	cause    error
}

type commandRuntime struct {
	start  func(*exec.Cmd) error
	wait   func(*exec.Cmd) error
	signal func(*os.Process, syscall.Signal) error
	alive  func(*os.Process) (bool, error)
}

func defaultCommandRuntime() commandRuntime {
	return commandRuntime{
		start:  func(command *exec.Cmd) error { return command.Start() },
		wait:   func(command *exec.Cmd) error { return command.Wait() },
		signal: signalProcessGroup,
		alive:  processGroupAlive,
	}
}

func (failure *commandError) Error() string {
	if failure.exitCode >= 0 {
		return fmt.Sprintf("command %s with exit %d", failure.category, failure.exitCode)
	}

	return "command " + failure.category
}

func (failure *commandError) Unwrap() error {
	return failure.cause
}

type lockedWriter struct {
	mutex  sync.Mutex
	target io.Writer
}

func consumeManifestJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, keyError := decoder.Token()
			if keyError != nil {
				return keyError
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("conformance object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate conformance manifest field %q", key)
			}
			seen[key] = true
			if valueError := consumeManifestJSON(decoder); valueError != nil {
				return valueError
			}
		}
	case '[':
		for decoder.More() {
			if itemError := consumeManifestJSON(decoder); itemError != nil {
				return itemError
			}
		}
	}
	_, err = decoder.Token()

	return err
}

func (writer *lockedWriter) Write(data []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()

	return writer.target.Write(data)
}

func decodeManifest(data []byte) (Manifest, error) {
	duplicateDecoder := json.NewDecoder(bytes.NewReader(data))
	if err := consumeManifestJSON(duplicateDecoder); err != nil {
		return Manifest{}, err
	}
	if _, err := duplicateDecoder.Token(); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("conformance manifest must contain one JSON value")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("conformance manifest must contain one JSON value")
	}

	return manifest, nil
}

func validateCommand(command Command) error {
	if len(command.Arguments) == 0 || command.Arguments[0] == "" {
		return errors.New("conformance command must contain an executable")
	}

	return nil
}

func validateRawManifest(manifest Manifest) error {
	if manifest.HIPPOBinary == "" || manifest.HIPPOSHA256 == "" || manifest.SharedRoot == "" {
		return errors.New("hippoBinary, hippoSha256, and sharedRoot are required")
	}
	for _, consumer := range manifest.Consumers {
		if consumer.Name == "" || consumer.Path == "" {
			return errors.New("consumer names and paths are required")
		}
	}

	return nil
}

func canonicalPath(base, value string, mustExist bool) (string, error) {
	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return filepath.Clean(canonical), nil
	}
	if mustExist || !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	ancestor, suffix := absolute, []string{}
	for {
		if _, statError := os.Lstat(ancestor); statError == nil {
			break
		} else if !errors.Is(statError, os.ErrNotExist) {
			return "", statError
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", err
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = parent
	}
	ancestor, err = filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	for _, component := range slices.Backward(suffix) {
		ancestor = filepath.Join(ancestor, component)
	}

	return filepath.Clean(ancestor), nil
}

func pathsOverlap(first, second string) bool {
	relative, err := filepath.Rel(first, second)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return true
	}
	relative, err = filepath.Rel(second, first)

	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func canonicalizeManifest(manifest Manifest, base string) (Manifest, error) {
	var err error
	manifest.HIPPOBinary, err = canonicalPath(base, manifest.HIPPOBinary, true)
	if err != nil {
		return Manifest{}, errors.New("HIPPO binary identity is unavailable")
	}
	manifest.SharedRoot, err = canonicalPath(base, manifest.SharedRoot, false)
	if err != nil {
		return Manifest{}, errors.New("shared root identity is unavailable")
	}
	for index := range manifest.Consumers {
		manifest.Consumers[index].Path, err = canonicalPath(base, manifest.Consumers[index].Path, true)
		if err != nil {
			return Manifest{}, fmt.Errorf("consumer %q checkout is unavailable", manifest.Consumers[index].Name)
		}
	}

	return manifest, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("unsupported conformance manifest schema %d", manifest.SchemaVersion)
	}
	if len(manifest.Consumers) != 4 {
		return errors.New("conformance manifest requires exactly four consumers")
	}
	if manifest.HIPPOBinary == "" || manifest.HIPPOSHA256 == "" || manifest.SharedRoot == "" {
		return errors.New("hippoBinary, hippoSha256, and sharedRoot are required")
	}
	names, paths := map[string]bool{}, map[string]bool{}
	for _, consumer := range manifest.Consumers {
		if consumer.Name == "" || consumer.Path == "" || names[consumer.Name] {
			return errors.New("consumer names and paths must be present and unique")
		}
		if paths[consumer.Path] {
			return errors.New("consumer names and paths must be present and unique")
		}
		if pathsOverlap(manifest.SharedRoot, consumer.Path) {
			return errors.New("shared root and consumer checkout scopes must not overlap")
		}
		names[consumer.Name], paths[consumer.Path] = true, true
		info, err := os.Stat(consumer.Path)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("consumer %q checkout is unavailable", consumer.Name)
		}
		if len(consumer.Gates) == 0 {
			return fmt.Errorf("consumer %q requires at least one gate", consumer.Name)
		}
		for _, command := range slices.Concat(consumer.Bootstrap, consumer.Gates) {
			if err = validateCommand(command); err != nil {
				return fmt.Errorf("consumer %q: %w", consumer.Name, err)
			}
		}
	}
	for _, check := range manifest.CoordinationChecks {
		if !names[check.Consumer] {
			return fmt.Errorf("coordination check names unknown consumer %q", check.Consumer)
		}
		if err := validateCommand(check.Command); err != nil {
			return err
		}
	}

	return nil
}

func captureCheckoutIdentities(consumers []Consumer) (map[string]checkoutIdentity, error) {
	identities := make(map[string]checkoutIdentity, len(consumers))
	for _, consumer := range consumers {
		info, err := os.Stat(consumer.Path)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("consumer %q checkout identity is unavailable", consumer.Name)
		}
		identities[consumer.Name] = checkoutIdentity{info: info}
	}

	return identities, nil
}

func verifyCheckoutIdentity(consumer Consumer, identity checkoutIdentity) error {
	info, err := os.Stat(consumer.Path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("consumer %q checkout identity is unavailable", consumer.Name)
	}
	if !os.SameFile(identity.info, info) {
		return fmt.Errorf("consumer %q checkout identity changed", consumer.Name)
	}

	return nil
}

func captureSharedRootIdentity(path string) (checkoutIdentity, error) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return checkoutIdentity{}, errors.New("conformance shared-root identity is unavailable")
	}

	return checkoutIdentity{info: info}, nil
}

func verifySharedRootIdentity(path string, identity checkoutIdentity) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return errors.New("conformance shared-root identity is unavailable")
	}
	if !os.SameFile(identity.info, info) {
		return errors.New("conformance shared-root identity changed")
	}

	return nil
}

func openBinaryObject(path string) (*os.File, os.FileInfo, error) {
	file, err := os.OpenFile(filepath.Clean(path), os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		_ = file.Close()

		return nil, nil, errors.New("binary object must be a regular executable file")
	}

	return file, info, nil
}

func checksumOpenFile(file *os.File) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func captureBinaryIdentity(manifest Manifest) (binaryIdentity, error) {
	decoded, err := hex.DecodeString(manifest.HIPPOSHA256)
	if err != nil || len(decoded) != sha256.Size || strings.ToLower(manifest.HIPPOSHA256) != manifest.HIPPOSHA256 {
		return binaryIdentity{}, errors.New("HIPPO binary checksum must be full lowercase SHA-256")
	}
	file, info, err := openBinaryObject(manifest.HIPPOBinary)
	if err != nil {
		return binaryIdentity{}, errors.New("HIPPO binary identity is unavailable")
	}
	defer func() { _ = file.Close() }()
	temporaryRoot, err := os.MkdirTemp("", "hippo-conformance-bin.")
	if err != nil {
		return binaryIdentity{}, errors.New("HIPPO verified binary storage is unavailable")
	}
	executablePath := filepath.Join(temporaryRoot, "hippo")
	executable, err := os.OpenFile(executablePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		_ = os.RemoveAll(temporaryRoot)

		return binaryIdentity{}, errors.New("HIPPO verified binary storage is unavailable")
	}
	hash := sha256.New()
	var content bytes.Buffer
	_, copyError := io.Copy(io.MultiWriter(executable, hash, &content), file)
	closeError := executable.Close()
	checksum := hex.EncodeToString(hash.Sum(nil))
	if copyError != nil || closeError != nil {
		_ = os.RemoveAll(temporaryRoot)

		return binaryIdentity{}, errors.New("HIPPO binary identity is unavailable")
	}
	if checksum != manifest.HIPPOSHA256 {
		_ = os.RemoveAll(temporaryRoot)

		return binaryIdentity{}, errors.New("HIPPO binary checksum does not match the manifest")
	}
	executableInfo, err := os.Stat(executablePath)
	if err != nil {
		_ = os.RemoveAll(temporaryRoot)

		return binaryIdentity{}, errors.New("HIPPO verified binary storage is unavailable")
	}
	if err = os.Chmod(executablePath, 0o500); err != nil || os.Chmod(temporaryRoot, 0o500) != nil {
		_ = os.Chmod(temporaryRoot, 0o700)
		_ = os.RemoveAll(temporaryRoot)

		return binaryIdentity{}, errors.New("HIPPO verified binary storage is unavailable")
	}

	return binaryIdentity{
		path: manifest.HIPPOBinary, executablePath: executablePath, checksum: checksum, info: info,
		executableInfo: executableInfo, temporaryRoot: temporaryRoot, content: slices.Clone(content.Bytes()),
	}, nil
}

func verifyBinaryObject(path string, expected os.FileInfo, checksum string) error {
	file, info, err := openBinaryObject(path)
	if err != nil || !os.SameFile(expected, info) {
		return errors.New("HIPPO binary identity changed during conformance")
	}
	defer func() { _ = file.Close() }()
	observed, err := checksumOpenFile(file)
	if err != nil || observed != checksum {
		return errors.New("HIPPO binary identity changed during conformance")
	}

	return nil
}

func (identity binaryIdentity) verify() error {
	return errors.Join(
		verifyBinaryObject(identity.path, identity.info, identity.checksum),
		verifyBinaryObject(identity.executablePath, identity.executableInfo, identity.checksum),
	)
}

func (identity binaryIdentity) cleanup() error {
	if identity.temporaryRoot == "" {
		return nil
	}

	return errors.Join(os.Chmod(identity.temporaryRoot, 0o700), os.RemoveAll(identity.temporaryRoot))
}

func (identity binaryIdentity) materializeCommandBinary() (commandBinaryIdentity, error) {
	temporaryRoot, err := os.MkdirTemp("", "hippo-conformance-command.")
	if err != nil {
		return commandBinaryIdentity{}, errors.New("HIPPO verified command binary storage is unavailable")
	}
	executablePath := filepath.Join(temporaryRoot, "hippo")
	if err = os.WriteFile(executablePath, identity.content, 0o700); err != nil {
		_ = os.RemoveAll(temporaryRoot)

		return commandBinaryIdentity{}, errors.New("HIPPO verified command binary storage is unavailable")
	}
	info, err := os.Stat(executablePath)
	if err != nil {
		_ = os.RemoveAll(temporaryRoot)

		return commandBinaryIdentity{}, errors.New("HIPPO verified command binary storage is unavailable")
	}
	if err = os.Chmod(executablePath, 0o500); err != nil || os.Chmod(temporaryRoot, 0o500) != nil {
		_ = os.Chmod(temporaryRoot, 0o700)
		_ = os.RemoveAll(temporaryRoot)

		return commandBinaryIdentity{}, errors.New("HIPPO verified command binary storage is unavailable")
	}

	return commandBinaryIdentity{
		executablePath: executablePath, checksum: identity.checksum, info: info, temporaryRoot: temporaryRoot,
	}, nil
}

func (identity commandBinaryIdentity) verify() error {
	return verifyBinaryObject(identity.executablePath, identity.info, identity.checksum)
}

func (identity commandBinaryIdentity) cleanup() error {
	return errors.Join(os.Chmod(identity.temporaryRoot, 0o700), os.RemoveAll(identity.temporaryRoot))
}

func replaceEnvironment(environment []string, name, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}

	return append(result, prefix+value)
}

func removeEnvironment(environment []string, names ...string) []string {
	removed := make(map[string]bool, len(names))
	for _, name := range names {
		removed[name] = true
	}
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if !found || !removed[name] {
			result = append(result, entry)
		}
	}

	return result
}

func signalProcessGroup(process *os.Process, signal syscall.Signal) error {
	if process == nil {
		return nil
	}
	err := syscall.Kill(-process.Pid, signal)
	// ESRCH means the group is gone, and on Darwin EPERM means its members have
	// exited but have not been reaped yet, so no member is left to signal.
	// Neither answer says the group is still running, and retiring a group that
	// already stopped is not a failure of the run that owned it.
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.EPERM) {
		return nil
	}

	return err
}

func processGroupAlive(process *os.Process) (bool, error) {
	if process == nil {
		return false, nil
	}
	err := syscall.Kill(-process.Pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}

	return true, err
}

func observeCommandGroupRetirement(runtime commandRuntime, process *os.Process, limit time.Duration) (bool, error) {
	if runtime.alive == nil {
		return true, nil
	}
	deadline := time.Now().Add(limit)
	for {
		alive, err := runtime.alive(process)
		if err != nil || !alive {
			return !alive, err
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		time.Sleep(min(commandGroupPoll, time.Until(deadline)))
	}
}

func retireCompletedCommandGroup(runtime commandRuntime, process *os.Process) error {
	if runtime.alive == nil {
		return nil
	}
	alive, err := runtime.alive(process)
	if err != nil {
		return err
	}
	if !alive {
		return nil
	}
	termError := runtime.signal(process, syscall.SIGTERM)
	retired, observeError := observeCommandGroupRetirement(runtime, process, commandStopGrace)
	if retired {
		return errors.Join(termError, observeError)
	}
	killError := runtime.signal(process, syscall.SIGKILL)
	retired, reapError := observeCommandGroupRetirement(runtime, process, commandReapGrace)
	if !retired {
		return errors.Join(termError, observeError, killError, reapError, errors.New("command group retirement timed out"))
	}

	return errors.Join(termError, observeError, killError, reapError)
}

func commandResult(waitError error) error {
	if waitError == nil {
		return nil
	}
	if exitError, ok := errors.AsType[*exec.ExitError](waitError); ok {
		return &commandError{category: "exited", exitCode: exitError.ExitCode()}
	}

	return &commandError{category: "wait failed", exitCode: -1}
}

func cleanCapacitySkip(err error) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		return len(children) == 1 && cleanCapacitySkip(children[0])
	}
	failure, ok := err.(*commandError) //nolint:errorlint // Capacity skip intentionally rejects wrapped or joined errors.

	return ok && failure.category == "exited" && failure.exitCode == 75 && failure.cause == nil
}

func execute(ctx context.Context, directory string, command Command, environment []string, output io.Writer) error {
	return executeWithRuntime(ctx, directory, command, environment, output, defaultCommandRuntime())
}

func executeVerifiedCommand(
	ctx context.Context,
	directory string,
	command Command,
	environment []string,
	output io.Writer,
	identity binaryIdentity,
) error {
	executionIdentity, err := identity.materializeCommandBinary()
	if err != nil {
		return err
	}
	environment = replaceEnvironment(environment, "HIPPO_BIN", executionIdentity.executablePath)
	executeError := execute(ctx, directory, command, environment, output)
	verifyError := executionIdentity.verify()
	cleanupError := executionIdentity.cleanup()
	if cleanupError != nil {
		cleanupError = errors.New("verified HIPPO binary cleanup failed")
	}

	return errors.Join(executeError, verifyError, cleanupError)
}

func executeWithRuntime(ctx context.Context, directory string, command Command, environment []string, output io.Writer, runtime commandRuntime) error {
	if err := ctx.Err(); err != nil {
		return &commandError{category: "cancelled before start", exitCode: -1, cause: err}
	}
	process := exec.Command(command.Arguments[0], command.Arguments[1:]...)
	process.Dir = directory
	process.Env = environment
	process.Stdout, process.Stderr = output, output
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := runtime.start(process); err != nil {
		return &commandError{category: "start failed", exitCode: -1}
	}
	done := make(chan error, 1)
	go func() { done <- runtime.wait(process) }()
	select {
	case err := <-done:
		if retireError := retireCompletedCommandGroup(runtime, process.Process); retireError != nil {
			return &commandError{category: "group retirement failed", exitCode: -1, cause: retireError}
		}

		return commandResult(err)
	case <-ctx.Done():
		termError := runtime.signal(process.Process, syscall.SIGTERM)
		timer := time.NewTimer(commandStopGrace)
		defer timer.Stop()
		select {
		case <-done:
			retired, observeError := observeCommandGroupRetirement(runtime, process.Process, commandStopGrace)
			if retired {
				return &commandError{category: commandCancelled, exitCode: -1, cause: errors.Join(ctx.Err(), termError, observeError)}
			}
			killError := runtime.signal(process.Process, syscall.SIGKILL)
			retired, reapError := observeCommandGroupRetirement(runtime, process.Process, commandReapGrace)
			if !retired {
				return &commandError{category: commandReapTimedOut, exitCode: -1, cause: ctx.Err()}
			}
			if killError != nil {
				return &commandError{category: "force-stop signal failed", exitCode: -1, cause: errors.Join(ctx.Err(), killError)}
			}

			return &commandError{category: commandCancelled, exitCode: -1, cause: errors.Join(ctx.Err(), termError, observeError, reapError)}
		case <-timer.C:
			killError := runtime.signal(process.Process, syscall.SIGKILL)
			reapTimer := time.NewTimer(commandReapGrace)
			defer reapTimer.Stop()
			select {
			case <-done:
				retired, observeError := observeCommandGroupRetirement(runtime, process.Process, commandReapGrace)
				if !retired {
					return &commandError{category: commandReapTimedOut, exitCode: -1, cause: ctx.Err()}
				}
				if killError != nil {
					return &commandError{category: "force-stop signal failed", exitCode: -1, cause: errors.Join(ctx.Err(), killError)}
				}

				return &commandError{category: commandCancelled, exitCode: -1, cause: errors.Join(ctx.Err(), termError, observeError)}
			case <-reapTimer.C:
				return &commandError{category: commandReapTimedOut, exitCode: -1, cause: ctx.Err()}
			}
		}
	}
}

func gitOutput(ctx context.Context, directory string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	output, err := command.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
			return "", fmt.Errorf("git command exited with status %d", exitError.ExitCode())
		}

		return "", errors.New("git command start failed")
	}

	return string(bytes.TrimSpace(output)), nil
}

func snapshot(ctx context.Context, consumer Consumer, identity checkoutIdentity) (checkoutState, error) {
	if err := verifyCheckoutIdentity(consumer, identity); err != nil {
		return checkoutState{}, err
	}
	head, err := gitOutput(ctx, consumer.Path, "rev-parse", "HEAD")
	if err != nil {
		return checkoutState{}, fmt.Errorf("consumer %q HEAD snapshot: %w", consumer.Name, err)
	}
	if err = verifyCheckoutIdentity(consumer, identity); err != nil {
		return checkoutState{}, err
	}
	dirty, err := gitOutput(ctx, consumer.Path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return checkoutState{}, fmt.Errorf("consumer %q dirty snapshot: %w", consumer.Name, err)
	}
	if err = verifyCheckoutIdentity(consumer, identity); err != nil {
		return checkoutState{}, err
	}

	return checkoutState{head: head, dirty: dirty}, nil
}

func consumerByName(manifest Manifest, name string) Consumer {
	for _, consumer := range manifest.Consumers {
		if consumer.Name == name {
			return consumer
		}
	}

	return Consumer{}
}

func reconcileCheckouts(ctx context.Context, consumers []Consumer, identities map[string]checkoutIdentity, states map[string]checkoutState) error {
	var result error
	for _, consumer := range consumers {
		after, err := snapshot(ctx, consumer, identities[consumer.Name])
		if err != nil {
			result = errors.Join(result, err)

			continue
		}
		if after != states[consumer.Name] {
			result = errors.Join(result, fmt.Errorf("consumer %q checkout changed during conformance", consumer.Name))
		}
	}

	return result
}

func executeConsumerPhase(
	ctx context.Context,
	consumers []Consumer,
	commands func(Consumer) []Command,
	phase string,
	environment []string,
	output io.Writer,
	identity binaryIdentity,
	checkoutIdentities map[string]checkoutIdentity,
	sharedRoot string,
	sharedRootIdentity checkoutIdentity,
) error {
	errorsByConsumer := make([]error, len(consumers))
	var group sync.WaitGroup
	for index := range consumers {
		group.Go(func() {
			consumer := consumers[index]
			for _, command := range commands(consumer) {
				if err := errors.Join(
					verifyCheckoutIdentity(consumer, checkoutIdentities[consumer.Name]),
					verifySharedRootIdentity(sharedRoot, sharedRootIdentity),
				); err != nil {
					errorsByConsumer[index] = fmt.Errorf("consumer %q %s failed: %w", consumer.Name, phase, err)

					return
				}
				if err := identity.verify(); err != nil {
					errorsByConsumer[index] = err

					return
				}
				executeError := executeVerifiedCommand(ctx, consumer.Path, command, environment, output, identity)
				checkoutError := verifyCheckoutIdentity(consumer, checkoutIdentities[consumer.Name])
				sharedRootError := verifySharedRootIdentity(sharedRoot, sharedRootIdentity)
				if err := errors.Join(executeError, checkoutError, sharedRootError); err != nil {
					errorsByConsumer[index] = fmt.Errorf("consumer %q %s failed: %w", consumer.Name, phase, err)

					return
				}
			}
		})
	}
	group.Wait()

	return errors.Join(errorsByConsumer...)
}

func executeManifestCommands(
	ctx context.Context,
	manifest Manifest,
	environment []string,
	output io.Writer,
	identity binaryIdentity,
	checkoutIdentities map[string]checkoutIdentity,
	sharedRootIdentity checkoutIdentity,
) error {
	if err := executeConsumerPhase(ctx, manifest.Consumers, func(consumer Consumer) []Command { return consumer.Bootstrap }, "bootstrap", environment, output, identity, checkoutIdentities, manifest.SharedRoot, sharedRootIdentity); err != nil {
		return err
	}
	for _, check := range manifest.CoordinationChecks {
		consumer := consumerByName(manifest, check.Consumer)
		if err := errors.Join(
			verifyCheckoutIdentity(consumer, checkoutIdentities[consumer.Name]),
			verifySharedRootIdentity(manifest.SharedRoot, sharedRootIdentity),
		); err != nil {
			return fmt.Errorf("coordination check for consumer %q failed: %w", consumer.Name, err)
		}
		if err := identity.verify(); err != nil {
			return err
		}
		executeError := executeVerifiedCommand(ctx, consumer.Path, check.Command, environment, output, identity)
		checkoutError := verifyCheckoutIdentity(consumer, checkoutIdentities[consumer.Name])
		sharedRootError := verifySharedRootIdentity(manifest.SharedRoot, sharedRootIdentity)
		if err := errors.Join(executeError, checkoutError, sharedRootError); err != nil {
			if check.AllowCapacitySkip && cleanCapacitySkip(err) {
				_, _ = fmt.Fprintf(output, "coordination check for consumer %q skipped: live host capacity is unsuitable\n", consumer.Name)

				continue
			}

			return fmt.Errorf("coordination check for consumer %q failed: %w", consumer.Name, err)
		}
	}

	return executeConsumerPhase(ctx, manifest.Consumers, func(consumer Consumer) []Command { return consumer.Gates }, "gate", environment, output, identity, checkoutIdentities, manifest.SharedRoot, sharedRootIdentity)
}

// Run validates one manifest, executes consumer gates concurrently, and proves checkout state is unchanged.
func Run(ctx context.Context, manifestPath string, output io.Writer) (returnError error) {
	data, err := os.ReadFile(filepath.Clean(manifestPath))
	if err != nil {
		return errors.New("conformance manifest is unavailable")
	}
	manifest, err := decodeManifest(data)
	if err != nil {
		return err
	}
	if err = validateRawManifest(manifest); err != nil {
		return err
	}
	manifest, err = canonicalizeManifest(manifest, filepath.Dir(filepath.Clean(manifestPath)))
	if err != nil {
		return err
	}
	if err = validateManifest(manifest); err != nil {
		return err
	}
	checkoutIdentities, err := captureCheckoutIdentities(manifest.Consumers)
	if err != nil {
		return err
	}
	identity, err := captureBinaryIdentity(manifest)
	if err != nil {
		return err
	}
	defer func() {
		if cleanupError := identity.cleanup(); cleanupError != nil {
			returnError = errors.Join(returnError, errors.New("verified HIPPO binary cleanup failed"))
		}
	}()
	if err = os.MkdirAll(filepath.Clean(manifest.SharedRoot), 0o700); err != nil {
		return errors.New("conformance shared root is unavailable")
	}
	sharedRootIdentity, err := captureSharedRootIdentity(manifest.SharedRoot)
	if err != nil {
		return err
	}

	environment := removeEnvironment(
		os.Environ(),
		"HIPPO_SESSION", "HIPPO_PROFILE", "HIPPO_CONCURRENCY", "HIPPO_RESERVED_MEMORY_BYTES", "HIPPO_DEFAULT_CONFIG",
	)
	environment = replaceEnvironment(environment, "HIPPO_ROOT", manifest.SharedRoot)
	executionOutput := io.Discard
	if output != nil {
		executionOutput = &lockedWriter{target: output}
	}
	states := make(map[string]checkoutState, len(manifest.Consumers))
	for _, consumer := range manifest.Consumers {
		states[consumer.Name], err = snapshot(ctx, consumer, checkoutIdentities[consumer.Name])
		if err != nil {
			return err
		}
	}
	result := executeManifestCommands(ctx, manifest, environment, executionOutput, identity, checkoutIdentities, sharedRootIdentity)
	cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), reconciliationLimit)
	defer cancelCleanup()
	result = errors.Join(
		result,
		identity.verify(),
		verifySharedRootIdentity(manifest.SharedRoot, sharedRootIdentity),
		reconcileCheckouts(cleanupContext, manifest.Consumers, checkoutIdentities, states), //nolint:contextcheck // Reconciliation deliberately survives caller cancellation.
	)
	if result == nil && output != nil {
		_, result = fmt.Fprintln(output, "four-consumer conformance passed with unchanged checkouts")
	}

	return result
}
