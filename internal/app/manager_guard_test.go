package app_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// failBuild records a finished Windows build job on the ticket that failed one
// of the ticket's own tests.
func (f *fixture) failBuild(ticketID int64, n int) {
	f.t.Helper()
	result, _ := json.Marshal(map[string]any{"commands": []map[string]any{{"label": "verify", "exit_code": 8,
		"log": "The following tests FAILED:\n\t12 - perf_tests:AC-14 holes rebuild in budget (Failed)\n"}}})
	finished := f.clock.Now()
	job := domain.BuildJob{ID: fmt.Sprintf("%064x", n), TicketID: ticketID, Repo: "acme/api", SHA: fmt.Sprintf("%040x", n),
		Preset: "script", Target: domain.BuildJobTarget{OS: "windows"}, Status: "failed", Attempt: 1,
		CreatedAt: finished, FinishedAt: &finished, Result: result, BundleBytes: 3, BundleSHA256: fmt.Sprintf("%064x", 1)}
	if err := f.store.Write(f.ctx, func(r app.Repos) error {
		if err := r.BuildJobs().Insert(f.ctx, job); err != nil {
			return err
		}
		return r.BuildJobs().Save(f.ctx, job)
	}); err != nil {
		f.t.Fatalf("record build job: %v", err)
	}
}

func (f *fixture) autonomous(title string, repoID int64) int64 {
	f.t.Helper()
	detail, err := f.board.Create(f.ctx, owner, domain.TicketDraft{Kind: "story", Title: title, RepoID: repoID, IsAutonomous: true})
	if err != nil {
		f.t.Fatalf("create %q: %v", title, err)
	}
	return detail.Ticket.ID
}

func TestTheManagerCannotParkATicketWhoseOwnTestsFail(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	fix := f.autonomous("CI fix", api.ID)
	story := f.autonomous("Threading", api.ID)
	f.failBuild(story, 1)

	_, err := f.board.LinkTickets(f.ctx, manager, story, "depends_on", fix)
	requireErrorKind(t, err, domain.KindConflict)
	if !strings.Contains(err.Error(), "perf_tests:AC-14") {
		t.Fatalf("error = %v, want it to name the failing test", err)
	}
	// A person still decides for themselves.
	if _, err := f.board.LinkTickets(f.ctx, owner, story, "depends_on", fix); err != nil {
		t.Fatalf("owner link: %v", err)
	}
}

func TestTheManagersLinksExpireAfterSixHours(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	fix := f.autonomous("CI fix", api.ID)
	story := f.autonomous("story", api.ID)
	if _, err := f.board.LinkTickets(f.ctx, manager, story, "depends_on", fix); err != nil {
		t.Fatalf("manager link: %v", err)
	}
	if removed, err := f.board.ExpireManagerLinks(f.ctx, manager); err != nil || removed != 0 {
		t.Fatalf("fresh link expired: %d %v", removed, err)
	}
	f.clock.Advance(app.ManagerLinkTTL + time.Minute)
	if _, err := f.board.ExpireManagerLinks(f.ctx, agent); err == nil {
		t.Fatal("an ordinary agent expired links")
	}
	if removed, err := f.board.ExpireManagerLinks(f.ctx, manager); err != nil || removed != 1 {
		t.Fatalf("removed = %d %v, want 1", removed, err)
	}
	detail, err := f.board.Ticket(f.ctx, story)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(detail.WaitingOn) != 0 {
		t.Fatalf("still waiting on %v", detail.WaitingOn)
	}
}

// 2026-11-21: makercad #239's links to #218 expired every 6 hours while #218
// was building, and workers re-claimed #239 every 10 minutes.
func TestAManagerLinkToWorkStillMovingDoesNotExpire(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	prerequisite := f.autonomous("B2 heatmap", api.ID)
	story := f.autonomous("C integration", api.ID)
	if _, err := f.board.LinkTickets(f.ctx, manager, story, "depends_on", prerequisite); err != nil {
		t.Fatalf("manager link: %v", err)
	}
	f.clock.Advance(app.ManagerLinkTTL - time.Hour)
	// Worker activity lands on the timeline, not on the ticket's UpdatedAt
	// (2026-11-22: #218 was worked all night and #239's link still expired).
	if err := f.store.Write(f.ctx, func(repos app.Repos) error {
		entry := domain.NewEntry(prerequisite, domain.TimelineComment, domain.SystemActor, f.clock.Now()).WithBody("Windows verification queued")
		return repos.Timeline().Append(f.ctx, &entry)
	}); err != nil {
		t.Fatalf("touch prerequisite: %v", err)
	}
	f.clock.Advance(time.Hour + time.Minute)
	if removed, err := f.board.ExpireManagerLinks(f.ctx, manager); err != nil || removed != 0 {
		t.Fatalf("removed = %d %v, want the link to moving work kept", removed, err)
	}
	f.clock.Advance(app.ManagerLinkTTL)
	if removed, err := f.board.ExpireManagerLinks(f.ctx, manager); err != nil || removed != 1 {
		t.Fatalf("removed = %d %v, want the link expired once the prerequisite went quiet", removed, err)
	}
}

func TestTheManagerCannotFileADuplicateOfOpenWork(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	draft := autonomousDraft(api.ID)
	draft.Title = "Stop the audit eviction"
	draft.Spec += "\n\nChange `.github/workflows/ci.yml`."

	_, err := f.board.CreateAutonomous(f.ctx, manager, draft)
	requireErrorKind(t, err, domain.KindInvalid) // no files named

	draft.Files = []string{".github/workflows/ci.yml"}
	first, err := f.board.CreateAutonomous(f.ctx, manager, draft)
	if err != nil {
		t.Fatalf("first filing: %v", err)
	}
	if !strings.Contains(first.Ticket.Spec, "Files expected to change:") {
		t.Fatalf("spec = %q, want the declared files", first.Ticket.Spec)
	}
	draft.Title = "Fix the audit concurrency group"
	_, err = f.board.CreateAutonomous(f.ctx, manager, draft)
	requireErrorKind(t, err, domain.KindConflict)
	if !strings.Contains(err.Error(), fmt.Sprintf("ticket %d", first.Ticket.ID)) {
		t.Fatalf("error = %v, want it to point at the open ticket", err)
	}
}
