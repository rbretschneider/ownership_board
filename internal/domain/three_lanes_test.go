package domain_test

import (
	"github.com/rbretschneider/ownership_board/internal/domain"
	"reflect"
	"strings"
	"testing"
)

func TestThreeLaneCatalog(t *testing.T) {
	var ids []domain.Lane
	var labels []string
	for _, lane := range domain.Lanes() {
		ids = append(ids, lane.ID)
		labels = append(labels, lane.Label)
		if strings.Contains(strings.ToLower(lane.Description), "delivery") {
			t.Errorf("description: %s", lane.Description)
		}
	}
	if !reflect.DeepEqual(ids, []domain.Lane{domain.LaneTodo, domain.LaneInProgress, domain.LaneMerged}) {
		t.Fatalf("lanes: %v", ids)
	}
	if !reflect.DeepEqual(labels, []string{"Todo", "In progress", "Merged"}) {
		t.Fatalf("labels: %v", labels)
	}
	if _, err := domain.ParseLane("review"); err == nil {
		t.Fatal("retired lane accepted")
	}
}

func TestPRStageMetadata(t *testing.T) {
	if domain.StagePR.Lane() != domain.LaneInProgress || !domain.StagePR.IsOwnerReview() {
		t.Fatal("PR metadata")
	}
	for stage, label := range map[domain.Stage]string{domain.StagePR: "PR open", domain.StageDesign: "Designing", domain.StageDone: "Merged"} {
		if stage.Label() != label {
			t.Errorf("%s label = %s", stage, stage.Label())
		}
	}
	ticket := mustTicket(t)
	ticket.MoveToStage(domain.StagePR, 1, now)
	if !ticket.NeedsOwner(0) {
		t.Fatal("PR does not need owner")
	}
}
