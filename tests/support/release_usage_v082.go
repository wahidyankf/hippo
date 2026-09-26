package support

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// releaseMonitorUsageMistake arranges one release monitor invocation whose
// flags parse but whose values the command cannot use. Every other input is
// valid, so the refusal can only be for the named mistake.
func (driver *Driver) releaseMonitorUsageMistake(mistake string) error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	inputs := map[string]string{
		outputFlag:         filepath.Join(root, "samples.jsonl"),
		summaryFlag:        filepath.Join(root, "summary.json"),
		deploymentRootFlag: root,
		healthURLFlag:      testHealthURL,
		routedOriginFlag:   testRoutedOrigin,
	}
	var extra []string
	switch mistake {
	case "a malformed health URL":
		inputs[healthURLFlag] = "ftp://127.0.0.1/health"
	case "a malformed routed origin":
		inputs[routedOriginFlag] = "http://service.example/path"
	case "no routed origin":
		inputs[routedOriginFlag] = ""
	case "no output path":
		inputs[outputFlag] = ""
	case "no summary path":
		inputs[summaryFlag] = ""
	case "no deployment root":
		inputs[deploymentRootFlag] = ""
	case "a negative duration":
		extra = []string{"--duration-ms", "-1"}
	case "an out-of-range service port":
		extra = []string{"--service-port", "70000"}
	default:
		return fmt.Errorf("unknown release monitor mistake %q", mistake)
	}

	driver.releaseCollector = &sequenceCollector{samples: []policy.Sample{healthySample(time.Unix(0, 0))}}
	driver.releaseArguments = []string{releaseCommandName, monitorCommandName}
	for _, flag := range []string{outputFlag, summaryFlag, deploymentRootFlag, healthURLFlag, routedOriginFlag} {
		driver.releaseArguments = append(driver.releaseArguments, flag, inputs[flag])
	}
	driver.releaseArguments = append(driver.releaseArguments, extra...)

	return nil
}

// requireReleaseUsageMistakeRefused holds the refusal to the caller's view:
// exit 2 naming hippo.args.invalid, and no host sample taken first.
func (driver *Driver) requireReleaseUsageMistakeRefused() error {
	if driver.exitCode != status.CallerError ||
		!strings.Contains(driver.errorOutput, "hippo: ["+string(status.CodeArgsInvalid)+"]") {
		return fmt.Errorf("%q: exit=%d stderr=%q, want exit %d naming %s",
			strings.Join(driver.releaseArguments, " "), driver.exitCode, driver.errorOutput,
			status.CallerError, status.CodeArgsInvalid)
	}
	if driver.releaseCollector != nil && driver.releaseCollector.index != 0 {
		return fmt.Errorf("collected %d samples before refusing the invocation", driver.releaseCollector.index)
	}

	return nil
}
