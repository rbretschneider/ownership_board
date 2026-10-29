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

// SourceCatalog returns the repo sources that are currently usable. It is
// asked again before every sync, so connecting or disconnecting a provider
// takes effect without a restart.
type SourceCatalog interface {
	Sources(ctx context.Context) ([]RepoSource, error)
}

// ProviderSummary is what the board knows about one provider's repos.
type ProviderSummary struct {
	RepoCount int
	LastSync  *SyncOutcome
}

// RepoService manages the repos tickets target and syncs them from hosting
// providers.
type RepoService struct {
	store   Store
	clock   Clock
	catalog SourceCatalog
	logger  *slog.Logger
}

// NewRepoService wires the service to its dependencies.
func NewRepoService(store Store, clock Clock, catalog SourceCatalog, logger *slog.Logger) *RepoService {
	return &RepoService{store: store, clock: clock, catalog: catalog, logger: logger}
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

// Restyle changes a repo's color, description, visibility override or
// checkpoints.
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
		if err := repos.CodeRepos().Update(ctx, repo); err != nil || !style.ChangesGates() {
			return err
		}
		return repos.CodeRepos().SetGates(ctx, repo.ID, domain.GatesFor(&repo))
	})
	if err == nil && style.ChangesGates() {
		s.logger.Info("repo checkpoints changed", "repo", repo.FullName, "by", actor.Name,
			"require_spec_approval", repo.RequireSpecApproval, "require_design_approval", repo.RequireDesignApproval)
	}
	return repo, err
}

// Summaries reports repo counts and the last sync for every provider.
func (s *RepoService) Summaries(ctx context.Context) (map[domain.Provider]ProviderSummary, error) {
	summaries := map[domain.Provider]ProviderSummary{}
	err := s.store.Read(ctx, func(repos Repos) error {
		outcomes, err := repos.SourceStatuses().All(ctx)
		if err != nil {
			return err
		}
		all, err := repos.CodeRepos().List(ctx)
		for _, repo := range all {
			summary := summaries[repo.Provider]
			summary.RepoCount++
			summaries[repo.Provider] = summary
		}
		for provider, outcome := range outcomes {
			summary := summaries[provider]
			summary.LastSync = &outcome
			summaries[provider] = summary
		}
		return err
	})
	return summaries, err
}

// Sync refreshes repos from every usable provider. A failing provider is
// recorded on its status and does not stop the others. Repos a provider no
// longer lists are kept and marked missing, because tickets may still point
// at them.
func (s *RepoService) Sync(ctx context.Context) error {
	return s.syncWhere(ctx, func(domain.Provider) bool { return true })
}

// SyncProvider refreshes one provider's repos, for example right after it was
// connected.
func (s *RepoService) SyncProvider(ctx context.Context, provider domain.Provider) error {
	return s.syncWhere(ctx, func(candidate domain.Provider) bool { return candidate == provider })
}

func (s *RepoService) syncWhere(ctx context.Context, isWanted func(domain.Provider) bool) error {
	sources, err := s.catalog.Sources(ctx)
	if err != nil {
		return err
	}
	for _, source := range sources {
		if !source.IsConfigured() || !isWanted(source.Provider()) {
			continue
		}
		outcome := SyncOutcome{At: s.clock.Now()}
		if err := s.syncSource(ctx, source); err != nil {
			outcome.Error = err.Error()
			s.logger.Warn("repo sync failed", "provider", source.Provider(), "error", err)
		}
		if err := s.saveOutcome(ctx, source.Provider(), outcome); err != nil {
			return err
		}
	}
	return nil
}

// syncSource lists outside any transaction (it is network I/O), then writes.
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
		delete(byName, listing.FullName)
		err := s.upsertOne(ctx, store, existingRepo(existing, listing.FullName), listing, now)
		if domain.KindOf(err) == domain.KindInvalid {
			// Provider data is untrusted: one odd listing must not block the rest.
			s.logger.Warn("skipped an invalid repo listing", "provider", listing.Provider, "repo", listing.FullName, "error", err)
			continue
		}
		if err != nil {
			return fmt.Errorf("repo %s: %w", listing.FullName, err)
		}
	}
	return markMissing(ctx, store, byName, now)
}

func (s *RepoService) upsertOne(ctx context.Context, store RepoRepository, repo *domain.Repo, listing domain.RepoListing, now time.Time) error {
	if repo == nil {
		created, err := domain.NewSyncedRepo(listing, now)
		if err != nil {
			return err
		}
		return store.Insert(ctx, &created)
	}
	if err := repo.Refresh(listing, now); err != nil {
		return err
	}
	return store.Update(ctx, *repo)
}

func (s *RepoService) saveOutcome(ctx context.Context, provider domain.Provider, outcome SyncOutcome) error {
	return s.store.Write(ctx, func(repos Repos) error {
		return repos.SourceStatuses().Save(ctx, provider, outcome)
	})
}

func existingRepo(existing []domain.Repo, fullName string) *domain.Repo {
	for i := range existing {
		if existing[i].FullName == fullName {
			return &existing[i]
		}
	}
	return nil
}

// markMissing flags repos the provider did not list this time. They are kept,
// because tickets may still point at them.
func markMissing(ctx context.Context, store RepoRepository, unlisted map[string]domain.Repo, now time.Time) error {
	for _, repo := range unlisted {
		if repo.IsMissing() {
			continue
		}
		repo.MarkMissing(now)
		if err := store.Update(ctx, repo); err != nil {
			return fmt.Errorf("repo %s: %w", repo.FullName, err)
		}
	}
	return nil
}
