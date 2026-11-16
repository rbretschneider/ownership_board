package app_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// worker is the identity a worker sharing token tokenID presents.
func worker(tokenID int, workerID string) domain.Actor {
	return domain.Actor{Kind: domain.ActorAgent, ID: int64(tokenID), Name: fmt.Sprintf("@worker/%d/%s", tokenID, workerID)}
}

func (f *fixture) runnerName(actor domain.Actor) (domain.RunnerName, bool) {
	f.t.Helper()
	name, isNamed, err := f.agents.EnsureRunnerName(f.ctx, actor)
	if err != nil {
		f.t.Fatalf("ensure runner name for %s: %v", actor.Name, err)
	}
	return name, isNamed
}

func (f *fixture) requireRunnerName(actor domain.Actor, want string) {
	f.t.Helper()
	if name, _ := f.runnerName(actor); name.Name != want {
		f.t.Fatalf("%s is named %q, want %q", actor.Name, name.Name, want)
	}
}

func TestRunnerNameFollowsTheWorkerSlotWhateverTheOrder(t *testing.T) {
	f := newFixture(t)
	f.requireRunnerName(worker(7, "claude_3"), "Jim")
	f.requireRunnerName(worker(7, "codex_2"), "Debra")
	f.requireRunnerName(worker(7, "claude_1"), "Michael")
	f.requireRunnerName(worker(7, "codex_1"), "Ray")
	f.requireRunnerName(worker(7, "claude_6"), "Andy")
}

func TestRunnerNameIsStableAcrossCallsAndReRegistration(t *testing.T) {
	f := newFixture(t)
	f.requireRunnerName(worker(7, "claude_1"), "Michael")
	f.requireRunnerName(worker(7, "claude_1"), "Michael")
	// A new token (re-registration) keeps the worker's name.
	f.requireRunnerName(worker(9, "claude_1"), "Michael")
	names, err := f.board.RunnerNames(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 {
		t.Fatalf("assigned names = %+v, want one", names)
	}
}

func TestRunnerWithoutAFreeSlotTakesTheLastFreeName(t *testing.T) {
	f := newFixture(t)
	f.requireRunnerName(worker(7, "claude"), "Holly")
	f.requireRunnerName(worker(7, "claude_99"), "Jan")
	f.requireRunnerName(worker(7, "claude_1"), "Michael")
}

func TestRunnerOutsideAnyPoolKeepsItsIdentity(t *testing.T) {
	f := newFixture(t)
	for _, actor := range []domain.Actor{worker(7, "gemini_1"), agent, owner} {
		if name, isNamed := f.runnerName(actor); isNamed {
			t.Errorf("%s got name %+v, want none", actor.Name, name)
		}
	}
	if got := domain.RunnerLabel("@worker/7/gemini_1", ""); got != "@worker/7/gemini_1" {
		t.Errorf("label = %q", got)
	}
}

func TestRunnerNameAssignmentIsRaceSafe(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	got := make([]string, 12)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name, _, err := f.agents.EnsureRunnerName(f.ctx, worker(7, fmt.Sprintf("claude_%d", i%6+1)))
			if err != nil {
				t.Errorf("ensure: %v", err)
			}
			got[i] = name.Name
		}()
	}
	wg.Wait()
	want := []string{"Michael", "Dwight", "Jim", "Pam", "Angela", "Andy"}
	for i, name := range got {
		if name != want[i%6] {
			t.Errorf("claude_%d = %q, want %q", i%6+1, name, want[i%6])
		}
	}
}

func TestRenamedRunnerKeepsItsNewName(t *testing.T) {
	f := newFixture(t)
	f.runnerName(worker(7, "claude_1"))
	if _, err := f.board.RenameRunner(f.ctx, owner, "claude_1", "  Scott  "); err != nil {
		t.Fatal(err)
	}
	f.requireRunnerName(worker(7, "claude_1"), "Scott")
	f.requireRunnerName(worker(9, "claude_1"), "Scott")
}

func TestRenameToAFreePoolNameSwapsIt(t *testing.T) {
	f := newFixture(t)
	f.runnerName(worker(7, "claude_1"))
	if _, err := f.board.RenameRunner(f.ctx, owner, "claude_1", "kevin"); err != nil {
		t.Fatal(err)
	}
	f.requireRunnerName(worker(7, "claude_1"), "kevin")
	// Kevin's slot now holds Michael, so the pool keeps every name.
	f.requireRunnerName(worker(7, "claude_7"), "Michael")
}

func TestRenameRefusesAnotherRunnersNameAndAgents(t *testing.T) {
	f := newFixture(t)
	f.runnerName(worker(7, "claude_1"))
	f.runnerName(worker(7, "claude_2"))
	_, err := f.board.RenameRunner(f.ctx, owner, "claude_1", "Dwight")
	if domain.KindOf(err) != domain.KindConflict {
		t.Errorf("rename to a taken name: %v, want conflict", err)
	}
	if _, err := f.board.RenameRunner(f.ctx, agent, "claude_1", "Scott"); domain.KindOf(err) != domain.KindForbidden {
		t.Errorf("agent rename: %v, want forbidden", err)
	}
	if _, err := f.board.RenameRunner(f.ctx, owner, "claude_5", "Scott"); domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("rename unnamed worker: %v, want not found", err)
	}
	if _, err := f.board.RenameRunner(f.ctx, owner, "claude_1", "@board"); domain.KindOf(err) != domain.KindInvalid {
		t.Errorf("rename to @board: %v, want invalid", err)
	}
	f.requireRunnerName(worker(7, "claude_1"), "Michael")
}

func TestClaimNamesTheWorkerForTheTeamAndTheBrief(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("example/tools")
	f.ticket("Add a thing", repo.ID)
	codex := worker(7, "codex_1")
	work := f.mustClaim(codex, app.ClaimInput{})
	if !strings.Contains(work.Brief, "- Lease: Ray (codex_1) until") {
		t.Errorf("brief lease line does not name the runner:\n%s", work.Brief)
	}
	team, err := f.board.Team(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(team.Runners) != 1 {
		t.Fatalf("runners = %+v", team.Runners)
	}
	runner := team.Runners[0]
	if runner.Name != codex.Name || runner.DisplayName != "Ray" || runner.Family != "codex" {
		t.Errorf("runner = %+v, want Ray of codex keeping its identity", runner)
	}
}
