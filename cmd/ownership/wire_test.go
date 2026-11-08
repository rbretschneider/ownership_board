package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/config"
)

func TestBuildJobReaperShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.Config{DataDir: t.TempDir(), AuthTokenSecret: "0123456789abcdef0123456789abcdef"}
	a, e := build(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	a.startBackground(ctx)
	a.close()
	select {
	case <-a.reaperDone:
	case <-time.After(time.Second):
		t.Fatal("reaper was not joined")
	}
}
