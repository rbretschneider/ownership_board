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
	store  *sqlite.DB
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
		t: t, ctx: ctx, clock: clk, events: events, store: db,
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

// agentAsk claims the next ticket, which must be id, and asks on it, the way a
// real agent does.
func (f *fixture) agentAsk(id int64, input app.AskInput) (app.TicketDetail, error) {
	f.t.Helper()
	if work := f.mustClaim(agent, app.ClaimInput{}); work.Ticket.Ticket.ID != id {
		f.t.Fatalf("claimed #%d, want #%d", work.Ticket.Ticket.ID, id)
	}
	return f.board.Ask(f.ctx, agent, id, input)
}

func (f *fixture) requireStage(id int64, want domain.Stage) {
	f.t.Helper()
	if got := f.detail(id).Ticket.Stage; got != want {
		f.t.Fatalf("ticket %d is in stage %s, want %s", id, got, want)
	}
}

// toReady takes ticket id, which must be the next claimable one, through
// refinement and the owner's spec approval.
func (f *fixture) toReady(id int64) {
	f.t.Helper()
	if work := f.mustClaim(agent, app.ClaimInput{}); work.Ticket.Ticket.ID != id {
		f.t.Fatalf("claimed #%d, want #%d", work.Ticket.Ticket.ID, id)
	}
	detail, err := f.board.SaveSpec(f.ctx, agent, id, app.SaveTextInput{Text: "Do the thing.", IsSubmit: true})
	if err != nil {
		f.t.Fatalf("submit spec: %v", err)
	}
	review := app.ReviewInput{Checkpoint: "spec", Revision: detail.Ticket.SpecRevision}
	if _, err := f.board.Approve(f.ctx, owner, id, review); err != nil {
		f.t.Fatalf("approve spec: %v", err)
	}
	f.requireStage(id, domain.StageReady)
}

// toBuild gives ticket id a design and drags it into In Progress, where it
// lands in build because the design checkpoint is off by default.
func (f *fixture) toBuild(id int64) {
	f.t.Helper()
	if _, err := f.board.SaveDesign(f.ctx, owner, id, app.SaveTextInput{Text: "The plan."}); err != nil {
		f.t.Fatalf("save design: %v", err)
	}
	f.move(owner, id, domain.LaneInProgress, nil)
	f.requireStage(id, domain.StageBuild)
}
