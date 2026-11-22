package cli //nolint:testpackage // ConformanceMain is driven through an injected conformance environment.

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
)

// conformanceOpener is a conformance environment whose Open is the function a test supplies.
type conformanceOpener func(ctx context.Context, path string) (application.ConformanceSession, error)

func (open conformanceOpener) Open(ctx context.Context, path string) (application.ConformanceSession, error) {
	return open(ctx, path)
}

// emptyConformanceSession is a session over a manifest with no consumers and no coordination checks, so every
// phase has nothing to run and the verdict is a pass.
func emptyConformanceSession() application.ConformanceSession {
	return application.ConformanceSession{
		Manifest:     func() application.ConformanceManifest { return application.ConformanceManifest{} },
		Verify:       func(string) error { return nil },
		VerifyBinary: func() error { return nil },
		Close:        func() error { return nil },
	}
}

func TestConformanceMainExitsZeroAndReportsThePassWhenTheRunConforms(t *testing.T) {
	t.Parallel()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	service := application.ConformanceService{Environment: conformanceOpener(
		func(context.Context, string) (application.ConformanceSession, error) {
			return emptyConformanceSession(), nil
		},
	)}

	code := ConformanceMain(context.Background(), []string{"manifest.json"}, stdout, stderr, service)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "four-consumer conformance passed") {
		t.Fatalf("code=%d stdout=%q stderr=%q, want 0 with the pass line and no diagnostic", code, stdout, stderr)
	}
}

func TestConformanceMainExitsOneAndNamesTheFailureWhenTheRunDoesNotConform(t *testing.T) {
	t.Parallel()

	stderr := &bytes.Buffer{}
	service := application.ConformanceService{Environment: conformanceOpener(
		func(context.Context, string) (application.ConformanceSession, error) {
			return application.ConformanceSession{}, errors.New("manifest names no consumer checkout")
		},
	)}

	code := ConformanceMain(context.Background(), []string{"manifest.json"}, &bytes.Buffer{}, stderr, service)
	if code != 1 || !strings.Contains(stderr.String(), "manifest names no consumer checkout") {
		t.Fatalf("code=%d stderr=%q, want 1 naming the failure", code, stderr)
	}
}
