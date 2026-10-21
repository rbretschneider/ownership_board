package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// syncTimeout bounds one provider's listing so a hung API cannot stall a sync.
const syncTimeout = 60 * time.Second

// RepoService manages the repos tickets target and syncs them from hosting
// providers.
type RepoService struct {
	store   Store
	clock   Clock
	sources []RepoSource
	logger  *slog.Logger
}

// NewRepoService wires the service to its dependencies. sources lists every
// supported provider, configured or not, so settings can show what is off.
func NewRepoService(store Store, clock Clock, sources []RepoSource, logger *slog.Logger) *RepoService {
	return &RepoService{store: store, clock: clock, sources: sources, logger: logger}
}

// List returns every repo by full name.
func (s *RepoService) List(ctx context.Context) ([]domain.Repo, error) {
	var list []domain.Repo
	err := s.store.Read(ctx, func(repos Repos) error {
		var err error
		list, err = repos.CodeRepos().List(ctx)
		return err
	})
	return list, err
}

// CreateManual adds a repo no provider lists.
func (s *RepoService) CreateManual(ctx context.Context, actor domain.Actor, draft domain.ManualRepoDraft) (domain.Repo, error) {
	if !actor.IsHuman() {
		return domain.Repo{}, domain.Forbiddenf("only a person can add repos")
	}
	repo, err := domain.NewManualRepo(draft, s.clock.Now())
	if err != nil {
		return domain.Repo{}, err
	}
	err = s.store.Write(ctx, func(repos Repos) error {
		return repos.CodeRepos().Insert(ctx, &repo)
	})
	return repo, err
}

// Restyle changes a repo's color or description.
func (s *RepoService) Restyle(ctx context.Context, actor domain.Actor, id int64, style domain.RepoStyle) (domain.Repo, error) {
	if !actor.IsHuman() {
		return domain.Repo{}, domain.Forbiddenf("only a person can edit repos")
	}
	var repo domain.Repo
	err := s.store.Write(ctx, func(repos Repos) error {
		var err error
		if repo, err = repos.CodeRepos().Get(ctx, id); err != nil {
			return err
		}
		if err := repo.Restyle(style); err != nil {
			return err
		}
		return repos.CodeRepos().Update(ctx, repo)
	})
	return repo, err
}

// Sources reports each provider's configuration and last sync.
func (s *RepoService) Sources(ctx context.Context) ([]SourceStatus, error) {
	var statuses []SourceStatus
	err := s.store.Read(ctx, func(repos Repos) error {
		outcomes, err := repos.SourceStatuses().All(ctx)
		if err != nil {
			return err
		}
		all, err := repos.CodeRepos().List(ctx)
		statuses = s.describe(outcomes, countByProvider(all))
		return err
	})
	return statuses, err
}

// Sync refreshes repos from every configured provider. A failing provider is
// recorded on its status and does not stop the others. Repos a provider no
// longer lists are kept, because tickets may still point at them.
func (s *RepoService) Sync(ctx context.Context) ([]SourceStatus, error) {
	for _, source := range s.sources {
		if !source.IsConfigured() {
			continue
		}
		outcome := SyncOutcome{At: s.clock.Now()}
		if err := s.syncSource(ctx, source); err != nil {
			outcome.Error = err.Error()
			s.logger.Warn("repo sync failed", "provider", source.Provider(), "error", err)
		}
		if err := s.saveOutcome(ctx, source.Provider(), outcome); err != nil {
			return nil, err
		}
	}
	return s.Sources(ctx)
}

func (s *RepoService) syncSource(ctx context.Context, source RepoSource) error {
	listCtx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()
	listings, err := source.List(listCtx)
	if err != nil {
		return err
	}
	return s.store.Write(ctx, func(repos Repos) error {
		existing, err := repos.CodeRepos().ByProvider(ctx, source.Provider())
		if err != nil {
			return err
		}
		return s.upsert(ctx, repos.CodeRepos(), existing, listings)
	})
}

func (s *RepoService) upsert(ctx context.Context, store RepoRepository, existing []domain.Repo, listings []domain.RepoListing) error {
	byName := make(map[string]domain.Repo, len(existing))
	for _, repo := range existing {
		byName[repo.FullName] = repo
	}
	now := s.clock.Now()
	for _, listing := range listings {
		if err := s.upsertOne(ctx, store, byName, listing, now); err != nil {
			return fmt.Errorf("repo %s: %w", listing.FullName, err)
		}
	}
	return nil
}

func (s *RepoService) upsertOne(ctx context.Context, store RepoRepository, byName map[string]domain.Repo, listing domain.RepoListing, now time.Time) error {
	repo, isKnown := byName[listing.FullName]
	if !isKnown {
		created, err := domain.NewSyncedRepo(listing, now)
		if err != nil {
			return err
		}
		return store.Insert(ctx, &created)
	}
	if err := repo.Refresh(listing, now); err != nil {
		return err
	}
	return store.Update(ctx, repo)
}

func (s *RepoService) saveOutcome(ctx context.Context, provider domain.Provider, outcome SyncOutcome) error {
	return s.store.Write(ctx, func(repos Repos) error {
		return repos.SourceStatuses().Save(ctx, provider, outcome)
	})
}

func (s *RepoService) describe(outcomes map[domain.Provider]SyncOutcome, counts map[domain.Provider]int) []SourceStatus {
	statuses := make([]SourceStatus, 0, len(s.sources))
	for _, source := range s.sources {
		status := SourceStatus{
			Provider: source.Provider(), Label: source.Label(),
			IsConfigured: source.IsConfigured(), RepoCount: counts[source.Provider()],
		}
		if outcome, hasRun := outcomes[source.Provider()]; hasRun {
			status.LastSync = &outcome
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func countByProvider(all []domain.Repo) map[domain.Provider]int {
	counts := map[domain.Provider]int{}
	for _, repo := range all {
		counts[repo.Provider]++
	}
	return counts
}
