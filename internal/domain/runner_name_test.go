package domain_test

import (
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestWorkerIdentityParts(t *testing.T) {
	cases := []struct {
		identity, worker, family string
		slot                     int
		isWorker                 bool
	}{
		{"@worker/7/claude_1", "claude_1", "claude", 1, true},
		{"@worker/12/Codex-3", "Codex-3", "codex", 3, true},
		{"@worker/7/builder", "builder", "builder", 0, true},
		{"claude-loop", "", "", 0, false},
		{"@worker/x/claude_1", "", "", 0, false},
	}
	for _, c := range cases {
		workerID, isWorker := domain.WorkerIDOf(c.identity)
		if workerID != c.worker || isWorker != c.isWorker {
			t.Errorf("WorkerIDOf(%q) = %q, %v", c.identity, workerID, isWorker)
			continue
		}
		if !isWorker {
			continue
		}
		if family, slot := domain.WorkerFamily(workerID); family != c.family || slot != c.slot {
			t.Errorf("WorkerFamily(%q) = %q, %d", workerID, family, slot)
		}
	}
}

func TestRunnerDirectoryLabels(t *testing.T) {
	directory := domain.NewRunnerDirectory([]domain.RunnerName{{Family: "claude", Position: 1, Name: "Michael", Worker: "claude_1"}})
	if got := directory.Label("@worker/7/claude_1"); got != "Michael (claude_1)" {
		t.Errorf("named label = %q", got)
	}
	for _, raw := range []string{"@worker/7/gemini_1", "claude-loop"} {
		if got := directory.Label(raw); got != raw {
			t.Errorf("unnamed label = %q, want %q", got, raw)
		}
	}
}
