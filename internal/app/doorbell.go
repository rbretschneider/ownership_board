package app

import (
	"context"
	"time"
)

// Doorbell lets a caller wait for the next change on the board.
type Doorbell interface {
	// Listen starts listening. Changes after this call wake the listener, even
	// ones that happen before Wait is called.
	Listen() Listener
}

// Listener waits for board changes. Close it when done.
type Listener interface {
	// Wait blocks until a change, the timeout, or the end of ctx, and reports
	// whether a change woke it.
	Wait(ctx context.Context, timeout time.Duration) bool
	Close()
}
