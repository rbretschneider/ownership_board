package app

import (
	"context"
	"time"

	"github.com/rbretschneider/ownership_board/internal/clock"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Store runs units of work against persistence. Write wraps fn in a
// transaction that commits only when fn returns nil.
type Store interface {
	Read(ctx context.Context, fn func(Repos) error) error
	Write(ctx context.Context, fn func(Repos) error) error
}

// Repos is the set of repositories bound to one unit of work. ("Repository"
// here is the persistence pattern; code repositories are Repo.)
type Repos interface {
	Tickets() TicketRepository
	Questions() QuestionRepository
	Timeline() TimelineRepository
	CodeRepos() RepoRepository
	SourceStatuses() SourceStatusRepository
}

// TicketFilter narrows a ticket listing. The zero value matches every ticket.
type TicketFilter struct {
	// IsScoped limits results to RepoIDs, plus tickets without a repo when
	// IncludeUnassigned is set.
	IsScoped          bool
	RepoIDs           []int64
	IncludeUnassigned bool
}

// TicketRepository persists tickets.
type TicketRepository interface {
	Get(ctx context.Context, id int64) (domain.Ticket, error)
	List(ctx context.Context, filter TicketFilter) ([]domain.Ticket, error)
	Insert(ctx context.Context, ticket *domain.Ticket) error
	Update(ctx context.Context, ticket domain.Ticket) error
	Delete(ctx context.Context, id int64) error
	// LaneOrder returns ticket ids and ranks in a lane, sorted by rank.
	LaneOrder(ctx context.Context, lane domain.Lane) ([]RankedTicket, error)
	SetRank(ctx context.Context, id int64, rank float64) error
	// NextClaimable returns the top unleased ticket in lane matching filter.
	NextClaimable(ctx context.Context, lane domain.Lane, filter TicketFilter, now time.Time) (domain.Ticket, bool, error)
}

// RankedTicket is a ticket's position within its lane.
type RankedTicket struct {
	ID   int64
	Rank float64
}

// QuestionRepository persists question rounds.
type QuestionRepository interface {
	ForTicket(ctx context.Context, ticketID int64) ([]domain.Question, error)
	Insert(ctx context.Context, question *domain.Question) error
	SaveAnswer(ctx context.Context, question domain.Question) error
	// OpenCounts maps ticket id to its number of unanswered questions.
	OpenCounts(ctx context.Context) (map[int64]int, error)
	LatestRound(ctx context.Context, ticketID int64) (int, error)
}

// TimelineRepository persists ticket history.
type TimelineRepository interface {
	Append(ctx context.Context, entry *domain.TimelineEntry) error
	ForTicket(ctx context.Context, ticketID int64) ([]domain.TimelineEntry, error)
}

// RepoRepository persists code repos.
type RepoRepository interface {
	List(ctx context.Context) ([]domain.Repo, error)
	Get(ctx context.Context, id int64) (domain.Repo, error)
	// ByFullName returns every repo with that name; mirrors on two providers
	// share one.
	ByFullName(ctx context.Context, fullName string) ([]domain.Repo, error)
	ByProvider(ctx context.Context, provider domain.Provider) ([]domain.Repo, error)
	Insert(ctx context.Context, repo *domain.Repo) error
	Update(ctx context.Context, repo domain.Repo) error
}

// SourceStatusRepository persists the outcome of the last sync per provider.
type SourceStatusRepository interface {
	All(ctx context.Context) (map[domain.Provider]SyncOutcome, error)
	Save(ctx context.Context, provider domain.Provider, outcome SyncOutcome) error
}

// Clock supplies the current time so tests can control it.
type Clock = clock.Clock

// EventSink receives events after the change that produced them commits.
type EventSink interface {
	Publish(ctx context.Context, events []Event)
}

// Notifier pushes a message to the human.
type Notifier interface {
	Notify(ctx context.Context, notification Notification) error
}

// RepoSource lists the repos one hosting provider account can see.
type RepoSource interface {
	Provider() domain.Provider
	// Label names the source for people, such as "Forgejo (git.example.com)".
	Label() string
	IsConfigured() bool
	List(ctx context.Context) ([]domain.RepoListing, error)
}
