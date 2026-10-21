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
