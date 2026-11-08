package app_test

import (
	"context"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// scriptedSource lists whatever repos it currently holds.
type scriptedSource struct {
	names []string
}

func (s *scriptedSource) Provider() domain.Provider { return domain.ProviderForgejo }
func (s *scriptedSource) Label() string             { return "Forgejo" }
func (s *scriptedSource) IsConfigured() bool        { return true }

func (s *scriptedSource) List(context.Context) ([]domain.RepoListing, error) {
	listings := make([]domain.RepoListing, 0, len(s.names))
	for _, name := range s.names {
		listings = append(listings, domain.RepoListing{Provider: domain.ProviderForgejo, FullName: name})
	}
	return listings, nil
}

func (f *fixture) repoNamed(name string) domain.Repo {
	f.t.Helper()
	repos, err := f.repos.List(f.ctx)
	if err != nil {
		f.t.Fatalf("list repos: %v", err)
	}
	for _, repo := range repos {
		if repo.FullName == name {
			return repo
		}
	}
	f.t.Fatalf("repo %s is not on the board", name)
	return domain.Repo{}
}

func TestRepoDroppedByProviderIsKeptAndMarkedMissing(t *testing.T) {
	source := &scriptedSource{names: []string{"me/kept", "me/dropped"}}
	f := newFixture(t, source)
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	source.names = []string{"me/kept"}
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if !f.repoNamed("me/dropped").IsMissing() || f.repoNamed("me/kept").IsMissing() {
		t.Fatal("missing flags are wrong after the repo dropped out")
	}
	source.names = []string{"me/kept", "me/dropped"}
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatalf("third sync: %v", err)
	}
	if f.repoNamed("me/dropped").IsMissing() {
		t.Fatal("a repo that came back is still marked missing")
	}
}

func TestOneInvalidListingDoesNotBlockTheRest(t *testing.T) {
	source := &scriptedSource{names: []string{"not a valid name", "me/fine"}}
	f := newFixture(t, source)
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}
	f.repoNamed("me/fine")
}

func TestRepoSyncLeavesCheckpointsAlone(t *testing.T) {
	source := &scriptedSource{names: []string{"me/app"}}
	f := newFixture(t, source)
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	on, off := true, false
	repo := f.repoNamed("me/app")
	if _, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{RequireSpecApproval: &off, RequireDesignApproval: &on}); err != nil {
		t.Fatalf("Restyle: %v", err)
	}
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if got := f.repoNamed("me/app"); got.RequireSpecApproval || !got.RequireDesignApproval {
		t.Fatalf("sync changed the checkpoints: spec %v design %v", got.RequireSpecApproval, got.RequireDesignApproval)
	}
}

func TestOnlyPeopleChangeTheHealthCheckInterval(t *testing.T) {
	source := &scriptedSource{names: []string{"me/app"}}
	f := newFixture(t, source)
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}
	repo := f.repoNamed("me/app")
	if repo.HealthCheckEvery != 8 {
		t.Fatalf("default interval %d, want 8", repo.HealthCheckEvery)
	}
	every := 12
	if _, err := f.repos.Restyle(f.ctx, agent, repo.ID, domain.RepoStyle{HealthCheckEvery: &every}); domain.KindOf(err) != domain.KindForbidden {
		t.Fatalf("agent restyle error: %v", err)
	}
	if got := f.repoNamed("me/app"); got.HealthCheckEvery != 8 {
		t.Fatalf("agent changed the interval to %d", got.HealthCheckEvery)
	}
	if _, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{HealthCheckEvery: &every}); err != nil {
		t.Fatalf("owner restyle: %v", err)
	}
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if got := f.repoNamed("me/app"); got.HealthCheckEvery != 12 {
		t.Fatalf("interval %d after owner change and sync, want 12", got.HealthCheckEvery)
	}
}

func TestRepoBuildTargetsSurviveProviderChanges(t *testing.T) {
	source := &scriptedSource{names: []string{"example/app"}}
	f := newFixture(t, source)
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatal(err)
	}
	repo := f.repoNamed("example/app")
	targets := []domain.RepoBuildTarget{{OS: "linux", Preset: "cmake"}}
	if _, err := f.repos.Restyle(f.ctx, agent, repo.ID, domain.RepoStyle{BuildTargets: &targets}); err == nil {
		t.Fatal("agent edit")
	}
	if _, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{BuildTargets: &targets}); err != nil {
		t.Fatal(err)
	}
	for _, names := range [][]string{{"example/app"}, {}, {"example/app"}} {
		source.names = names
		if err := f.repos.Sync(f.ctx); err != nil {
			t.Fatal(err)
		}
		if got := f.repoNamed("example/app"); len(got.BuildTargets) != 1 || got.BuildTargets[0].Preset != "cmake" {
			t.Fatal(got)
		}
	}
	empty := []domain.RepoBuildTarget{}
	got, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{BuildTargets: &empty})
	if err != nil || len(got.BuildTargets) != 0 {
		t.Fatal(got, err)
	}
}

func TestRepoBuildTargetsCombinedEditRollback(t *testing.T) {
	f := newFixture(t)
	repo := f.repo("example/app")
	for _, trigger := range []string{
		`CREATE TRIGGER fail_repo_edit BEFORE UPDATE OF build_targets ON repos BEGIN SELECT RAISE(ABORT,'refused'); END`,
		`CREATE TRIGGER fail_repo_edit BEFORE UPDATE OF health_check_every ON repos BEGIN SELECT RAISE(ABORT,'refused'); END`,
	} {
		if _, err := f.store.SQL().Exec(trigger); err != nil {
			t.Fatal(err)
		}
		targets := []domain.RepoBuildTarget{{OS: "windows", Preset: "dotnet"}}
		description := "Changed"
		interval := 12
		if _, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{BuildTargets: &targets, Description: &description, HealthCheckEvery: &interval}); err == nil {
			t.Fatal("expected failure")
		}
		got := f.repoNamed("example/app")
		if len(got.BuildTargets) != 0 || got.Description != repo.Description || got.HealthCheckEvery != repo.HealthCheckEvery {
			t.Fatal("partial edit", got)
		}
		if _, err := f.store.SQL().Exec(`DROP TRIGGER fail_repo_edit`); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRepoOverlapIgnoreEditsAndSync(t *testing.T) {
	source := &scriptedSource{names: []string{"example/app"}}
	f := newFixture(t, source)
	if err := f.repos.Sync(f.ctx); err != nil {
		t.Fatal(err)
	}
	repo := f.repoNamed("example/app")
	patterns := []string{" src/*/Catalog.cpp "}
	every := 3
	style := domain.RepoStyle{OverlapIgnore: &patterns, HealthCheckEvery: &every}
	if _, err := f.repos.Restyle(f.ctx, agent, repo.ID, style); domain.KindOf(err) != domain.KindForbidden {
		t.Fatal(err)
	}
	saved, err := f.repos.Restyle(f.ctx, owner, repo.ID, style)
	if err != nil || saved.OverlapIgnore[0] != "src/*/Catalog.cpp" || saved.HealthCheckEvery != 3 {
		t.Fatal(saved, err)
	}
	for _, names := range [][]string{{"example/app"}, {}, {"example/app"}} {
		source.names = names
		if err := f.repos.Sync(f.ctx); err != nil {
			t.Fatal(err)
		}
		if got := f.repoNamed("example/app"); len(got.OverlapIgnore) != 1 || got.OverlapIgnore[0] != "src/*/Catalog.cpp" {
			t.Fatal(got)
		}
	}
	bad := []string{"../file.go"}
	description := "changed"
	every = 4
	if _, err := f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{Description: &description, OverlapIgnore: &bad, HealthCheckEvery: &every}); err == nil {
		t.Fatal("invalid update accepted")
	}
	if got := f.repoNamed("example/app"); got.Description == description || got.HealthCheckEvery != 3 || len(got.OverlapIgnore) != 1 {
		t.Fatal(got)
	}
	patterns = []string{}
	saved, err = f.repos.Restyle(f.ctx, owner, repo.ID, domain.RepoStyle{OverlapIgnore: &patterns})
	if err != nil || saved.OverlapIgnore == nil || len(saved.OverlapIgnore) != 0 {
		t.Fatal(saved, err)
	}
}
