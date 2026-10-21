// Command ownership runs the ownership_board server.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rbretschneider/ownership_board/internal/config"
	"github.com/rbretschneider/ownership_board/internal/logging"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// Server timeouts. There is no write timeout: the event stream and the agent
// long-poll hold responses open on purpose, and each bounds itself.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 15 * time.Second
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger, logFile, err := newLogger(cfg)
	if err != nil {
		return err
	}
	defer logFile.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	application, err := build(ctx, cfg, logger)
	if err != nil {
		logger.Error("startup failed", "error", err)
		return err
	}
	defer application.close()
	application.startBackground(ctx)
	return serve(ctx, cfg, logger, application.handler)
}

func newLogger(cfg config.Config) (*slog.Logger, io.Closer, error) {
	var alerts *logging.AlertHandler
	if cfg.Ntfy.AlertsURL != "" {
		alerts = logging.NewAlertHandler(newNtfy(cfg.Ntfy.AlertsURL, cfg.Ntfy.Token), systemClock)
	}
	return logging.New(cfg.LogLevel, cfg.LogDir(), alerts)
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger, handler http.Handler) error {
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", cfg.Port), Handler: handler,
		ReadHeaderTimeout: readHeaderTimeout, ReadTimeout: readTimeout, IdleTimeout: idleTimeout,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	failed := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", server.Addr, "version", version)
		failed <- server.ListenAndServe()
	}()
	select {
	case err := <-failed:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http shutdown: %w", err)
	}
	return nil
}

// healthcheck lets the distroless image check itself without curl.
func healthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
