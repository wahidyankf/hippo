package hostprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"time"
)

// Run serves one actual Darwin collector subprocess.
func Run(ctx context.Context) error {
	if os.Getenv(HelperEnvironment) != "1" {
		return errors.New("public host probe helper marker is absent")
	}
	address := os.Getenv("PUBLIC_TUI_HOST_PROBE_ADDRESS")
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return errors.New("public host probe helper address is not owned loopback")
	}
	separator := slices.Index(os.Args, "--")
	if separator < 0 || separator+1 >= len(os.Args) {
		return errors.New("public host probe helper argv is missing")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	request := Request{
		Nonce: os.Getenv("PUBLIC_TUI_HOST_PROBE_NONCE"), Executable: executable,
		RaceOptions: os.Getenv("GORACE"),
		PID:         os.Getpid(), ParentPID: os.Getppid(), Name: os.Args[separator+1], Arguments: slices.Clone(os.Args[separator+2:]),
	}
	if request.Nonce == "" || CommandIndex(request) < 0 {
		return errors.New("public host probe helper request is invalid")
	}
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "tcp4", address)
	if err != nil {
		return err
	}
	return exchangeAndClose(connection, request, os.Stdout)
}

// Exchange writes real query output before acknowledging and awaiting commit.
func Exchange(connection net.Conn, request Request, output io.Writer) error {
	if err := connection.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	encoder := json.NewEncoder(connection)
	if err := encoder.Encode(request); err != nil {
		return err
	}
	decoder := json.NewDecoder(io.LimitReader(connection, 8192))
	var response Response
	if err := decoder.Decode(&response); err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("external host probe refused: %s", response.Error)
	}
	if _, err := io.WriteString(output, response.Output); err != nil {
		return err
	}
	if err := encoder.Encode("output-written"); err != nil {
		return err
	}
	var committed string
	if err := decoder.Decode(&committed); err != nil {
		return err
	}
	if committed != "committed" {
		return errors.New("public host probe output was not committed")
	}
	return nil
}

func exchangeAndClose(connection net.Conn, request Request, output io.Writer) error {
	return errors.Join(Exchange(connection, request, output), connection.Close())
}
