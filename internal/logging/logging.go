// Package logging sets up the board's logs: stdout for docker logs, a
// size-capped rolling file that cannot fill the disk, and ntfy alerts for
// errors.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Rolling file limits: at most four files of five megabytes.
const (
	fileMaxSizeMB  = 5
	fileMaxBackups = 3
	fileName       = "ownership.log"
)

// New builds the logger. alerts may be nil to skip ntfy alerting.
func New(level slog.Level, logDir string, alerts *AlertHandler) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return nil, nil, err
	}
	file := &lumberjack.Logger{
		Filename: filepath.Join(logDir, fileName), MaxSize: fileMaxSizeMB, MaxBackups: fileMaxBackups,
	}
	output := io.MultiWriter(os.Stdout, neverFail{file})
	var handler slog.Handler = slog.NewTextHandler(output, &slog.HandlerOptions{Level: level})
	if alerts != nil {
		handler = alerts.Wrap(handler)
	}
	return slog.New(handler), file, nil
}

// neverFail keeps a broken log file (full disk, bad permissions) from ever
// taking down the request that tried to log.
type neverFail struct {
	w io.Writer
}

func (n neverFail) Write(p []byte) (int, error) {
	_, _ = n.w.Write(p)
	return len(p), nil
}
