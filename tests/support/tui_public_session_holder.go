package support

import (
	"errors"
	"os"
	"os/signal"
	"sync"
)

// PublicTUISessionHolderEnvironment marks only the fixed owned holder dispatch.
const PublicTUISessionHolderEnvironment = "PUBLIC_TUI_SESSION_HOLDER"

// PublicTUISessionHolderHelper is the sole selected stable-session fixture entry.
const PublicTUISessionHolderHelper = "TestBDDPublicTUISessionHolderHelper"

type publicHolderSignals struct {
	mu            sync.Mutex
	names         []string
	failure       error
	notifications chan os.Signal
	stop, joined  chan struct{}
}

func startPublicHolderSignals(signals ...os.Signal) *publicHolderSignals {
	owner := &publicHolderSignals{notifications: make(chan os.Signal, 16), stop: make(chan struct{}), joined: make(chan struct{})}
	// Caught dispositions reset at child exec. Ignoring or forwarding would
	// change the unchanged candidate's own terminal and CLI signal handling.
	signal.Notify(owner.notifications, signals...)
	go func() {
		defer close(owner.joined)
		for {
			select {
			case observed := <-owner.notifications:
				owner.mu.Lock()
				if len(owner.names) >= 1024 {
					owner.failure = errors.New("public holder signal capture exceeds bounds")
				} else {
					owner.names = append(owner.names, observed.String())
				}
				owner.mu.Unlock()
			case <-owner.stop:
				return
			}
		}
	}()
	return owner
}

func (owner *publicHolderSignals) snapshot() []string {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return append([]string(nil), owner.names...)
}

func (owner *publicHolderSignals) close() {
	signal.Stop(owner.notifications)
	close(owner.stop)
	<-owner.joined
}

func (owner *publicHolderSignals) captureError() error {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return owner.failure
}
