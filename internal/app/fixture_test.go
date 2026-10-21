package app_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/broker"
	"github.com/rbretschneider/ownership_board/internal/clock/clocktest"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/store/sqlite"
)

var (
	owner = domain.Actor{Kind: domain.ActorHuman, ID: 1, Name: "Owner"}
	agent = domain.Actor{Kind: domain.ActorAgent, ID: 1, Name: "loop-a"}
	other = domain.Actor{Kind: domain.ActorAgent, ID: 2, Name: "loop-b"}
)

// fixture is a board on a real, throwaway database.
type fixture struct {
	t      *testing.T
	ctx    context.Context
	clock  *clocktest.Clock
	events *recordingSink
	board  *app.BoardService
	agents *app.AgentService
	repos  *app.RepoService
}

func newFixture(t *testing.T, sources ...app.RepoSource) *fixture {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "board.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	clk := clocktest.New()
	events := &recordingSink{}
	hub := broker.New()
	sink := app.FanoutSink{events, hub}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &fixture{
		t: t, ctx: ctx, clock: clk, events: events,
		board:  app.NewBoardService(db, clk, sink),
		agents: app.NewAgentService(db, clk, sink, hub),
		repos:  app.NewRepoService(db, clk, staticCatalog(sources), logger),
	}
}

func (f *fixture) repo(fullName string) domain.Repo {
	f.t.Helper()
	repo, err := f.repos.CreateManual(f.ctx, owner, domain.ManualRepoDraft{FullName: fullName})
	if err != nil {
		f.t.Fatalf("create repo %s: %v", fullName, err)
	}
	return repo
}

func (f *fixture) ticket(title string, repoID int64) int64 {
	f.t.Helper()
	detail, err := f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "story", Title: title, RepoID: repoID})
	if err != nil {
		f.t.Fatalf("create ticket %q: %v", title, err)
	}
	return detail.Ticket.ID
}

func (f *fixture) claim(actor domain.Actor, input app.ClaimInput) (app.Work, bool) {
	f.t.Helper()
	work, isClaimed, err := f.agents.Claim(f.ctx, actor, input)
	if err != nil {
		f.t.Fatalf("claim: %v", err)
	}
	return work, isClaimed
}

func (f *fixture) mustClaim(actor domain.Actor, input app.ClaimInput) app.Work {
	f.t.Helper()
	work, isClaimed := f.claim(actor, input)
	if !isClaimed {
		f.t.Fatal("expected work, got none")
	}
	return work
}

func (f *fixture) move(actor domain.Actor, id int64, lane domain.Lane, afterID *int64) {
	f.t.Helper()
	if _, err := f.board.Move(f.ctx, actor, id, app.MoveInput{Lane: string(lane), AfterID: afterID}); err != nil {
		f.t.Fatalf("move %d to %s: %v", id, lane, err)
	}
}

func (f *fixture) detail(id int64) app.TicketDetail {
	f.t.Helper()
	detail, err := f.board.Ticket(f.ctx, id)
	if err != nil {
		f.t.Fatalf("get ticket %d: %v", id, err)
	}
	return detail
}

func (f *fixture) requireLane(id int64, want domain.Lane) {
	f.t.Helper()
	if got := f.detail(id).Ticket.Lane; got != want {
		f.t.Fatalf("ticket %d is in %s, want %s", id, got, want)
	}
}

func (f *fixture) laneOrder(lane domain.Lane) []int64 {
	f.t.Helper()
	board, err := f.board.Board(f.ctx, app.RepoScope{})
	if err != nil {
		f.t.Fatalf("board: %v", err)
	}
	var ids []int64
	for _, summary := range board.Tickets {
		if summary.Ticket.Lane == lane {
			ids = append(ids, summary.Ticket.ID)
		}
	}
	return ids
}

func ask(prompts ...string) app.AskInput {
	input := app.AskInput{Summary: "I need to know"}
	for _, prompt := range prompts {
		input.Questions = append(input.Questions, domain.QuestionDraft{Prompt: prompt, Options: []string{"yes", "no"}})
	}
	return input
}

func requireErrorKind(t *testing.T, err error, want domain.ErrorKind) {
	t.Helper()
	if got := domain.KindOf(err); got != want {
		t.Fatalf("error kind = %v (%v), want %v", got, err, want)
	}
}

func int64Ptr(v int64) *int64 { return &v }

// recordingSink keeps every published event.
type recordingSink struct {
	mu     sync.Mutex
	events []app.Event
}

func (r *recordingSink) Publish(_ context.Context, events []app.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
}

func (r *recordingSink) types() []domain.TimelineType {
	r.mu.Lock()
	defer r.mu.Unlock()
	types := make([]domain.TimelineType, 0, len(r.events))
	for _, event := range r.events {
		types = append(types, event.Type)
	}
	return types
}

// staticCatalog is a fixed list of repo sources.
type staticCatalog []app.RepoSource

func (c staticCatalog) Sources(context.Context) ([]app.RepoSource, error) {
	return c, nil
}
