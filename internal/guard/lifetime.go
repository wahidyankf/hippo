package guard

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	lifetimeLauncherArgument      = "--hippo-internal-lifetime-launcher"
	lifetimeLauncherEnvironment   = "HIPPO_INTERNAL_LIFETIME_LAUNCHER"
	lifetimeCapabilityEnvironment = "HIPPO_INTERNAL_LIFETIME_CAPABILITY"
	lifetimeHandshakeTimeout      = 5 * time.Second
	lifetimeGroupPollInterval     = 10 * time.Millisecond
)

var errLifetimeHandshakeDeadline = errors.New("lifetime activation handshake exceeded its deadline")

type lifetimeHandshake struct {
	ProcessGroup int    `json:"processGroup,omitempty"`
	Failure      string `json:"failure,omitempty"`
}

type supervisedLifetime struct {
	command      *exec.Cmd
	processGroup int
	exited       chan error
}

// init is the process boundary for the private launcher. It deliberately runs
// before a test binary or the public CLI can interpret the launcher's payload
// arguments. The environment flag and sentinel argument must both match.
//
//nolint:gochecknoinits // A self-exec boundary must precede every possible main, including Go test binaries.
func init() {
	if !lifetimeLauncherAuthorized(os.Args, os.Environ()) {
		return
	}

	os.Exit(runLifetimeLauncher(os.Args[2], os.Args[3], os.Args[4:]))
}

func descriptorIsPipe(descriptor uintptr) bool {
	var info unix.Stat_t
	err := unix.Fstat(int(descriptor), &info)

	return err == nil && info.Mode&unix.S_IFMT == unix.S_IFIFO
}

func lifetimeLauncherAuthorized(arguments, environment []string) bool {
	return lifetimeLauncherInputsAuthorized(
		arguments, environment, descriptorIsPipe(3), descriptorIsPipe(4),
	)
}

func lifetimeLauncherInputsAuthorized(arguments, environment []string, reportPipe, capabilityPipe bool) bool {
	return environmentValue(environment, lifetimeLauncherEnvironment) == "1" &&
		sessionTokenPattern.MatchString(environmentValue(environment, lifetimeCapabilityEnvironment)) &&
		len(arguments) >= 4 && arguments[1] == lifetimeLauncherArgument &&
		reportPipe && capabilityPipe
}

func withoutEnvironment(environment []string, name string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		if len(entry) < len(prefix) || entry[:len(prefix)] != prefix {
			result = append(result, entry)
		}
	}

	return result
}

func reportLifetimeHandshake(report *os.File, handshake lifetimeHandshake) error {
	return json.NewEncoder(report).Encode(handshake)
}

func awaitLifetimeHandshake(ctx context.Context, report io.Reader, timeout time.Duration) (lifetimeHandshake, error) {
	type result struct {
		handshake lifetimeHandshake
		err       error
	}
	decoded := make(chan result, 1)
	go func() {
		var handshake lifetimeHandshake
		err := json.NewDecoder(report).Decode(&handshake)
		decoded <- result{handshake: handshake, err: err}
	}()
	timer := time.NewTimer(max(timeout, time.Millisecond))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return lifetimeHandshake{}, ctx.Err()
	case <-timer.C:
		return lifetimeHandshake{}, errLifetimeHandshakeDeadline
	case outcome := <-decoded:
		return outcome.handshake, outcome.err
	}
}

func startSupervisedLifetime(
	ctx context.Context,
	config RunConfig,
	executable string,
	environment []string,
	identityFiles ...*os.File,
) (*supervisedLifetime, error) {
	launcher, err := os.Executable()
	if err != nil {
		return nil, err
	}

	return startSupervisedLifetimeWithLauncher(
		ctx, config, executable, environment, launcher, lifetimeHandshakeTimeout, identityFiles...,
	)
}

func startSupervisedLifetimeWithLauncher(
	ctx context.Context,
	config RunConfig,
	executable string,
	environment []string,
	launcher string,
	handshakeTimeout time.Duration,
	identityFiles ...*os.File,
) (*supervisedLifetime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reportReader, reportWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reportReader.Close() }()
	capabilityReader, capabilityWriter, err := os.Pipe()
	if err != nil {
		_ = reportWriter.Close()

		return nil, err
	}
	defer func() { _ = capabilityReader.Close() }()
	capability, err := token()
	if err != nil {
		_ = reportWriter.Close()
		_ = capabilityWriter.Close()

		return nil, err
	}
	if _, err = capabilityWriter.WriteString(capability); err != nil {
		_ = reportWriter.Close()
		_ = capabilityWriter.Close()

		return nil, err
	}
	_ = capabilityWriter.Close()
	arguments := make([]string, 0, 3+len(config.Arguments))
	arguments = append(arguments, lifetimeLauncherArgument, config.WorkingDirectory, executable)
	arguments = append(arguments, config.Arguments...)
	command := exec.Command(launcher, arguments...)
	command.Env = withEnvironment(environment, lifetimeLauncherEnvironment, "1")
	command.Env = withEnvironment(command.Env, lifetimeCapabilityEnvironment, capability)
	command.Stdin, command.Stdout, command.Stderr = config.ChildStdin, config.ChildStdout, config.ChildStderr
	command.ExtraFiles = []*os.File{reportWriter, capabilityReader}
	for _, identity := range identityFiles {
		if identity != nil {
			command.ExtraFiles = append(command.ExtraFiles, identity)
		}
	}
	if err = command.Start(); err != nil {
		_ = reportWriter.Close()

		return nil, err
	}
	_ = reportWriter.Close()
	lifetime := &supervisedLifetime{command: command, exited: make(chan error, 1)}
	go func() { lifetime.exited <- command.Wait() }()
	handshake, handshakeError := awaitLifetimeHandshake(ctx, reportReader, handshakeTimeout)
	if handshakeError != nil {
		return lifetime, errors.New("lifetime launcher did not confirm payload activation")
	}
	if handshake.Failure != "" || handshake.ProcessGroup <= 0 {
		return lifetime, errors.New("lifetime launcher could not activate the payload")
	}
	lifetime.processGroup = handshake.ProcessGroup

	return lifetime, nil
}

func processGroupAlive(processGroup int) (bool, error) {
	if processGroup <= 0 {
		return false, errors.New("invalid child process group")
	}
	err := syscall.Kill(-processGroup, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}

	return true, err
}

func waitForProcessGroupRetirement(processGroup int) error {
	for {
		alive, err := processGroupAlive(processGroup)
		if err != nil || !alive {
			return err
		}
		time.Sleep(lifetimeGroupPollInterval)
	}
}

func runLifetimeLauncher(workingDirectory, executable string, arguments []string) int {
	report := os.NewFile(3, "hippo-lifetime-report")
	capabilityReader := os.NewFile(4, "hippo-lifetime-capability")
	if report == nil || capabilityReader == nil {
		return 1
	}
	defer func() { _ = report.Close() }()
	defer func() { _ = capabilityReader.Close() }()
	expectedCapability := os.Getenv(lifetimeCapabilityEnvironment)
	actualCapability, err := io.ReadAll(io.LimitReader(capabilityReader, 33))
	if err != nil || len(actualCapability) != len(expectedCapability) ||
		subtle.ConstantTimeCompare(actualCapability, []byte(expectedCapability)) != 1 {
		_ = reportLifetimeHandshake(report, lifetimeHandshake{Failure: "launcher capability failed"})

		return 1
	}
	_ = capabilityReader.Close()
	// The launcher exclusively owns its report channel and advisory-lock
	// descriptions. None may cross the next exec boundary into arbitrary
	// payload code, where a forged handshake or explicit unlock could corrupt
	// supervision and kernel ownership.
	syscall.CloseOnExec(3)
	syscall.CloseOnExec(5)
	syscall.CloseOnExec(6)

	forwardedSignals := make(chan os.Signal, 2)
	signal.Notify(forwardedSignals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(forwardedSignals)

	command := exec.Command(executable, arguments...) //nolint:gosec // The caller supplies an argv-safe executable and arguments; no shell interpolation occurs.
	command.Dir = workingDirectory
	command.Env = withoutEnvironment(os.Environ(), lifetimeLauncherEnvironment)
	command.Env = withoutEnvironment(command.Env, lifetimeCapabilityEnvironment)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	terminal := prepareTerminalForeground(os.Stdin)
	terminal.configure(command.SysProcAttr)
	if err := command.Start(); err != nil {
		_ = reportLifetimeHandshake(report, lifetimeHandshake{Failure: "payload start failed"})
		_ = terminal.restore()

		return 1
	}
	processGroup := command.Process.Pid
	if err := reportLifetimeHandshake(report, lifetimeHandshake{ProcessGroup: processGroup}); err != nil {
		_ = syscall.Kill(-processGroup, syscall.SIGKILL)
		_ = command.Wait()
		_ = waitForProcessGroupRetirement(processGroup)
		_ = terminal.restore()

		return 1
	}
	_ = report.Close()

	forwardingDone := make(chan struct{})
	go func() {
		defer close(forwardingDone)
		for forwardedSignal := range forwardedSignals {
			signalValue, ok := forwardedSignal.(syscall.Signal)
			if ok {
				_ = syscall.Kill(-processGroup, signalValue)
			}
		}
	}()

	waitError := command.Wait()
	retirementError := waitForProcessGroupRetirement(processGroup)
	signal.Stop(forwardedSignals)
	close(forwardedSignals)
	<-forwardingDone
	restoreError := terminal.restore()
	if retirementError != nil || restoreError != nil {
		return 1
	}

	return waitStatusCode(waitError)
}
