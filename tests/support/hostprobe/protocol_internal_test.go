package hostprobe

import (
	"io"
	"net"
)

// ExchangeAndCloseForTest exposes only the existing private exchange to external protocol tests.
func ExchangeAndCloseForTest(connection net.Conn, request Request, output io.Writer) error {
	return exchangeAndClose(connection, request, output)
}
