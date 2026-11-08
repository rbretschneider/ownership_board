package stats_test

import (
	"github.com/rbretschneider/ownership_board/internal/domain"
	"testing"
)

func TestDeploymentKeepsOriginalCompletionTime(t *testing.T) {
	b := newBoard(t)
	b.ticket(1, false, 60*day).to(domain.StagePR, 50*day).to(domain.StageDone, 40*day).to(domain.StageDeployed, hour)
	b.ticket(2, false, 3*day).to(domain.StagePR, 2*day).to(domain.StageDeployed, hour)
	// Migrated tickets keep their old Done history.
	b.ticket(3, false, 60*day).to(domain.StageDone, 40*day)
	b.input.Tickets[2].Stage = domain.StageDeployed
	// Reopening followed by completion should use the new completion time.
	b.ticket(4, false, 60*day).to(domain.StageDone, 40*day).to(domain.StageBuild, day).to(domain.StageDeployed, hour)
	r := b.report(7)
	if r.All.Merged != 2 || r.All.MergedRecent != 2 || r.All.LeadTime.Count != 2 {
		t.Fatalf("completion metrics: %+v", r.All)
	}
	for _, w := range r.All.WIP {
		if w.Count != 0 || w.Stage.IsCompleted() {
			t.Fatalf("WIP: %+v", r.All.WIP)
		}
	}
	for _, s := range r.All.StageTimes {
		if s.Stage.IsCompleted() {
			t.Fatalf("terminal stage timing: %+v", s)
		}
	}
	if len(r.Repos) != 1 || r.Repos[0].Owner.Open != 0 || r.Repos[0].Owner.Merged != 2 {
		t.Fatalf("repo totals: %+v", r.Repos)
	}
	if all := b.report(90); all.All.Merged != 4 {
		t.Fatalf("double counted deployment: %+v", all.All)
	}
}
