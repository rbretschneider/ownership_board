package httpapi

import (
	"errors"
	"testing"
	"time"
)

func TestWriteLimiterRefusesPastTheWindowThenRecovers(t *testing.T) {
	now := time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)
	limiter := newWriteLimiter(func() time.Time { return now })
	for i := 0; i < agentWritesPerWindow; i++ {
		if err := limiter.allow(1); err != nil {
			t.Fatalf("write %d refused: %v", i, err)
		}
	}
	var limited *rateLimitedError
	if err := limiter.allow(1); !errors.As(err, &limited) {
		t.Fatalf("write past the limit: %v", err)
	}
	if err := limiter.allow(2); err != nil {
		t.Fatalf("another token was limited: %v", err)
	}
	now = now.Add(agentWriteWindow)
	if err := limiter.allow(1); err != nil {
		t.Fatalf("after the window: %v", err)
	}
}
