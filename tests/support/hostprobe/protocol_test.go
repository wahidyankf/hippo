package hostprobe_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/support/hostprobe"
)

func TestCommandIndexRequiresExactProductionQuery(t *testing.T) {
	for index, command := range hostprobe.Commands() {
		request := hostprobe.Request{Name: command[0], Arguments: slices.Clone(command[1:])}
		if hostprobe.CommandIndex(request) != index {
			t.Fatalf("query %v lost its index", command)
		}
		request.Arguments = append(request.Arguments, "foreign")
		if hostprobe.CommandIndex(request) != -1 {
			t.Fatal("extra query argument was accepted")
		}
	}
	if hostprobe.CommandIndex(hostprobe.Request{Name: "foreign"}) != -1 {
		t.Fatal("foreign query was accepted")
	}
	clone := hostprobe.Commands()
	clone[0][0] = "foreign"
	if hostprobe.Commands()[0][0] != "ps" {
		t.Fatal("caller changed shared whitelist")
	}
}

func TestClientRefusesUnownedInputsBeforeDial(t *testing.T) {
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	for _, test := range []struct {
		name, marker, address, nonce string
		args                         []string
	}{
		{"marker", "", "127.0.0.1:1", "owned", []string{"helper", "--", "ps", "-A", "-o", "%cpu="}},
		{"foreign-address", "1", "192.0.2.1:1", "owned", []string{"helper", "--", "ps", "-A", "-o", "%cpu="}},
		{"missing-argv", "1", "127.0.0.1:1", "owned", []string{"helper"}},
		{"missing-nonce", "1", "127.0.0.1:1", "", []string{"helper", "--", "ps", "-A", "-o", "%cpu="}},
		{"foreign-query", "1", "127.0.0.1:1", "owned", []string{"helper", "--", "ps", "foreign"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(hostprobe.HelperEnvironment, test.marker)
			t.Setenv("PUBLIC_TUI_HOST_PROBE_ADDRESS", test.address)
			t.Setenv("PUBLIC_TUI_HOST_PROBE_NONCE", test.nonce)
			os.Args = test.args
			err := hostprobe.Run(context.Background())
			if err == nil || strings.Contains(err.Error(), "connect:") {
				t.Fatalf("invalid input reached dial: %v", err)
			}
		})
	}
}

func TestClientCanceledDialRetainsContextFailure(t *testing.T) {
	t.Setenv(hostprobe.HelperEnvironment, "1")
	t.Setenv("PUBLIC_TUI_HOST_PROBE_ADDRESS", "127.0.0.1:1")
	t.Setenv("PUBLIC_TUI_HOST_PROBE_NONCE", "owned")
	original := os.Args
	os.Args = []string{"helper", "--", "ps", "-A", "-o", "%cpu="}
	t.Cleanup(func() { os.Args = original })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := hostprobe.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled dial: %v", err)
	}
}

type refusingOutput struct{}

func (refusingOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestClientRealConnectionRequiresOutputAckAndCommit(t *testing.T) {
	for _, variant := range []string{"committed", "malformed", "refused", "wrong-commit", "missing-commit", "write-failure"} {
		t.Run(variant, func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}

			serverDone := make(chan error, 1)
			serverJoined := make(chan struct{})
			t.Cleanup(func() {
				closeError := listener.Close()
				if closeError != nil && !errors.Is(closeError, net.ErrClosed) {
					t.Error(closeError)
				}
				select {
				case <-serverJoined:
				case <-time.After(3 * time.Second):
					t.Error("owned protocol server did not join")
				}
			})
			request := hostprobe.Request{Nonce: "owned", Executable: "/owned/helper", PID: 17, ParentPID: 23, Name: "ps", Arguments: []string{"-A", "-o", "%cpu="}}
			go func() {
				defer close(serverJoined)
				connection, err := listener.Accept()
				if err != nil {
					serverDone <- err
					return
				}
				serveError := serveProtocolVariant(connection, request, variant)
				serverDone <- errors.Join(serveError, connection.Close())
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var dialer net.Dialer
			connection, err := dialer.DialContext(ctx, "tcp4", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			runProtocolVariantClient(ctx, t, connection, request, variant, serverDone, serverJoined)
		})
	}
}

type closeFailureConnection struct {
	net.Conn

	failure error
}

func (connection closeFailureConnection) Close() error {
	return errors.Join(connection.Conn.Close(), connection.failure)
}

func TestClientRetainsProtocolAndCloseFailure(t *testing.T) {
	client, server := net.Pipe()
	if err := server.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		var request hostprobe.Request
		err := json.NewDecoder(server).Decode(&request)
		if err == nil {
			err = json.NewEncoder(server).Encode(hostprobe.Response{Error: "protocol refusal"})
		}
		done <- errors.Join(err, server.Close())
	}()
	closeError := errors.New("actual connection close failure")
	result := hostprobe.ExchangeAndCloseForTest(closeFailureConnection{Conn: client, failure: closeError}, hostprobe.Request{Name: "ps"}, io.Discard)
	if !errors.Is(result, closeError) || !strings.Contains(result.Error(), "protocol refusal") {
		t.Fatalf("joined failures lost: %v", result)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("protocol reader did not join")
	}
	<-joined
}

func serveProtocolVariant(connection net.Conn, request hostprobe.Request, variant string) error {
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	decoder := json.NewDecoder(connection)
	var observed hostprobe.Request
	if err := decoder.Decode(&observed); err != nil {
		return err
	}
	if observed.Nonce != request.Nonce || observed.Executable != request.Executable || observed.PID != request.PID || observed.ParentPID != request.ParentPID || hostprobe.CommandIndex(observed) != 0 {
		return errors.New("wire identity changed")
	}
	if variant == "malformed" {
		_, err := io.WriteString(connection, "not-json\n")
		return err
	}
	encoder := json.NewEncoder(connection)
	response := hostprobe.Response{Output: "240.0\n"}
	if variant == "refused" {
		response.Error = "foreign nonce"
	}
	if err := encoder.Encode(response); err != nil {
		return err
	}
	if variant == "refused" {
		return nil
	}
	var ack string
	err := decoder.Decode(&ack)
	if variant == "write-failure" {
		if !errors.Is(err, io.EOF) {
			return errors.New("failed stdout emitted an ACK")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if ack != "output-written" {
		return errors.New("actual output ACK missing")
	}
	if variant == "missing-commit" {
		return nil
	}
	commit := "committed"
	if variant == "wrong-commit" {
		commit = "foreign"
	}
	return encoder.Encode(commit)
}

func runProtocolVariantClient(ctx context.Context, t *testing.T, connection net.Conn, request hostprobe.Request, variant string, serverDone <-chan error, serverJoined <-chan struct{}) {
	t.Helper()
	var output bytes.Buffer
	var writer io.Writer = &output
	if variant == "write-failure" {
		writer = refusingOutput{}
	}
	clientError := hostprobe.ExchangeAndCloseForTest(connection, request, writer)
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("server did not join", ctx.Err())
	}
	<-serverJoined
	if variant == "committed" {
		if clientError != nil || output.String() != "240.0\n" {
			t.Fatalf("actual commit/output: %q %v", output.String(), clientError)
		}
	} else if clientError == nil {
		t.Fatal("invalid protocol succeeded without true commit")
	}
	if variant == "write-failure" && !errors.Is(clientError, io.ErrClosedPipe) {
		t.Fatalf("output error lost: %v", clientError)
	}
}
