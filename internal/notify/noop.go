package notify

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/app"
)

// Noop drops every notification. It stands in when ntfy is not configured, so
// the rest of the board never checks.
type Noop struct{}

// Notify implements app.Notifier.
func (Noop) Notify(context.Context, app.Notification) error {
	return nil
}
