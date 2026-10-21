package sqlite

import "github.com/rbretschneider/ownership_board/internal/app"

// repos binds every repository to one querier, a transaction or the pool.
type repos struct {
	q querier
}

func (r repos) Tickets() app.TicketRepository              { return ticketRepo{q: r.q} }
func (r repos) Questions() app.QuestionRepository          { return questionRepo{q: r.q} }
func (r repos) Timeline() app.TimelineRepository           { return timelineRepo{q: r.q} }
func (r repos) CodeRepos() app.RepoRepository              { return codeRepoRepo{q: r.q} }
func (r repos) SourceStatuses() app.SourceStatusRepository { return sourceStatusRepo{q: r.q} }
func (r repos) WorkLinks() app.WorkLinkRepository          { return workLinkRepo{q: r.q} }
func (r repos) Decisions() app.DecisionRepository          { return decisionRepo{q: r.q} }
func (r repos) Relations() app.RelationRepository          { return relationRepo{q: r.q} }
