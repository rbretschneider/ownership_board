package app_test

import (
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// The board and the ticket detail both derive stage attempts from the stored
// history: a PR repair, a failed Windows run (re-sent once unchanged) and a
// review that asked for fixes.
func TestBoardCountsStageAttemptsFromHistory(t *testing.T) {
	f := newFixture(t)
	id := f.inReview("loops a few times")
	if _, err := f.board.ReportPullRequest(f.ctx, agent, id, app.PullRequestRepair, "check failed: unit tests"); err != nil {
		t.Fatalf("repair: %v", err)
	}
	f.mustClaim(agent, app.ClaimInput{})
	steps := []string{
		"comment:Windows verification queued for commit 1111111 (dotnet).",
		"gpt-6.1-sol · build-server:queued:Windows x64 · awaiting Windows",
		"gpt-6.1-sol · awaiting repair",
		"comment:Windows verification queued for commit 1111111 (dotnet).",
		"gpt-6.1-sol · build-server:queued:Windows x64 · awaiting Windows",
		"gpt-6.1-sol · awaiting repair",
		"gpt-6.1-sol",
		"comment:Windows verification queued for commit 2222222 (dotnet).",
		"gpt-6.1-sol · build-server:queued:Windows x64 · awaiting Windows",
		"gpt-6.1-sol · awaiting review",
		"gpt-6.1-sol-review · adversarial review",
		"gpt-6.1-sol",
	}
	for _, step := range steps {
		var err error
		if body, isComment := cutPrefix(step, "comment:"); isComment {
			_, err = f.board.Comment(f.ctx, agent, id, body)
		} else {
			_, err = f.agents.SetWorkModel(f.ctx, agent, id, step)
		}
		if err != nil {
			t.Fatalf("%q: %v", step, err)
		}
	}
	want := domain.StageAttempts{Build: 4, Test: 2, TestFailures: 1, Review: 1, ReviewFixes: 1, PR: 1, PRRepairs: 1}
	if got := f.detail(id).Attempts; got != want {
		t.Fatalf("detail attempts %+v, want %+v", got, want)
	}
	board, err := f.board.Board(f.ctx, app.RepoScope{})
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	found := false
	for _, summary := range board.Tickets {
		if summary.Ticket.ID == id {
			found = true
			if summary.Attempts != want {
				t.Fatalf("board attempts %+v, want %+v", summary.Attempts, want)
			}
		}
	}
	if !found {
		t.Fatal("ticket missing from the board")
	}
}

func cutPrefix(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return "", false
}
