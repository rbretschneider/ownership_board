package app

import (
	"context"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// maxTicketVerdicts bounds a ticket's verdict listing.
const maxTicketVerdicts = 100

// VerdictService reads verdicts and records the clean merges agents report.
// Build jobs write their own verdicts when they finish (BuildJobService.Complete).
type VerdictService struct {
	store Store
	clock Clock
}

func NewVerdictService(store Store, clock Clock) *VerdictService {
	return &VerdictService{store: store, clock: clock}
}

// ForCommit returns a commit's verdicts on every target of the repo.
func (s *VerdictService) ForCommit(ctx context.Context, repo, sha string) ([]domain.Verdict, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return nil, domain.Invalidf("repo is required with sha")
	}
	sha, err := domain.NormalizeCommitSHA("sha", sha)
	if err != nil {
		return nil, err
	}
	var out []domain.Verdict
	err = s.store.Read(ctx, func(r Repos) error {
		out, err = r.Verdicts().ForCommit(ctx, repo, sha)
		return err
	})
	return out, err
}

// ForTicket returns a ticket's verdicts, newest first.
func (s *VerdictService) ForTicket(ctx context.Context, ticketID int64) ([]domain.Verdict, error) {
	if ticketID <= 0 {
		return nil, domain.Invalidf("invalid ticket id")
	}
	var out []domain.Verdict
	err := s.store.Read(ctx, func(r Repos) error {
		if _, err := r.Tickets().Get(ctx, ticketID); err != nil {
			return err
		}
		var err error
		out, err = r.Verdicts().ForTicket(ctx, ticketID, maxTicketVerdicts)
		return err
	})
	return out, err
}

// ReportCleanMerge records that report.SHA is a conflict-free merge of the
// default branch into report.MergeOf, so it counts as passed without another
// host run. The board cannot see git: it trusts the agent holding the ticket's
// lease and records who reported it. It refuses unless MergeOf has a pass on
// the target (on any target, when none is named), and never overrules a run
// that failed the commit. It returns the commit's verdicts on those targets.
func (s *VerdictService) ReportCleanMerge(ctx context.Context, actor domain.Actor, report domain.CleanMergeReport) ([]domain.Verdict, error) {
	if err := report.Validate(); err != nil {
		return nil, err
	}
	var out []domain.Verdict
	err := s.store.Write(ctx, func(r Repos) error {
		now := s.clock.Now().UTC().Truncate(time.Millisecond)
		ticket, err := r.Tickets().Get(ctx, report.TicketID)
		if err != nil {
			return err
		}
		if err := requireHolder(ticket, actor, now); err != nil {
			return err
		}
		if ticket.RepoID <= 0 {
			return domain.Invalidf("ticket %d has no repo", ticket.ID)
		}
		repo, err := r.CodeRepos().Get(ctx, ticket.RepoID)
		if err != nil {
			return err
		}
		if repo.FullName != report.Repo {
			return domain.Invalidf("ticket %d belongs to %s, not %s", ticket.ID, repo.FullName, report.Repo)
		}
		bases, err := passedBases(ctx, r, report)
		if err != nil {
			return err
		}
		out = make([]domain.Verdict, 0, len(bases))
		for _, base := range bases {
			v, err := recordCleanMerge(ctx, r, report, base, actor, now)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return nil
	})
	return out, err
}

// passedBases returns MergeOf's passing verdicts on the report's target, or on
// every target when it names none.
func passedBases(ctx context.Context, r Repos, report domain.CleanMergeReport) ([]domain.Verdict, error) {
	if report.Target != "" {
		base, err := r.Verdicts().Get(ctx, report.Repo, report.Target, report.MergeOf)
		if domain.KindOf(err) == domain.KindNotFound || err == nil && !base.IsPassed() {
			return nil, domain.Conflictf("commit %s has no pass on %s; a clean merge counts only on top of a passed commit", report.MergeOf, report.Target)
		}
		if err != nil {
			return nil, err
		}
		return []domain.Verdict{base}, nil
	}
	all, err := r.Verdicts().ForCommit(ctx, report.Repo, report.MergeOf)
	if err != nil {
		return nil, err
	}
	var passed []domain.Verdict
	for _, v := range all {
		if v.IsPassed() {
			passed = append(passed, v)
		}
	}
	if len(passed) == 0 {
		return nil, domain.Conflictf("commit %s has no pass on any target; a clean merge counts only on top of a passed commit", report.MergeOf)
	}
	return passed, nil
}

func recordCleanMerge(ctx context.Context, r Repos, report domain.CleanMergeReport, base domain.Verdict, actor domain.Actor, now time.Time) (domain.Verdict, error) {
	existing, err := r.Verdicts().Get(ctx, report.Repo, base.Target, report.SHA)
	switch {
	case err == nil && existing.Status == domain.VerdictFailed:
		return domain.Verdict{}, domain.Conflictf("commit %s already failed on %s; a clean merge cannot overrule a run", report.SHA, base.Target)
	case err == nil:
		return existing, nil // already passed: reporting again changes nothing
	case domain.KindOf(err) != domain.KindNotFound:
		return domain.Verdict{}, err
	}
	v := domain.Verdict{
		Repo: report.Repo, Target: base.Target, SHA: report.SHA, Status: domain.VerdictCleanMerge, MergeOf: report.MergeOf,
		JobID: base.JobID, TicketID: report.TicketID, Source: domain.VerdictFromCleanMerge, ReportedBy: actor.Name, RecordedAt: now,
	}
	return v, r.Verdicts().Record(ctx, v)
}
