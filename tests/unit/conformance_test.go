package unit_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/conformance"
	"golang.org/x/sys/unix" //nolint:depguard // Manifest identity fixtures require direct kernel metadata and flock probes.
)

func writeConformanceManifest(t *testing.T, manifest conformance.Manifest) string {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func validConformanceManifest(t *testing.T) conformance.Manifest {
	t.Helper()
	manifest := conformance.Manifest{
		SchemaVersion: 1,
		HIPPOBinary:   os.Args[0],
		HIPPOSHA256:   strings.Repeat("0", 64),
		SharedRoot:    filepath.Join(t.TempDir(), "shared"),
	}
	for index := range 4 {
		path := filepath.Join(t.TempDir(), "checkout")
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		manifest.Consumers = append(manifest.Consumers, conformance.Consumer{
			Name:  string(rune('a' + index)),
			Path:  path,
			Gates: []conformance.Command{{Arguments: []string{"true"}}},
		})
	}

	return manifest
}

func TestConformanceManifestRejectsMalformedAndAmbiguousInputs(t *testing.T) {
	unknownPath := filepath.Join(t.TempDir(), "unknown.json")
	if err := os.WriteFile(unknownPath, []byte(`{"schemaVersion":1,"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := conformance.Run(context.Background(), unknownPath, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown manifest field was accepted")
	}
	duplicatePath := filepath.Join(t.TempDir(), "duplicate.json")
	if err := os.WriteFile(duplicatePath, []byte(`{"schemaVersion":1,"schemaVersion":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := conformance.Run(context.Background(), duplicatePath, &bytes.Buffer{}); err == nil {
		t.Fatal("duplicate manifest field was accepted")
	}

	for name, mutate := range map[string]func(*conformance.Manifest){
		"wrong count": func(manifest *conformance.Manifest) { manifest.Consumers = manifest.Consumers[:3] },
		"duplicate name": func(manifest *conformance.Manifest) {
			manifest.Consumers[1].Name = manifest.Consumers[0].Name
		},
		"duplicate path": func(manifest *conformance.Manifest) {
			manifest.Consumers[1].Path = manifest.Consumers[0].Path
		},
		"missing path": func(manifest *conformance.Manifest) { manifest.Consumers[0].Path = "" },
		"empty gate": func(manifest *conformance.Manifest) {
			manifest.Consumers[0].Gates = []conformance.Command{{}}
		},
		"unknown check consumer": func(manifest *conformance.Manifest) {
			manifest.CoordinationChecks = []conformance.Check{{
				Consumer: "missing", Command: conformance.Command{Arguments: []string{"true"}},
			}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			manifest := validConformanceManifest(t)
			mutate(&manifest)
			if err := conformance.Run(context.Background(), writeConformanceManifest(t, manifest), &bytes.Buffer{}); err == nil {
				t.Fatal("invalid conformance manifest was accepted")
			}
		})
	}
}

func TestConformanceErrorsDoNotExposeCheckoutPaths(t *testing.T) {
	manifest := validConformanceManifest(t)
	privatePath := filepath.Join(t.TempDir(), "private-checkout-name")
	manifest.Consumers[0].Path = privatePath
	err := conformance.Run(context.Background(), writeConformanceManifest(t, manifest), &bytes.Buffer{})
	if err == nil {
		t.Fatal("missing checkout was accepted")
	}
	if strings.Contains(err.Error(), privatePath) {
		t.Fatalf("conformance error exposed a checkout path: %v", err)
	}
}

func runnableConformanceManifest(t *testing.T) conformance.Manifest {
	t.Helper()
	manifest := validConformanceManifest(t)
	data, err := os.ReadFile(manifest.HIPPOBinary)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	manifest.HIPPOSHA256 = hex.EncodeToString(digest[:])
	for _, consumer := range manifest.Consumers {
		for _, arguments := range [][]string{
			{"init", "-q", "-b", "main"},
			{"-c", "user.name=HIPPO fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-q", "-m", "fixture"},
		} {
			command := exec.Command("git", arguments...)
			command.Dir = consumer.Path
			if output, commandError := command.CombinedOutput(); commandError != nil {
				t.Fatalf("initialize conformance checkout: %s: %v", output, commandError)
			}
		}
	}

	return manifest
}

func TestConformanceRejectsUnsafeHIPPOBinaryObjects(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		create func(string) string
	}{
		{name: "directory", create: func(path string) string {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}

			return strings.Repeat("0", 64)
		}},
		{name: "non executable", create: func(path string) string {
			data := []byte("not executable\n")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(data)

			return hex.EncodeToString(digest[:])
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			manifest := runnableConformanceManifest(t)
			privatePath := filepath.Join(t.TempDir(), "private-binary")
			manifest.HIPPOBinary = privatePath
			manifest.HIPPOSHA256 = testCase.create(privatePath)
			err := conformance.Run(context.Background(), writeConformanceManifest(t, manifest), &bytes.Buffer{})
			if err == nil {
				t.Fatal("unsafe HIPPO binary object was accepted")
			}
			if strings.Contains(err.Error(), privatePath) {
				t.Fatalf("binary validation error exposed private path: %v", err)
			}
		})
	}
}

func TestConformanceRejectsHIPPOBinaryFIFOWithoutBlocking(t *testing.T) {
	manifest := runnableConformanceManifest(t)
	privatePath := filepath.Join(t.TempDir(), "private-binary-fifo")
	if err := unix.Mkfifo(privatePath, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest.HIPPOBinary = privatePath
	manifest.HIPPOSHA256 = strings.Repeat("0", 64)
	manifestPath := writeConformanceManifest(t, manifest)
	done := make(chan error, 1)
	go func() { done <- conformance.Run(context.Background(), manifestPath, &bytes.Buffer{}) }()
	select {
	case err := <-done:
		if err == nil || strings.Contains(err.Error(), privatePath) {
			t.Fatalf("FIFO validation was not private and strict: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		writer, err := os.OpenFile(privatePath, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_ = writer.Close()
		<-done
		t.Fatal("FIFO validation blocked")
	}
}

func TestConformanceStartErrorsDoNotExposeCommandPaths(t *testing.T) {
	manifest := runnableConformanceManifest(t)
	privateExecutable := filepath.Join(t.TempDir(), "private-missing-command")
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{privateExecutable}}}
	err := conformance.Run(context.Background(), writeConformanceManifest(t, manifest), &bytes.Buffer{})
	if err == nil {
		t.Fatal("missing absolute command was accepted")
	}
	message := err.Error()
	if !strings.Contains(message, `consumer "a" gate`) || !strings.Contains(message, "start") {
		t.Fatalf("safe error metadata missing: %v", err)
	}
	if strings.Contains(message, privateExecutable) {
		t.Fatalf("command start error exposed private path: %v", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatal("private raw start error remained reachable")
	}
}
