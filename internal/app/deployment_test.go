package app_test

import (
	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"strings"
	"testing"
)

func TestHumanDeploymentHistoryAndReorder(t *testing.T) {
	f := newFixture(t)
	id := f.ticket("release", 0)
	f.move(owner, id, domain.Lane("deployed"), nil)
	detail := f.detail(id)
	f.requireStage(id, domain.Stage("deployed"))
	var found bool
	for _, entry := range detail.Timeline {
		if entry.Type == domain.TimelineStage && entry.Data["to"] == "deployed" {
			found = true
			if entry.Body != "Marked deployed by "+owner.Name || (entry.Actor.Kind != owner.Kind || entry.Actor.Name != owner.Name) {
				t.Fatalf("attribution: %+v", entry)
			}
		}
	}
	if !found {
		t.Fatal("missing deployment history")
	}
	f.move(owner, id, domain.Lane("deployed"), nil)
	if len(f.detail(id).Timeline) != len(detail.Timeline) {
		t.Fatal("reordering added history")
	}
	f.move(owner, id, domain.LaneMerged, nil)
	f.requireStage(id, domain.StageDone)
}

func TestDefaultMergedReportDeploys(t *testing.T) {
	f := newFixture(t)
	id := f.inReview("release")
	detail, err := f.board.ReportPullRequest(f.ctx, agent, id, app.PullRequestMerged, "")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Ticket.Stage != domain.Stage("deployed") || detail.Ticket.Lane != domain.Lane("deployed") {
		t.Fatalf("completion: %+v", detail.Ticket)
	}
	notifications := 0
	for _, event := range f.events.events {
		if event.TicketID == id && event.Stage == domain.StageDeployed {
			if note, wanted := policy().Evaluate(event); wanted {
				notifications++
				if note.Priority != app.PriorityLow {
					t.Fatal("completion is not quiet")
				}
			}
		}
	}
	if notifications != 1 {
		t.Fatalf("completion notifications: %d", notifications)
	}
	for _, entry := range detail.Timeline {
		if entry.Body == "Marked deployed by "+agent.Name {
			t.Fatal("automatic deployment claims human attribution")
		}
	}
}

func TestMergedReportRouting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   domain.DeployMode
		noRepo bool
		want   domain.Stage
	}{
		{"off", domain.DeployModeOff, false, domain.StageDeployed},
		{"agent", domain.DeployModeAgent, false, domain.StageDone},
		{"unassigned", domain.DeployModeOff, true, domain.StageDeployed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			var repoID int64
			if !tc.noRepo {
				repo := f.repo("acme/api")
				repoID = repo.ID
				if _, err := f.repos.Restyle(f.ctx, owner, repoID, domain.RepoStyle{DeployMode: &tc.mode}); err != nil {
					t.Fatal(err)
				}
			}
			id := f.ticket("release", repoID)
			if tc.noRepo {
				f.move(owner, id, domain.LaneInProgress, nil)
				f.mustClaim(agent, app.ClaimInput{})
				detail, err := f.board.SaveDesign(f.ctx, agent, id, app.SaveTextInput{Text: "Plan.", IsSubmit: true})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.board.Approve(f.ctx, owner, id, app.ReviewInput{Checkpoint: "design", Revision: detail.Ticket.DesignRevision}); err != nil {
					t.Fatal(err)
				}
			} else {
				f.toBuild(id)
			}
			f.mustClaim(agent, app.ClaimInput{})
			pr := "https://example.com/pr/1"
			if _, err := f.board.Edit(f.ctx, owner, id, domain.TicketEdit{PRURL: &pr}); err != nil {
				t.Fatal(err)
			}
			for _, lane := range []domain.Lane{domain.LaneMerged, domain.LaneDeployed} {
				_, err := f.board.Move(f.ctx, agent, id, app.MoveInput{Lane: string(lane)})
				requireErrorKind(t, err, domain.KindForbidden)
			}
			f.move(agent, id, domain.LaneInProgress, nil)
			if _, err := f.board.ReportPullRequest(f.ctx, agent, id, app.PullRequestMerged, ""); err != nil {
				t.Fatal(err)
			}
			f.requireStage(id, tc.want)
			brief, err := f.agents.Brief(f.ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			label := tc.want.LabelForRepo(f.detail(id).Repo)
			if !strings.Contains(brief, "stage "+label+" ("+string(tc.want)+")") {
				t.Fatalf("stage wording: %s", brief)
			}
		})
	}
}

func TestDeploymentPolicyServiceAuthorizationAndPersistence(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	mode := domain.DeployModeAgent
	_, err := f.repos.Restyle(f.ctx, agent, repo.ID, domain.RepoStyle{DeployMode: &mode})
	requireErrorKind(t, err, domain.KindForbidden)
	if _, err = f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{DeployMode: &mode}); err != nil {
		t.Fatal(err)
	}
	invalid := domain.DeployMode("workflow")
	description := "changed"
	_, err = f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{DeployMode: &invalid, Description: &description})
	requireErrorKind(t, err, domain.KindInvalid)
	repos, err := f.repos.List(f.ctx)
	if err != nil || len(repos) != 1 || repos[0].DeployMode != mode || repos[0].Description != "" {
		t.Fatalf("persisted: %+v %v", repos, err)
	}
	// Provider-owned updates must not write a stale policy snapshot.
	err = f.store.Write(f.ctx, func(r app.Repos) error {
		repo.Description = "refreshed"
		repo.DeployMode = domain.DeployModeOff
		return r.CodeRepos().Update(f.ctx, repo)
	})
	if err != nil {
		t.Fatal(err)
	}
	repos, err = f.repos.List(f.ctx)
	if err != nil || repos[0].DeployMode != mode {
		t.Fatalf("refresh changed policy: %+v %v", repos, err)
	}
}

func TestCompletedDependenciesReleaseOnlyOnce(t *testing.T) {
	for _, lane := range []domain.Lane{domain.LaneMerged, domain.LaneDeployed} {
		t.Run(string(lane), func(t *testing.T) {
			f := newFixture(t)
			repo := f.repo("acme/api")
			first := f.ticket("first", repo.ID)
			second := f.ticket("second", repo.ID)
			dependent, err := f.create(owner, "dependent", repo.ID, first, second)
			if err != nil {
				t.Fatal(err)
			}
			f.move(owner, first, lane, nil)
			if len(f.waitingOn(dependent.Ticket.ID)) != 1 {
				t.Fatal("one remaining dependency expected")
			}
			f.move(owner, second, domain.LaneDeployed, nil)
			if len(f.waitingOn(dependent.Ticket.ID)) != 0 || len(f.detail(dependent.Ticket.ID).WaitingOn) != 0 {
				t.Fatal("deployed prerequisite still waiting")
			}
			met := dependenciesMet(f.detail(dependent.Ticket.ID).Timeline)
			if len(met) != 1 || (met[0].Actor.Kind != owner.Kind || met[0].Actor.Name != owner.Name) || met[0].Data["merged_by_kind"] != string(owner.Kind) {
				t.Fatalf("release: %+v", met)
			}
			f.move(owner, first, domain.LaneDeployed, nil)
			if len(dependenciesMet(f.detail(dependent.Ticket.ID).Timeline)) != 1 {
				t.Fatal("duplicate release")
			}
			work := f.mustClaim(agent, app.ClaimInput{})
			if work.Ticket.Ticket.ID != dependent.Ticket.ID {
				t.Fatalf("claimed %d", work.Ticket.Ticket.ID)
			}
		})
	}
}

func TestDirectPRDeploymentReleasesAsAgent(t *testing.T) {
	f := newFixture(t)
	id := f.inReview("first")
	d, err := f.create(owner, "dependent", f.detail(id).Ticket.RepoID, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.board.ReportPullRequest(f.ctx, agent, id, app.PullRequestMerged, ""); err != nil {
		t.Fatal(err)
	}
	met := dependenciesMet(f.detail(d.Ticket.ID).Timeline)
	if len(met) != 1 || (met[0].Actor.Kind != agent.Kind || met[0].Actor.Name != agent.Name) {
		t.Fatalf("release: %+v", met)
	}
	if f.mustClaim(agent, app.ClaimInput{}).Ticket.Ticket.ID != d.Ticket.ID {
		t.Fatal("dependent not claimable")
	}
}

func TestDeploymentHealthChecksAndMergeSequence(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	f.healthInterval(repo.ID, 1)
	id := f.ticket("first", repo.ID)
	f.move(owner, id, domain.LaneDeployed, nil)
	seq := f.detail(id).Ticket.HealthCheckMergeSequence
	checks := f.checks(repo.ID)
	if seq != 1 || len(checks) != 1 {
		t.Fatalf("sequence %d, checks %+v", seq, checks)
	}
	f.move(owner, id, domain.LaneMerged, nil)
	f.move(owner, id, domain.LaneDeployed, nil)
	if f.detail(id).Ticket.HealthCheckMergeSequence != seq || len(f.checks(repo.ID)) != 1 {
		t.Fatal("terminal move repeated completion")
	}
	next := f.ticket("second", repo.ID)
	f.move(owner, next, domain.LaneDeployed, nil)
	if len(f.checks(repo.ID)) != 1 {
		t.Fatal("open check guard failed")
	}
	f.move(owner, checks[0].ID, domain.LaneDeployed, nil)
	if len(f.checks(repo.ID)) != 2 {
		t.Fatal("completed check still counted open")
	}
}

func TestDeployedNotificationIsQuiet(t *testing.T) {
	event := app.Event{Type: domain.TimelineStage, Stage: domain.StageDeployed, Actor: agent, TicketID: 1}
	note, wanted := policy().Evaluate(event)
	if !wanted || note.Priority != app.PriorityLow || note.Message != "The ticket reached Deployed." {
		t.Fatalf("notification: %+v %v", note, wanted)
	}
	event.Actor = owner
	if _, wanted := policy().Evaluate(event); wanted {
		t.Fatal("human action notified")
	}
}

func TestDeploymentPolicyChangePreservesMergedTickets(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("acme/api")
	mode := domain.DeployModeAgent
	if _, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{DeployMode: &mode}); err != nil {
		t.Fatal(err)
	}
	id := f.ticket("release", repo.ID)
	f.move(owner, id, domain.LaneMerged, nil)
	mode = domain.DeployModeOff
	if _, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{DeployMode: &mode}); err != nil {
		t.Fatal(err)
	}
	f.requireStage(id, domain.StageDone)
}
