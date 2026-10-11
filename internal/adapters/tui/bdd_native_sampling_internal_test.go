package tui

import (
	"fmt"
	"strings"
	"testing"
)

func samplingRenderedFrame(t *testing.T, content string) BDDNativeTransportFrame {
	t.Helper()
	screen := NewBDDNativeTransportScreen(40, 16)
	var encoded strings.Builder
	if _, err := encoded.WriteString("\x1b[?1049h\x1b[2J"); err != nil {
		t.Fatal(err)
	}
	for index, row := range strings.Split(content, "\n") {
		if _, err := fmt.Fprintf(&encoded, "\x1b[%d;1H%s", index+1, row); err != nil {
			t.Fatal(err)
		}
	}
	if err := screen.Feed([]byte(encoded.String())); err != nil {
		t.Fatal(err)
	}
	return screen.Snapshot()
}

func TestBDDNativeSamplingStableRendererPredicates(t *testing.T) {
	for _, test := range []struct {
		name   string
		key    rune
		steps  int
		offset int
		tokens []string
		want   bool
	}{
		{name: "capacity first8j", key: 'c', steps: 8, offset: 8, tokens: []string{"Disk free:", "Reservation accounting"}, want: true},
		{name: "capacity18j cannot contain accounting heading", key: 'c', steps: 18, offset: 18, tokens: []string{"Waiting request:", "Reservation accounting"}},
		{name: "capacity18j waiting request", key: 'c', steps: 18, offset: 18, tokens: []string{"Waiting request:"}, want: true},
		{name: "capacity end", key: 'c', steps: 218, offset: 25, tokens: []string{"process RSS or CPU usage."}, want: true},
		{name: "help first8j", key: '?', steps: 8, offset: 8, tokens: []string{"Enter: open selected job detail", "c:"}, want: true},
		{name: "help end", key: '?', steps: 208, offset: 16, tokens: []string{"Host prefixes"}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, _ := newViewsContract(t, 40, 16)
			state.snapshot = BDDNativeObserverSnapshot()
			state.busy = false
			state.Update(modelKey(test.key))
			for range test.steps {
				state.Update(modelKey('j'))
			}
			if state.body.YOffset() != test.offset {
				t.Fatalf("stable body offset=%d want=%d", state.body.YOffset(), test.offset)
			}
			view := state.View()
			frame := samplingRenderedFrame(t, view.Content)
			if frame.Width != 40 || frame.Height != 16 || !frame.Alternate || frame.OverflowWrites != 0 {
				t.Fatalf("invalid decoded renderer geometry: %+v", frame)
			}
			content := strings.Join(frame.Rows, "\n")
			matched := true
			for _, token := range test.tokens {
				matched = matched && strings.Contains(content, token)
			}
			if matched != test.want {
				t.Fatalf("stable predicate=%t want=%t tokens=%q; rows=%q", matched, test.want, test.tokens, frame.Rows)
			}
		})
	}
}
