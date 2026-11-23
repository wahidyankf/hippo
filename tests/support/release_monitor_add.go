package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/wahidyankf/hippo/tests/contract"
)

// The release monitor flags the service-port and duration scenarios pass.
const (
	releaseServicePortFlag = "--service-port"
	releaseDurationFlag    = "--duration-ms"
	releaseMonitorDuration = "1000"
)

func (driver *Driver) releaseMonitorAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a service listening on a port the release monitor is asked to count$`, driver.listenForServiceMemory),
		step(`^release monitoring is requested with that port as its service port$`, driver.requestMonitorWithServicePort),
		step(`^the release summary reports the memory of that service as above zero$`, driver.requireServiceMemoryAboveZero),
	}
}

// listenForServiceMemory opens a loopback listener. The monitor measures the
// memory of the process that owns a listening port, so this test process is the
// service it counts. The listener closes when the scenario's paths are released.
func (driver *Driver) listenForServiceMemory() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	driver.stops = append(driver.stops, func() { _ = listener.Close() })
	address, isTCP := listener.Addr().(*net.TCPAddr)
	if !isTCP {
		return fmt.Errorf("listener address %v is not TCP", listener.Addr())
	}
	driver.addCLI.servicePort = address.Port

	return nil
}

// requestMonitorWithServicePort runs the release monitor for a moment with the
// listening port as its service port, and the test endpoints the other release
// scenarios use.
func (driver *Driver) requestMonitorWithServicePort() error {
	if driver.addCLI.servicePort == 0 {
		return errors.New("no listening service port was arranged")
	}
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.releaseSummaryPath = filepath.Join(root, "summary.json")
	driver.releaseArguments = []string{
		releaseCommandName, monitorCommandName,
		outputFlag, filepath.Join(root, "samples.jsonl"),
		summaryFlag, driver.releaseSummaryPath,
		deploymentRootFlag, root,
		healthURLFlag, testHealthURL,
		routedOriginFlag, testRoutedOrigin,
		releaseServicePortFlag, strconv.Itoa(driver.addCLI.servicePort),
		releaseDurationFlag, releaseMonitorDuration,
	}
	driver.runReleaseMonitoring(context.Background())

	return nil
}

// requireServiceMemoryAboveZero holds the written summary to a service memory
// peak above zero, which only a counted listening port can produce.
func (driver *Driver) requireServiceMemoryAboveZero() error {
	if driver.exitCode != 0 {
		return fmt.Errorf("release monitor exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}
	data, err := os.ReadFile(driver.releaseSummaryPath)
	if err != nil {
		return err
	}
	var summary struct {
		ServiceRSSPeakBytes int64 `json:"serviceRssPeakBytes"`
	}
	if err = json.Unmarshal(data, &summary); err != nil {
		return err
	}
	if summary.ServiceRSSPeakBytes <= 0 {
		return fmt.Errorf("serviceRssPeakBytes is %d for listening port %d, want above zero", summary.ServiceRSSPeakBytes, driver.addCLI.servicePort)
	}

	return nil
}
