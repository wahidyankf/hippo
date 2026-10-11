package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const cliUsage = `Usage: evidence-verifier --host HOST --evidence ABSOLUTE_DIRECTORY --binary ABSOLUTE_BINARY
       tui-sequence.sh --host HOST --evidence ABSOLUTE_DIRECTORY --binary ABSOLUTE_BINARY [recording options]

Hosts: tmux, neovim, wezterm. Verification requires complete recorded evidence.json.
Recording is staged: --prepare; --step ID; --observation FILE; --finalize.
Record actual native input/captures first, then attach visually inspected observations tied to capture hashes.
Native Cmd-B requires --operator-events FILE containing an actual input receipt.
No offline verifier authenticates a consistently forged complete artifact set.
`

type cliOptions struct {
	host, evidence, binary, sourceRoot string
	record, prepare, finalize          bool
	step, observation, operatorEvents  string
	configRoots                        stringFlags
	target                             hostTarget
	fixtures                           string
	expect, inputReceipt               string
	beforeCapture, afterCapture        string
	wait                               time.Duration
}

type stringFlags []string

func (values *stringFlags) String() string         { return fmt.Sprint([]string(*values)) }
func (values *stringFlags) Set(value string) error { *values = append(*values, value); return nil }

func runCLI(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	if slices.Contains(arguments, "--help") || slices.Contains(arguments, "-h") {
		if _, err := io.WriteString(stdout, cliUsage); err != nil {
			return 125
		}
		return 0
	}
	options, err := parseOptions(arguments, stderr)
	if err != nil {
		if _, outputError := fmt.Fprintln(stderr, "host-evidence:", err); outputError != nil {
			return 125
		}
		return 2
	}
	if options.record {
		return runRecording(ctx, options, stdout, stderr)
	}
	document, root, err := openDocument(options.evidence)
	if err != nil {
		if _, outputError := fmt.Fprintln(stderr, "host-evidence: read evidence:", err); outputError != nil {
			return 125
		}
		return 125
	}
	defer func() {
		if closeError := root.Close(); closeError != nil {
			_, outputError := fmt.Fprintln(stderr, "host-evidence: close evidence:", closeError)
			if outputError != nil {
				return
			}
		}
	}()
	current, err := observeCandidate(ctx, options.host, options.binary, document.SourceRoot, document.ConfigRoots)
	if err != nil {
		if _, outputError := fmt.Fprintln(stderr, "host-evidence: inspect candidate:", err); outputError != nil {
			return 125
		}
		return 125
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		if _, outputError := fmt.Fprintln(stderr, "host-evidence: encode evidence:", err); outputError != nil {
			return 125
		}
		return 125
	}
	if err := Verify(bytes.NewReader(encoded), current, root.FS()); err != nil {
		if _, outputError := fmt.Fprintln(stderr, "host-evidence:", err); outputError != nil {
			return 125
		}
		return 1
	}
	if _, err := fmt.Fprintln(stdout, "host evidence verified; actual input/capture provenance and visual inspection remain required"); err != nil {
		return 125
	}
	return 0
}

func parseOptions(arguments []string, stderr io.Writer) (cliOptions, error) {
	options := cliOptions{}
	flags := flag.NewFlagSet("evidence-verifier", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.host, "host", "", "tmux, neovim or wezterm")
	flags.StringVar(&options.evidence, "evidence", "", "absolute evidence directory")
	flags.StringVar(&options.binary, "binary", "", "absolute candidate HIPPO binary")
	flags.StringVar(&options.sourceRoot, "source-root", "", "absolute source checkout")
	flags.BoolVar(&options.record, "record", false, "record actual host sequence")
	flags.BoolVar(&options.prepare, "prepare", false, "bind a fresh task-owned host target")
	flags.BoolVar(&options.finalize, "finalize", false, "verify complete observations")
	flags.StringVar(&options.step, "step", "", "drive one ordered protocol step")
	flags.StringVar(&options.observation, "observation", "", "attach capture-bound visual observation")
	flags.StringVar(&options.operatorEvents, "operator-events", "", "actual native-key event receipt")
	flags.Var(&options.configRoots, "config-root", "actual imported config file or tree; repeatable")
	flags.StringVar(&options.target.Socket, "socket", "", "task-owned tmux socket name")
	flags.StringVar(&options.target.Pane, "pane-id", "", "explicit task pane ID")
	flags.StringVar(&options.target.EditorServer, "editor-server", "", "task-owned Neovim RPC socket")
	flags.StringVar(&options.target.OwnerToken, "owner-token", "", "32-hex token visible in prepared task pane")
	flags.StringVar(&options.fixtures, "fixtures", "", "known two running/two queue fixture identities")
	flags.StringVar(&options.expect, "expect", "", "visible predicate to await before saving native capture")
	flags.StringVar(&options.inputReceipt, "input-receipt", "", "actual externally driven input/failure receipt")
	flags.StringVar(&options.beforeCapture, "before-capture", "", "actual native before-capture for an external input receipt")
	flags.StringVar(&options.afterCapture, "after-capture", "", "actual native after-capture for an external input receipt")
	flags.DurationVar(&options.wait, "wait", 5*time.Second, "bounded native capture wait, at most 30s")
	if err := flags.Parse(arguments); err != nil {
		return options, err
	}
	if flags.NArg() != 0 {
		return options, errors.New("unexpected positional arguments")
	}
	if !slices.Contains([]string{hostTmux, hostNeovim, hostWezterm}, options.host) {
		return options, errors.New("--host must be tmux, neovim, or wezterm")
	}
	if !filepath.IsAbs(options.evidence) || !filepath.IsAbs(options.binary) {
		return options, errors.New("--evidence and --binary must be absolute paths")
	}
	if options.wait <= 0 || options.wait > 30*time.Second {
		return options, errors.New("--wait must be positive and no more than 30s")
	}
	stages := 0
	for _, selected := range []bool{options.prepare, options.step != "", options.observation != "", options.finalize} {
		if selected {
			stages++
		}
	}
	if stages > 1 {
		return options, errors.New("select one recording stage")
	}
	if options.inputReceipt == "" && (options.beforeCapture != "" || options.afterCapture != "") {
		return options, errors.New("external captures require an actual input receipt")
	}
	if options.inputReceipt != "" && options.beforeCapture == "" {
		return options, errors.New("external input receipt requires the actual --before-capture")
	}
	return options, nil
}

func openDocument(directory string) (evidenceDocument, *os.Root, error) {
	var document evidenceDocument
	root, err := os.OpenRoot(directory)
	if err != nil {
		return document, nil, err
	}
	data, err := root.ReadFile("evidence.json")
	if err == nil {
		document, err = decodeEvidence(bytes.NewReader(data))
	}
	if err != nil {
		if closeError := root.Close(); closeError != nil {
			return document, nil, errors.Join(err, closeError)
		}
		return document, nil, err
	}
	return document, root, nil
}
