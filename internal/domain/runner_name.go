package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WorkerIdentityPrefix starts the identity of a worker sharing an agent token:
// "@worker/<token id>/<worker id>".
const WorkerIdentityPrefix = "@worker/"

// MaxRunnerNameLength bounds a runner's display name.
const MaxRunnerNameLength = 32

// RunnerName is one name in a family's pool. Worker is the worker id it is
// assigned to ("claude_1"), or empty while the name is free.
type RunnerName struct {
	Family     string
	Position   int
	Name       string
	Worker     string
	AssignedAt time.Time
}

// IsAssigned reports whether a worker holds the name.
func (n RunnerName) IsAssigned() bool {
	return n.Worker != ""
}

// workerIdentityPattern matches "@worker/<token id>/<worker id>".
var workerIdentityPattern = regexp.MustCompile(`^@worker/\d+/([A-Za-z0-9_-]{1,48})$`)

// workerSlotPattern splits a worker id into its family and slot: "claude_1"
// is family "claude", slot 1.
var workerSlotPattern = regexp.MustCompile(`^(.+?)[_-](\d{1,4})$`)

// WorkerIDOf returns the worker id inside a worker identity, or false when the
// name is not a worker identity.
func WorkerIDOf(identity string) (string, bool) {
	match := workerIdentityPattern.FindStringSubmatch(identity)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// WorkerFamily splits a worker id into its family and slot. A worker id with
// no numeric suffix is its own family with slot zero.
func WorkerFamily(workerID string) (family string, slot int) {
	match := workerSlotPattern.FindStringSubmatch(workerID)
	if match == nil {
		return strings.ToLower(workerID), 0
	}
	slot, _ = strconv.Atoi(match[2])
	return strings.ToLower(match[1]), slot
}

// NormalizeRunnerName validates a display name the owner chose for a runner.
func NormalizeRunnerName(name string) (string, error) {
	trimmed, err := RequireText("name", name, MaxRunnerNameLength)
	if err != nil {
		return "", err
	}
	if err := RejectHiddenText("name", trimmed); err != nil {
		return "", err
	}
	if strings.HasPrefix(trimmed, "@") {
		return "", Invalidf("name must not start with @")
	}
	return trimmed, nil
}

// RunnerLabel is how agent-facing text names a runner: "Michael (claude_1)"
// when it has a display name, otherwise the identity unchanged.
func RunnerLabel(identity, displayName string) string {
	workerID, isWorker := WorkerIDOf(identity)
	if !isWorker || displayName == "" {
		return identity
	}
	return fmt.Sprintf("%s (%s)", displayName, workerID)
}
