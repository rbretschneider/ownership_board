// Package notify delivers notifications to the human.
package notify

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/app"
)

// Ntfy publishes to one ntfy topic URL.
type Ntfy struct {
	topicURL string
	token    string
	http     *http.Client
}

// NewNtfy builds a notifier for the full topic URL, such as
// https://ntfy.example.com/board. token is optional.
func NewNtfy(client *http.Client, topicURL, token string) *Ntfy {
	return &Ntfy{topicURL: topicURL, token: token, http: client}
}

// Notify implements app.Notifier.
func (n *Ntfy) Notify(ctx context.Context, notification app.Notification) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.topicURL, strings.NewReader(notification.Message))
	if err != nil {
		return fmt.Errorf("ntfy: build request: %w", err)
	}
	n.setHeaders(request.Header, notification)
	response, err := n.http.Do(request)
	if err != nil {
		return fmt.Errorf("ntfy: publish: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 300))
		return fmt.Errorf("ntfy: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (n *Ntfy) setHeaders(header http.Header, notification app.Notification) {
	header.Set("Title", headerSafe(notification.Title))
	header.Set("Priority", strconv.Itoa(int(notification.Priority)))
	if len(notification.Tags) > 0 {
		header.Set("Tags", strings.Join(notification.Tags, ","))
	}
	if notification.ClickURL != "" {
		header.Set("Click", notification.ClickURL)
	}
	if actions := formatActions(notification.Actions); actions != "" {
		header.Set("Actions", actions)
	}
	if n.token != "" {
		header.Set("Authorization", "Bearer "+n.token)
	}
}

// formatActions uses ntfy's short header format: "view, Label, url; ...".
func formatActions(actions []app.NotificationAction) string {
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		parts = append(parts, fmt.Sprintf("view, %s, %s", actionLabel(action.Label), action.URL))
	}
	return strings.Join(parts, "; ")
}

// headerSafe strips line breaks, which would inject headers, and RFC 2047
// encodes non-ASCII text such as emoji in a ticket title, which ntfy decodes.
func headerSafe(value string) string {
	flat := strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
	return mime.BEncoding.Encode("UTF-8", flat)
}

// actionLabel also drops the separators of ntfy's action header format.
func actionLabel(value string) string {
	return headerSafe(strings.NewReplacer(",", " ", ";", " ").Replace(value))
}
