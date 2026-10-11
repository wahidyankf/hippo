package tui_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/tests/support"
)

func TestBDDNativeTeaTransportHelper(t *testing.T) {
	if os.Getenv("HIPPO_TEST_NATIVE_TUI_NONCE") == "" {
		t.Skip("selected only by an owned native transport")
	}
	if os.Getenv("HIPPO_BDD_SUITE") != "integration" || os.Getenv("HIPPO_TEST_ISOLATED_ROOT") == "" {
		t.Fatal("native helper requires inherited integration isolation")
	}
	report := os.NewFile(3, "owned-native-tui-ready")
	if report == nil {
		t.Fatal("owned readiness descriptor unavailable")
	}
	if directory := os.Getenv("HIPPO_TEST_NATIVE_TUI_LIFECYCLE_DIR"); directory != "" {
		os.Exit(runBDDNativeLifecycleTransportHelper(t, report, directory))
	}
	if directory := os.Getenv("HIPPO_TEST_NATIVE_TUI_FAULT_DIR"); directory != "" {
		os.Exit(runBDDNativeFaultTransportHelper(t, report, directory))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := tui.RunBDDNativeTeaHelper(ctx, os.Stdin, os.Stdout, func() error { return support.ReportNativeTUIReady(os.Stdin, report) }); err != nil {
		t.Fatal(err)
	}
}

func runBDDNativeFaultTransportHelper(t *testing.T, report *os.File, directory string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	code, err := tui.RunBDDNativeFaultHelper(ctx, os.Stdin, os.Stdout, os.Stderr, func() error {
		return support.ReportNativeTUIReady(os.Stdin, report)
	}, directory)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func runBDDNativeLifecycleTransportHelper(t *testing.T, report *os.File, directory string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	code, err := tui.RunBDDNativeLifecycleHelper(ctx, os.Stdin, os.Stdout, os.Stderr, func() error { return support.ReportNativeTUIReady(os.Stdin, report) }, directory)
	if err != nil {
		t.Fatal(err)
	}
	return code
}
