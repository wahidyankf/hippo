package cli

import (
	"context"
	"encoding/json"
	"fmt"

	coordination "github.com/wahidyankf/hippo/internal/domain/coordination"

	app "github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

func (application Application) version(options versionOptions) (int, error) {
	if options.jsonOutput {
		encoded, err := json.Marshal(struct {
			SchemaVersion int    `json:"schemaVersion"`
			Version       string `json:"version"`
			Commit        string `json:"commit"`
		}{
			SchemaVersion: 1,
			Version:       application.Version,
			Commit:        application.Commit,
		})
		if err != nil {
			return 1, fmt.Errorf("encode version JSON: %w", err)
		}

		_, err = fmt.Fprintln(application.Stdout, string(encoded))

		return 0, err
	}

	_, err := fmt.Fprintf(application.Stdout, "%s (%s)\n", application.Version, application.Commit)

	return 0, err
}

// runClass reads the --class flag of run: the class it names, or ephemeral for
// an empty one, which is what an unset class has always meant. A class run can
// never accept is the caller's mistake, found before anything else runs. Release
// work is guarded by the release commands, never by run.
func runClass(flag string) (policy.TaskClass, error) {
	if flag == "" {
		return policy.TaskEphemeral, nil
	}
	class, err := policy.ParseTaskClass(flag)
	if err != nil || class == policy.TaskRelease {
		return "", status.Fail(status.CodeArgsInvalid, "class must be ephemeral, service, or transactional")
	}

	return class, nil
}

// runArgumentMistake refuses flag values run can never accept. Each is the
// caller's mistake, so it is found here, before any configuration, host
// evidence, or coordination state is read, and reported as a usage mistake
// rather than as HIPPO failing.
func runArgumentMistake(options runOptions) error {
	if _, known := coordination.DefaultResourceTiers()[options.resourceTier]; options.resourceTier != "" && !known {
		return status.Fail(status.CodeArgsInvalid, "resource tier must be light, standard, or heavy")
	}
	if options.waitForAdmission < 0 {
		return status.Fail(status.CodeArgsInvalid, "--wait-for-admission must not be negative")
	}
	if options.leasePort != 0 {
		if err := coordination.ValidatePortLeaseRequest(
			options.leasePort, options.leaseOwner, options.leaseMinimum, options.leaseMaximum,
		); err != nil {
			return status.Fail(status.CodeArgsInvalid, "--lease-port: %v", err)
		}
	} else if options.leaseOwner != "" || options.leaseMinimum != 0 || options.leaseMaximum != 0 {
		// Without a port there is no lease for these to shape, and ignoring
		// them would let a caller believe a range or owner was enforced.
		return status.Fail(status.CodeArgsInvalid, "--lease-owner, --lease-min, and --lease-max require --lease-port")
	}
	if err := identity.ValidateOverrides(options.source, options.tags); err != nil {
		return status.Fail(status.CodeArgsInvalid, "%v", err)
	}

	return nil
}

func (application Application) run(ctx context.Context, options runOptions) (int, error) {
	return application.RunEntry.Execute(ctx, app.RunRequest{
		Command: options.command, Class: options.class, ConfigPath: options.configPath, RequestedProfile: options.requestedProfile(), WorkingDir: options.workingDir, DiskPath: options.diskPath, LeasePort: options.leasePort, LeaseOwner: options.leaseOwner, LeaseMinimum: options.leaseMinimum, LeaseMaximum: options.leaseMaximum, ReserveCPU: options.reserveCPU, ReserveMemoryMiB: options.reserveMemoryMiB, ResourceTier: options.resourceTier, Source: options.source, Tags: options.tags, ConcurrencyEnvironment: options.concurrencyEnvironment, WaitForAdmission: options.waitForAdmission, ObserveChild: options.observeChild,
	})
}
