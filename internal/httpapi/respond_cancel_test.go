package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

// 2026-11-12: a browser closing a tab mid-request logged "request failed:
// context canceled" at error level, and error logs page the owner. A request
// the client abandoned is not a server failure.
func TestWriteErrorKeepsClientCancellationsOutOfErrorAlerts(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	abandoned := httptest.NewRequest("GET", "/api/v1/board", nil).WithContext(ctx)
	writeError(httptest.NewRecorder(), abandoned, logger, context.Canceled)
	if strings.Contains(out.String(), "level=ERROR") || !strings.Contains(out.String(), "request canceled by client") {
		t.Fatalf("client cancellation logged as: %s", out.String())
	}

	out.Reset()
	live := httptest.NewRequest("GET", "/api/v1/board", nil)
	writeError(httptest.NewRecorder(), live, logger, context.Canceled)
	if !strings.Contains(out.String(), "level=ERROR") {
		t.Fatalf("a cancellation the client did not cause must stay an error: %s", out.String())
	}

	out.Reset()
	rec := httptest.NewRecorder()
	writeError(rec, live, logger, errors.New("disk full"))
	if !strings.Contains(out.String(), "level=ERROR") || rec.Code != 500 {
		t.Fatalf("a real failure must stay an error with a 500: %d %s", rec.Code, out.String())
	}
}
