package domain_test

import (
	"strings"
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

func TestEveryStageBelongsToALane(t *testing.T) {
	lanes := map[domain.Lane]bool{}
	for _, lane := range domain.Lanes() {
		lanes[lane.ID] = true
	}
	if len(lanes) != 4 {
		t.Fatalf("got %d lanes, want 4", len(lanes))
	}
	for _, stage := range domain.Stages() {
		if !lanes[stage.Lane] {
			t.Errorf("stage %s has unknown lane %q", stage.ID, stage.Lane)
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

func TestHealthCheckIntervalBounds(t *testing.T) {
	repo, err := domain.NewManualRepo(domain.ManualRepoDraft{FullName: "acme/api"}, now)
	if err != nil || repo.HealthCheckEvery != domain.DefaultHealthCheckEvery {
		t.Fatalf("new repo interval %d, err %v", repo.HealthCheckEvery, err)
	}
	if domain.DefaultHealthCheckEvery != 8 {
		t.Fatalf("new repo interval %d, err %v", repo.HealthCheckEvery, err)
	}
	for _, tc := range []struct {
		value int
		ok    bool
	}{{-1, false}, {0, true}, {8, true}, {50, true}, {51, false}, {1 << 40, false}} {
		r := repo
		v := tc.value
		err := r.Restyle(domain.RepoStyle{HealthCheckEvery: &v})
		if tc.ok && (err != nil || r.HealthCheckEvery != v) {
			t.Errorf("%d: got %d, err %v", v, r.HealthCheckEvery, err)
		}
		if !tc.ok {
			if domain.KindOf(err) != domain.KindInvalid || !strings.Contains(err.Error(), "between 0 and 50") {
				t.Errorf("%d: error %v", v, err)
			}
			if r.HealthCheckEvery != 8 {
				t.Errorf("%d: value changed to %d", v, r.HealthCheckEvery)
			}
		}
	}
	untouched := repo
	if err := untouched.Restyle(domain.RepoStyle{}); err != nil || untouched.HealthCheckEvery != 8 {
		t.Errorf("nil changed the interval: %d, %v", untouched.HealthCheckEvery, err)
	}
}
