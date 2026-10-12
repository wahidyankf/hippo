//go:build linux

package support

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/host"
)

// Ordinary hosted Linux regression: cancellation joins the real context reader
// before fixture Close observes ERR on the still-open original held writer.
func TestPublicHostProbeLinuxActualHeldFIFOReaderCancellation(t *testing.T) {
	probe := publicHostProtocolFixture()
	probe.platform, probe.origin = linuxPlatform, time.Now()
	probe.directory = t.TempDir()
	if err := probe.prepare(); err != nil {
		t.Fatal(err)
	}
	cleanup, stop := context.WithTimeout(t.Context(), 3*time.Second)
	defer stop()
	t.Cleanup(func() {
		later, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
		defer cancel()
		if err := probe.Close(later); err != nil {
			t.Error(err)
		}
	})
	reading, err := probe.HoldNextReading()
	if err != nil {
		t.Fatal(err)
	}
	readerContext, cancel := context.WithCancel(t.Context())
	defer cancel()
	type readResult struct {
		data []byte
		err  error
	}
	result := make(chan readResult, 1)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		data, err := host.RootedContextFileReader(probe.directory)(readerContext, "/proc/meminfo")
		result <- readResult{data: data, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		final, cancelFinal := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
		defer cancelFinal()
		select {
		case <-readerDone:
		case <-final.Done():
			t.Error("real context reader not joined during failure cleanup")
		}
	})
	if err := probe.WaitForReading(cleanup, reading); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	select {
	case read := <-result:
		if read.data != nil || !errors.Is(read.err, context.Canceled) {
			t.Fatal("real cancelled reader returned output or lost context", read.err)
		}
	case <-cleanup.Done():
		t.Fatal("real reader did not join before writer proof")
	}
	before := probe.Trace()
	if before.Active != 1 || before.Readings != 1 || before.MaximumActive != 1 {
		t.Fatalf("held writer evidence was lost before Close: %+v", before)
	}
	if err := probe.Close(cleanup); err != nil {
		t.Fatal(err)
	}
	assertActualFIFOReaderRetirement(t, probe.Trace(), reading)
	if _, err := os.Stat(probe.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned actual Linux fixture survived", err)
	}
}

func assertActualFIFOReaderRetirement(t *testing.T, after PublicTUIHostProbeTrace, reading int) {
	t.Helper()
	if !after.Closed || after.Active != 0 || after.Readings != 1 || after.MaximumActive != 1 || len(after.Events) != 2 {
		t.Fatalf("actual Linux original-writer retirement not joined: %+v", after)
	}
	witness := after.Events[1]
	if witness.Phase != "canceled-reader-absent" || witness.Reading != reading || witness.Descriptor < 0 || witness.PollEvents&0x0008 == 0 || witness.PollEvents&0x0020 != 0 {
		t.Fatalf("actual original writer ERR/noNVAL witness absent: %+v", witness)
	}
}
