//go:build darwin

package host

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func TestDarwinFIFOCommandHasFixedPrivateBoundary(t *testing.T) {
	var output bytes.Buffer
	command := newDarwinFIFOCommand(t.Context(), "/absolute/evidence", &output)
	if command.Path != "/bin/cat" || !reflect.DeepEqual(command.Args, []string{"/bin/cat", "--", "/absolute/evidence"}) || command.Stdout != &output || command.Stderr != io.Discard || command.Stdin != nil || len(command.ExtraFiles) != 0 || command.Env != nil || command.Cancel == nil {
		t.Fatalf("reader command lost fixed executable/operand/private-stream ownership")
	}
}
