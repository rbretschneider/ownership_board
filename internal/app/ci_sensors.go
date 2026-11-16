package app

import (
	"context"
	"fmt"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// CI sensor limits.
const (
	ciSensorRuns   = 50
	ciSensorDetail = 10
	// Bounds on the caller's queue limit.
	minCIQueueLimit = 5 * time.Minute
	maxCIQueueLimit = 24 * time.Hour
)

// CIFlagsResult is what the CI sensors found, plus a note for every
// repository or part the board could not read, so a broken token shows up in
// the manager's log instead of as silence.
type CIFlagsResult struct {
	Flags []domain.ManagerFlag
	Notes []string
}

// ciSensorScope is what the sensors read from the board itself.
type ciSensorScope struct {
	repos          []domain.Repo
	tickets        map[int64][]domain.Ticket
	boardCancelled map[int64]bool
}

// Flags runs the CI sensors over every repository that has an open
// autonomous ticket in scope: jobs queued past queueLimit, runs cancelled on
// a current head outside the board, failed default-branch or release runs,
// and offline runners. It runs no model.
func (s *CIService) Flags(ctx context.Context, actor domain.Actor, scope RepoScope, queueLimit time.Duration) (CIFlagsResult, error) {
	if err := requireManagerOrPerson(actor); err != nil {
		return CIFlagsResult{}, err
	}
	if queueLimit != 0 && (queueLimit < minCIQueueLimit || queueLimit > maxCIQueueLimit) {
		return CIFlagsResult{}, domain.Invalidf("the CI queue limit must be between %s and %s", minCIQueueLimit, maxCIQueueLimit)
	}
	sensorScope, err := s.loadSensorScope(ctx, scope)
	if err != nil {
		return CIFlagsResult{}, err
	}
	var result CIFlagsResult
	input := domain.CISensorInput{BoardCancelled: sensorScope.boardCancelled, QueueLimit: queueLimit, Now: s.clock.Now()}
	for _, repo := range sensorScope.repos {
		facts, notes := s.readRepoCI(ctx, repo, sensorScope.tickets[repo.ID])
		result.Notes = append(result.Notes, notes...)
		if facts != nil {
			input.Repos = append(input.Repos, *facts)
		}
	}
	result.Flags = domain.CIFlags(input)
	return result, nil
}

// loadSensorScope reads the repositories with open autonomous tickets, those
// tickets, and the runs the board itself cancelled.
func (s *CIService) loadSensorScope(ctx context.Context, scope RepoScope) (ciSensorScope, error) {
	out := ciSensorScope{tickets: map[int64][]domain.Ticket{}, boardCancelled: map[int64]bool{}}
	err := s.store.Read(ctx, func(repos Repos) error {
		tickets, err := repos.Tickets().List(ctx, scope.filter())
		if err != nil {
			return err
		}
		for _, ticket := range tickets {
			if ticket.IsAutonomous && !ticket.Stage.IsCompleted() && ticket.RepoID != 0 {
				out.tickets[ticket.RepoID] = append(out.tickets[ticket.RepoID], ticket)
			}
		}
		if out.repos, err = reposWithCI(ctx, repos, out.tickets); err != nil {
			return err
		}
		entries, err := repos.Timeline().OfTypes(ctx, []domain.TimelineType{domain.TimelineCI})
		for _, entry := range entries {
			if entry.Data["action"] == string(domain.CIActionCancel) {
				out.boardCancelled[dataInt(entry.Data["run_id"])] = true
			}
		}
		return err
	})
	return out, err
}

func reposWithCI(ctx context.Context, repos Repos, tickets map[int64][]domain.Ticket) ([]domain.Repo, error) {
	all, err := repos.CodeRepos().List(ctx)
	if err != nil {
		return nil, err
	}
	var withCI []domain.Repo
	for _, repo := range all {
		if len(tickets[repo.ID]) > 0 && repo.Provider != domain.ProviderManual && !repo.IsArchived {
			withCI = append(withCI, repo)
		}
	}
	return withCI, nil
}

// readRepoCI reads one repository's CI for the sensors. A failure costs only
// that repository (or that part), and is reported as a note.
func (s *CIService) readRepoCI(ctx context.Context, repo domain.Repo, tickets []domain.Ticket) (*domain.CIRepoFacts, []string) {
	provider, err := s.catalog.CI(ctx, repo.Provider)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: CI not readable: %v", repo.FullName, err)}
	}
	resolved := ciContext{repo: repo, provider: provider}
	runs, err := provider.Runs(ctx, repo.FullName, domain.CIRunQuery{Limit: ciSensorRuns})
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: CI runs not readable: %v", repo.FullName, err)}
	}
	facts := &domain.CIRepoFacts{Repo: repo}
	var notes []string
	if facts.Runs, err = withJobs(ctx, resolved, runs, isRunning, ciSensorDetail); err != nil {
		notes = append(notes, fmt.Sprintf("%s: CI jobs not readable: %v", repo.FullName, err))
		facts.Runs = runs
	}
	if repo.DefaultBranch != "" {
		if facts.DefaultHead, err = provider.BranchHead(ctx, repo.FullName, repo.DefaultBranch); err != nil {
			notes = append(notes, fmt.Sprintf("%s: default branch head not readable: %v", repo.FullName, err))
		}
	}
	facts.Tickets, notes = ticketHeads(ctx, provider, repo, tickets, notes)
	if facts.Runners, err = provider.Runners(ctx, repo.FullName); err != nil {
		notes = append(notes, fmt.Sprintf("%s: CI runners not readable (needs admin read on the repo): %v", repo.FullName, err))
	}
	return facts, notes
}

func ticketHeads(ctx context.Context, provider CIReader, repo domain.Repo, tickets []domain.Ticket, notes []string) ([]domain.CITicketFacts, []string) {
	var facts []domain.CITicketFacts
	for _, ticket := range tickets {
		branch, sha, err := ticketHead(ctx, provider, repo, ticket)
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: head of #%d not readable: %v", repo.FullName, ticket.ID, err))
			continue
		}
		if branch != "" {
			facts = append(facts, domain.CITicketFacts{Ticket: ticket, Branch: branch, HeadSHA: sha})
		}
	}
	return facts, notes
}

// dataInt reads a number back from timeline data, which JSON decodes as a
// float64.
func dataInt(value any) int64 {
	switch number := value.(type) {
	case float64:
		return int64(number)
	case int64:
		return number
	case int:
		return int64(number)
	default:
		return 0
	}
}
