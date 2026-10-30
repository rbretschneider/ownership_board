package app_test

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

const dependsOn = string(domain.RelationDependsOn)

// create files a story as actor, waiting on dependsOn.
func (f *fixture) create(actor domain.Actor, title string, repoID int64, dependsOn ...int64) (app.TicketDetail, error) {
	return f.board.Create(f.ctx, actor, domain.TicketDraft{Kind: "story", Title: title, RepoID: repoID, DependsOn: dependsOn})
}

func (f *fixture) mustLink(actor domain.Actor, id, otherID int64) {
	f.t.Helper()
	if _, err := f.board.LinkTickets(f.ctx, actor, id, dependsOn, otherID); err != nil {
		f.t.Fatalf("%s: %d depends_on %d: %v", actor.Name, id, otherID, err)
	}
}

func (f *fixture) ticketCount() int {
	f.t.Helper()
	board, err := f.board.Board(f.ctx, app.RepoScope{})
	if err != nil {
		f.t.Fatalf("board: %v", err)
	}
	return len(board.Tickets)
}

func (f *fixture) waitingOn(id int64) []int64 {
	f.t.Helper()
	board, err := f.board.Board(f.ctx, app.RepoScope{})
	if err != nil {
		f.t.Fatalf("board: %v", err)
	}
	for _, summary := range board.Tickets {
		if summary.Ticket.ID == id {
			return summary.WaitingOn
		}
	}
	f.t.Fatalf("ticket %d is not on the board", id)
	return nil
}

// requireNoClaim checks actor gets no work at all.
func (f *fixture) requireNoClaim(actor domain.Actor, why string) {
	f.t.Helper()
	if work, isClaimed := f.claim(actor, app.ClaimInput{}); isClaimed {
		f.t.Fatalf("%s: claimed #%d in %s", why, work.Ticket.Ticket.ID, work.Ticket.Ticket.Stage)
	}
}

// buildingByOther returns a ticket in build, leased by the other agent.
func (f *fixture) buildingByOther(title string, repoID int64) int64 {
	f.t.Helper()
	id := f.ticket(title, repoID)
	f.toReady(id)
	f.toBuild(id)
	if work := f.mustClaim(other, app.ClaimInput{}); work.Ticket.Ticket.ID != id {
		f.t.Fatalf("other claimed #%d, want #%d", work.Ticket.Ticket.ID, id)
	}
	return id
}

func dependenciesMet(timeline []domain.TimelineEntry) []domain.TimelineEntry {
	var met []domain.TimelineEntry
	for _, entry := range timeline {
		if entry.Type == domain.TimelineDependenciesMet {
			met = append(met, entry)
		}
	}
	return met
}

func requireMessage(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error %v, want it to contain %q", err, want)
	}
}

func TestDependsOnShowsFromBothEnds(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	a := f.ticket("a", api.ID)
	b := f.ticket("b", api.ID)
	f.mustLink(owner, a, b)

	related := f.detail(a).Related
	if len(related) != 1 || !related[0].IsOutgoing || related[0].Relation.Kind.Label(true) != "depends on" || related[0].Other.ID != b {
		t.Fatalf("dependant side %+v", related)
	}
	related = f.detail(b).Related
	if len(related) != 1 || related[0].IsOutgoing || related[0].Relation.Kind.Label(false) != "needed by" || related[0].Other.ID != a {
		t.Fatalf("prerequisite side %+v", related)
	}
	brief, err := f.agents.Brief(f.ctx, a)
	if err != nil || !strings.Contains(brief, fmt.Sprintf("- Depends on: #%d (waiting)", b)) {
		t.Fatalf("dependant brief lacks the dependency (%v):\n%s", err, brief)
	}
	brief, err = f.agents.Brief(f.ctx, b)
	if err != nil || !strings.Contains(brief, fmt.Sprintf("- Needed by: #%d", a)) {
		t.Fatalf("prerequisite brief lacks the dependant (%v):\n%s", err, brief)
	}
}

func TestCreateWithDependsOn(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	web := f.repo("acme/web")
	b := f.ticket("b", api.ID)
	c := f.ticket("c", api.ID)
	elsewhere := f.ticket("elsewhere", web.ID)

	detail, err := f.create(agent, "a", api.ID, b, c, b)
	if err != nil {
		t.Fatalf("create with dependencies: %v", err)
	}
	if !slices.Equal(detail.WaitingOn, []int64{b, c}) {
		t.Fatalf("waiting on %v, want [%d %d]", detail.WaitingOn, b, c)
	}

	unassigned := f.ticket("unassigned", 0)
	tooMany := make([]int64, domain.MaxDependencies+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	cases := map[string]struct {
		repoID    int64
		dependsOn []int64
		want      string
	}{
		"unknown id":                  {api.ID, []int64{b, 999}, "ticket 999 does not exist"},
		"another repo":                {api.ID, []int64{elsewhere}, fmt.Sprintf("ticket %d is in another repo", elsewhere)},
		"dependant in another repo":   {web.ID, []int64{b}, fmt.Sprintf("ticket %d is in another repo", b)},
		"prerequisite without a repo": {api.ID, []int64{unassigned}, fmt.Sprintf("ticket %d is in another repo", unassigned)},
		"dependant without a repo":    {0, []int64{b}, "needs repo_id"},
		"non-positive id":             {api.ID, []int64{0}, "positive"},
		"too many ids":                {api.ID, tooMany, fmt.Sprintf("at most %d", domain.MaxDependencies)},
	}
	before := f.ticketCount()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.create(owner, name, tc.repoID, tc.dependsOn...)
			requireErrorKind(t, err, domain.KindInvalid)
			requireMessage(t, err, tc.want)
			if strings.Contains(err.Error(), "acme/") {
				t.Fatalf("the error names a repo: %v", err)
			}
			if got := f.ticketCount(); got != before {
				t.Fatalf("a refused create left %d tickets, want %d", got, before)
			}
		})
	}
}

func TestCreateAutonomousWithDependsOn(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	web := f.repo("acme/web")
	b := f.ticket("b", api.ID)
	elsewhere := f.ticket("elsewhere", web.ID)

	draft := autonomousDraft(api.ID)
	draft.DependsOn = []int64{b}
	detail, err := f.board.CreateAutonomous(f.ctx, delegate, draft)
	if err != nil || !slices.Equal(detail.WaitingOn, []int64{b}) {
		t.Fatalf("autonomous with a dependency: waiting %v, %v", detail.WaitingOn, err)
	}
	before := f.ticketCount()
	for _, bad := range []int64{999, elsewhere} {
		draft.DependsOn = []int64{bad}
		_, err := f.board.CreateAutonomous(f.ctx, delegate, draft)
		requireErrorKind(t, err, domain.KindInvalid)
		requireMessage(t, err, fmt.Sprintf("ticket %d", bad))
	}
	if got := f.ticketCount(); got != before {
		t.Fatalf("refused filings left %d tickets, want %d", got, before)
	}
}

func TestCreateRelatedRefusesDependsOn(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	b := f.ticket("b", api.ID)
	_, err := f.board.CreateRelated(f.ctx, owner, domain.TicketDraft{Kind: "story", Title: "a", RepoID: api.ID}, dependsOn, b)
	requireErrorKind(t, err, domain.KindInvalid)
}

func TestClaimSkipsTicketsWaitingOnUnmergedWork(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	b := f.buildingByOther("b", api.ID)

	if _, err := f.create(owner, "a in refine", api.ID, b); err != nil {
		t.Fatalf("create: %v", err)
	}
	f.requireNoClaim(agent, "refine, waiting on a ticket in build")
}

func TestClaimSkipsAReadyTicketWaitingOnOneInProgress(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	b := f.buildingByOther("b", api.ID)
	a := f.ticket("a", api.ID)
	f.toReady(a)
	f.mustLink(owner, a, b)
	f.requireNoClaim(agent, "ready, waiting on a ticket in build")
	if got := f.waitingOn(a); !slices.Equal(got, []int64{b}) {
		t.Fatalf("waiting_on %v, want [%d]", got, b)
	}
}

func TestClaimSkipsWaitingTicketsInDesignAndBuild(t *testing.T) {
	for _, stage := range []domain.Stage{domain.StageDesign, domain.StageBuild} {
		t.Run(string(stage), func(t *testing.T) {
			f := newFixture(t)
			api := f.repo("acme/api")
			b := f.buildingByOther("b", api.ID)
			a := f.ticket("a", api.ID)
			f.toReady(a)
			if stage == domain.StageDesign {
				f.designGated(api.ID)
				f.move(owner, a, domain.LaneInProgress, nil)
			} else {
				f.toBuild(a)
			}
			f.requireStage(a, stage)
			f.mustLink(owner, a, b)
			f.requireNoClaim(agent, string(stage)+", waiting on a ticket in build")
		})
	}
}

func TestMergeReportFreesDependantsAndNamesTheReporter(t *testing.T) {
	f := newFixture(t)
	b := f.inReview("b")
	repoID := f.detail(b).Ticket.RepoID
	created, err := f.create(owner, "a", repoID, b)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	a := created.Ticket.ID
	f.requireNoClaim(agent, "waiting on a ticket in review")

	if _, err := f.board.ReportPullRequest(f.ctx, other, b, app.PullRequestMerged, ""); err != nil {
		t.Fatalf("report merged: %v", err)
	}
	met := dependenciesMet(f.detail(a).Timeline)
	if len(met) != 1 {
		t.Fatalf("%d dependencies_met entries, want 1", len(met))
	}
	if met[0].Actor.Name != other.Name || met[0].Actor.Kind != domain.ActorAgent {
		t.Fatalf("release attributed to %+v, want the reporting agent %s", met[0].Actor, other.Name)
	}
	if got := fmt.Sprint(met[0].Data["ticket_id"]); got != fmt.Sprint(b) {
		t.Fatalf("release names ticket %s, want %d", got, b)
	}
	if len(f.waitingOn(a)) != 0 {
		t.Fatalf("still waiting on %v after the merge", f.waitingOn(a))
	}
	if work := f.mustClaim(agent, app.ClaimInput{}); work.Ticket.Ticket.ID != a {
		t.Fatalf("claimed #%d after the merge, want #%d", work.Ticket.Ticket.ID, a)
	}
}

func TestPersonMovingToMergedFreesDependants(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	b := f.ticket("b", api.ID)
	c := f.ticket("c", api.ID)
	created, err := f.create(owner, "a", api.ID, b, c)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	a := created.Ticket.ID

	f.move(owner, b, domain.LaneMerged, nil)
	if met := dependenciesMet(f.detail(a).Timeline); len(met) != 0 {
		t.Fatalf("released while still waiting on #%d: %+v", c, met)
	}
	if got := f.waitingOn(a); !slices.Equal(got, []int64{c}) {
		t.Fatalf("waiting_on %v, want [%d]", got, c)
	}

	f.move(owner, c, domain.LaneMerged, nil)
	met := dependenciesMet(f.detail(a).Timeline)
	if len(met) != 1 || met[0].Actor.Name != owner.Name || met[0].Actor.Kind != domain.ActorHuman {
		t.Fatalf("release entries %+v, want one by %s", met, owner.Name)
	}
	if got := fmt.Sprint(met[0].Data["ticket_id"]); got != fmt.Sprint(c) {
		t.Fatalf("release names ticket %s, want %d", got, c)
	}
	if work := f.mustClaim(agent, app.ClaimInput{}); work.Ticket.Ticket.ID != a {
		t.Fatalf("claimed #%d, want #%d", work.Ticket.Ticket.ID, a)
	}
}

func TestWaitingOnIsNeverNil(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	b := f.ticket("b", api.ID)
	created, err := f.create(owner, "a", api.ID, b)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := f.waitingOn(created.Ticket.ID); !slices.Equal(got, []int64{b}) {
		t.Fatalf("waiting_on %v, want [%d]", got, b)
	}
	if got := f.waitingOn(b); len(got) != 0 {
		t.Fatalf("a ticket without dependencies waits on %v", got)
	}
}

func TestDependencyCyclesAreRefusedWithThePath(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	a := f.ticket("a", api.ID)
	b := f.ticket("b", api.ID)
	c := f.ticket("c", api.ID)
	f.mustLink(owner, a, b)

	_, err := f.board.LinkTickets(f.ctx, owner, b, dependsOn, a)
	requireErrorKind(t, err, domain.KindConflict)
	requireMessage(t, err, fmt.Sprintf("#%d -> #%d -> #%d", b, a, b))

	f.mustLink(owner, b, c)
	_, err = f.board.LinkTickets(f.ctx, owner, c, dependsOn, a)
	requireErrorKind(t, err, domain.KindConflict)
	requireMessage(t, err, fmt.Sprintf("#%d -> #%d -> #%d -> #%d", c, a, b, c))

	_, err = f.board.LinkTickets(f.ctx, owner, a, dependsOn, a)
	requireErrorKind(t, err, domain.KindInvalid)
}

func TestConcurrentOppositeLinksCannotBothSucceed(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	a := f.ticket("a", api.ID)
	b := f.ticket("b", api.ID)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, pair := range [][2]int64{{a, b}, {b, a}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.board.LinkTickets(f.ctx, owner, pair[0], dependsOn, pair[1])
		}()
	}
	wg.Wait()
	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		requireErrorKind(t, err, domain.KindConflict)
		requireMessage(t, err, "cycle")
	}
	if succeeded != 1 {
		t.Fatalf("%d of the two opposite links succeeded (%v), want exactly 1", succeeded, errs)
	}
}

func TestWhoMayAddADependency(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	held := f.claimed("held by agent", api.ID)
	b := f.ticket("b", api.ID)
	f.mustLink(agent, held, b)

	filed, err := f.create(agent, "filed by agent", api.ID)
	if err != nil {
		t.Fatalf("agent files a ticket: %v", err)
	}
	f.mustLink(agent, filed.Ticket.ID, b)

	byOwner := f.ticket("filed by owner", api.ID)
	_, err = f.board.LinkTickets(f.ctx, other, byOwner, dependsOn, b)
	requireErrorKind(t, err, domain.KindForbidden)

	// Holding only the prerequisite's lease is not enough.
	_, err = f.board.LinkTickets(f.ctx, agent, byOwner, dependsOn, held)
	requireErrorKind(t, err, domain.KindForbidden)
	// The refusal comes before the other ticket is looked up.
	_, err = f.board.LinkTickets(f.ctx, other, byOwner, dependsOn, 999)
	requireErrorKind(t, err, domain.KindForbidden)

	// A person whose name matches the token's is not the token.
	namesake := domain.Actor{Kind: domain.ActorHuman, ID: 9, Name: delegate.Name}
	byNamesake, err := f.create(namesake, "filed by a person named like the token", api.ID)
	if err != nil {
		t.Fatalf("namesake files: %v", err)
	}
	_, err = f.board.LinkTickets(f.ctx, delegate, byNamesake.Ticket.ID, dependsOn, b)
	requireErrorKind(t, err, domain.KindForbidden)

	f.mustLink(owner, byOwner, b)
}

func TestTheFilingAgentManagesItsDependenciesWithoutALease(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	b := f.ticket("b", api.ID)
	a := f.mustCreateAutonomous(api.ID)

	f.mustLink(delegate, a, b)
	if _, err := f.board.UnlinkTickets(f.ctx, delegate, a, dependsOn, b); err != nil {
		t.Fatalf("remove from the dependant's end: %v", err)
	}
	f.mustLink(delegate, a, b)
	if _, err := f.board.UnlinkTickets(f.ctx, delegate, b, dependsOn, a); err != nil {
		t.Fatalf("remove from the prerequisite's end: %v", err)
	}
	if len(f.detail(a).Related) != 0 {
		t.Fatalf("relations left: %+v", f.detail(a).Related)
	}
}

func TestAgentsRemoveOnlyDependenciesTheyAdded(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	b := f.ticket("b", api.ID)
	byDelegate := f.mustCreateAutonomous(api.ID)
	byOwner := f.mustCreateAutonomous(api.ID)
	f.mustLink(delegate, byDelegate, b)
	f.mustLink(owner, byOwner, b)

	refusals := []struct {
		name     string
		actor    domain.Actor
		from, to int64
	}{
		{"another token's link, dependant end", other, byDelegate, b},
		{"another token's link, prerequisite end", other, b, byDelegate},
		{"a person's link, dependant end", delegate, byOwner, b},
		{"a person's link, prerequisite end", delegate, b, byOwner},
		{"a person's link, agent with the same id", agent, byOwner, b},
	}
	for _, refusal := range refusals {
		t.Run(refusal.name, func(t *testing.T) {
			_, err := f.board.UnlinkTickets(f.ctx, refusal.actor, refusal.from, dependsOn, refusal.to)
			requireErrorKind(t, err, domain.KindForbidden)
		})
	}
	if _, err := f.board.UnlinkTickets(f.ctx, delegate, byOwner, string(domain.RelationCausedBy), b); domain.KindOf(err) != domain.KindForbidden {
		t.Fatalf("agent removing another kind of link: %v", err)
	}
	if _, err := f.board.UnlinkTickets(f.ctx, delegate, byDelegate, dependsOn, 999); domain.KindOf(err) != domain.KindNotFound {
		t.Fatalf("removing a link that does not exist: %v", err)
	}
	for _, id := range []int64{byDelegate, byOwner} {
		if _, err := f.board.UnlinkTickets(f.ctx, owner, b, dependsOn, id); err != nil {
			t.Fatalf("a person removes the link on #%d: %v", id, err)
		}
	}
}

func TestRepoChangesCannotSplitDependencies(t *testing.T) {
	f := newFixture(t)
	api := f.repo("acme/api")
	web := f.repo("acme/web")
	a := f.ticket("a", api.ID)
	b := f.ticket("b", api.ID)
	f.mustLink(owner, a, b)

	none, webID := int64(0), web.ID
	for _, id := range []int64{a, b} {
		for name, repoID := range map[string]*int64{"another repo": &webID, "no repo": &none} {
			_, err := f.board.Edit(f.ctx, owner, id, domain.TicketEdit{RepoID: repoID})
			requireErrorKind(t, err, domain.KindConflict)
			if got := f.detail(id).Ticket.RepoID; got != api.ID {
				t.Fatalf("%s on #%d: repo changed to %d despite the refusal", name, id, got)
			}
		}
	}
	if len(f.detail(a).Related) != 1 {
		t.Fatal("a refused repo change touched the dependency")
	}

	free := f.ticket("free", api.ID)
	if _, err := f.board.Edit(f.ctx, owner, free, domain.TicketEdit{RepoID: &webID}); err != nil {
		t.Fatalf("repo change without dependencies: %v", err)
	}
}
