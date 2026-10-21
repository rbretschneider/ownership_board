package domain_test

import (
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func ptr(v float64) *float64 { return &v }

func TestRankBetween(t *testing.T) {
	cases := []struct {
		name       string
		prev, next *float64
		want       float64
	}{
		{"empty lane", nil, nil, domain.RankStep},
		{"top", nil, ptr(100), 100 - domain.RankStep},
		{"bottom", ptr(100), nil, 100 + domain.RankStep},
		{"between", ptr(100), ptr(200), 150},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, needsRenumber := domain.RankBetween(tc.prev, tc.next)
			if needsRenumber || got != tc.want {
				t.Fatalf("got %v renumber=%v, want %v", got, needsRenumber, tc.want)
			}
		})
	}
}

func TestRankBetweenAsksForRenumberWhenNeighboursTouch(t *testing.T) {
	if _, needsRenumber := domain.RankBetween(ptr(1), ptr(1.0000000001)); !needsRenumber {
		t.Fatal("expected a renumber request")
	}
}

func TestLaneOwners(t *testing.T) {
	want := map[domain.Lane]domain.Owner{
		domain.LaneNew: domain.OwnerAgent, domain.LaneAnswers: domain.OwnerHuman, domain.LaneReady: domain.OwnerAgent,
		domain.LaneInProgress: domain.OwnerAgent, domain.LaneReview: domain.OwnerHuman, domain.LaneDone: domain.OwnerNobody,
	}
	lanes := domain.Lanes()
	if len(lanes) != len(want) {
		t.Fatalf("got %d lanes, want %d", len(lanes), len(want))
	}
	for _, lane := range lanes {
		if lane.ID.Owner() != want[lane.ID] {
			t.Errorf("%s owner = %s, want %s", lane.ID, lane.ID.Owner(), want[lane.ID])
		}
	}
}

func TestRepoNamesMustLookLikeOwnerSlashName(t *testing.T) {
	for _, name := range []string{"api", "acme/", "/api", "acme api/x", "acme/../etc", "./x"} {
		if _, err := domain.NewManualRepo(domain.ManualRepoDraft{FullName: name}, now); domain.KindOf(err) != domain.KindInvalid {
			t.Errorf("%q accepted", name)
		}
	}
	repo, err := domain.NewManualRepo(domain.ManualRepoDraft{FullName: "group/sub/repo.git"}, now)
	if err != nil || repo.Name() != "repo.git" {
		t.Fatalf("nested group repo: %v name %q", err, repo.Name())
	}
}

func TestRepoRefreshKeepsChosenColor(t *testing.T) {
	repo, err := domain.NewSyncedRepo(domain.RepoListing{Provider: domain.ProviderGitHub, FullName: "acme/api"}, now)
	if err != nil {
		t.Fatalf("NewSyncedRepo: %v", err)
	}
	color := "#AABBCC"
	if err := repo.Restyle(domain.RepoStyle{Color: &color}); err != nil {
		t.Fatalf("Restyle: %v", err)
	}
	if err := repo.Refresh(domain.RepoListing{FullName: "acme/api", IsArchived: true}, now); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if repo.Color != "#aabbcc" || !repo.IsArchived {
		t.Fatalf("color %q archived %v", repo.Color, repo.IsArchived)
	}
}
