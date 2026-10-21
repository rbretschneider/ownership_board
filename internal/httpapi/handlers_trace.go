package httpapi

import (
	"net/http"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func (s *server) traceRoutes(rt *router) {
	rt.anyone("POST /api/v1/tickets/{id}/work-links", s.addWorkLink)
	rt.human("DELETE /api/v1/tickets/{id}/work-links/{link}", s.removeWorkLink)
	rt.anyone("GET /api/v1/work-links", s.findWork)
	rt.anyone("POST /api/v1/tickets/{id}/decisions", s.recordDecision)
	rt.human("DELETE /api/v1/tickets/{id}/decisions/{decision}", s.removeDecision)
	rt.anyone("POST /api/v1/tickets/{id}/relations", s.linkTickets)
	rt.human("DELETE /api/v1/tickets/{id}/relations/{kind}/{other}", s.unlinkTickets)
}

type actorRefFullDTO struct {
	Kind domain.ActorKind `json:"kind"`
	Name string           `json:"name"`
}

type workLinkDTO struct {
	ID      int64               `json:"id"`
	Kind    domain.WorkLinkKind `json:"kind"`
	URL     string              `json:"url"`
	SHA     string              `json:"sha"`
	Host    string              `json:"host"`
	AddedBy actorRefFullDTO     `json:"added_by"`
	AddedAt time.Time           `json:"added_at"`
}

type decisionDTO struct {
	ID             int64                 `json:"id"`
	Summary        string                `json:"summary"`
	Rationale      string                `json:"rationale"`
	Alternatives   []string              `json:"alternatives"`
	Source         domain.DecisionSource `json:"source"`
	Weight         domain.DecisionWeight `json:"weight"`
	DecidedBy      actorRefFullDTO       `json:"decided_by"`
	AskedBy        string                `json:"asked_by"`
	QuestionID     *int64                `json:"question_id"`
	SupersedesID   *int64                `json:"supersedes_id"`
	SupersededByID *int64                `json:"superseded_by_id"`
	IsRemoved      bool                  `json:"is_removed"`
	At             time.Time             `json:"at"`
}

type relationDTO struct {
	Kind      domain.RelationKind `json:"kind"`
	Direction string              `json:"direction"`
	Label     string              `json:"label"`
	Ticket    relatedTicketDTO    `json:"ticket"`
	AddedBy   actorRefFullDTO     `json:"added_by"`
	AddedAt   time.Time           `json:"added_at"`
}

type relatedTicketDTO struct {
	ID         int64             `json:"id"`
	Title      string            `json:"title"`
	Lane       domain.Lane       `json:"lane"`
	Visibility domain.Visibility `json:"visibility"`
}

type foundTicketDTO struct {
	ID    int64       `json:"id"`
	Title string      `json:"title"`
	Lane  domain.Lane `json:"lane"`
}

func (s *server) addWorkLink(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Kind string `json:"kind"`
		URL  string `json:"url"`
		SHA  string `json:"sha"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	detail, err := s.deps.Board.AddWorkLink(r.Context(), who.actor, id, domain.WorkLinkDraft{Kind: body.Kind, URL: body.URL, SHA: body.SHA})
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) removeWorkLink(w http.ResponseWriter, r *http.Request, who caller) error {
	id, linkID, err := twoIDs(r, "id", "link")
	if err != nil {
		return err
	}
	detail, err := s.deps.Board.RemoveWorkLink(r.Context(), who.actor, id, linkID)
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) findWork(w http.ResponseWriter, r *http.Request, _ caller) error {
	query := r.URL.Query()
	tickets, err := s.deps.Board.FindTicketsForWork(r.Context(), query.Get("sha"), query.Get("url"))
	if err != nil {
		return err
	}
	out := make([]foundTicketDTO, 0, len(tickets))
	for _, summary := range tickets {
		out = append(out, foundTicketDTO{ID: summary.Ticket.ID, Title: summary.Ticket.Title, Lane: summary.Ticket.Lane})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *server) recordDecision(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Summary      string   `json:"summary"`
		Rationale    string   `json:"rationale"`
		Alternatives []string `json:"alternatives"`
		SupersedesID *int64   `json:"supersedes_id"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	draft := domain.DecisionDraft{Summary: body.Summary, Rationale: body.Rationale, Alternatives: body.Alternatives, SupersedesID: body.SupersedesID}
	detail, err := s.deps.Board.RecordDecision(r.Context(), who.actor, id, draft)
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) removeDecision(w http.ResponseWriter, r *http.Request, who caller) error {
	id, decisionID, err := twoIDs(r, "id", "decision")
	if err != nil {
		return err
	}
	detail, err := s.deps.Board.RemoveDecision(r.Context(), who.actor, id, decisionID)
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) linkTickets(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Kind     string `json:"kind"`
		TicketID int64  `json:"ticket_id"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	detail, err := s.deps.Board.LinkTickets(r.Context(), who.actor, id, body.Kind, body.TicketID)
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) unlinkTickets(w http.ResponseWriter, r *http.Request, who caller) error {
	id, otherID, err := twoIDs(r, "id", "other")
	if err != nil {
		return err
	}
	detail, err := s.deps.Board.UnlinkTickets(r.Context(), who.actor, id, r.PathValue("kind"), otherID)
	return respondDetail(w, http.StatusOK, detail, err)
}

func twoIDs(r *http.Request, first, second string) (int64, int64, error) {
	a, err := pathID(r, first)
	if err != nil {
		return 0, 0, err
	}
	b, err := pathID(r, second)
	return a, b, err
}

func toWorkLinks(links []domain.WorkLink) []workLinkDTO {
	out := make([]workLinkDTO, 0, len(links))
	for _, link := range links {
		out = append(out, workLinkDTO{
			ID: link.ID, Kind: link.Kind, URL: link.URL, SHA: link.SHA, Host: link.Host(),
			AddedBy: actorRefFullDTO{Kind: link.AddedBy.Kind, Name: link.AddedBy.Name}, AddedAt: link.AddedAt,
		})
	}
	return out
}

func toDecisions(decisions []domain.Decision) []decisionDTO {
	out := make([]decisionDTO, 0, len(decisions))
	for _, d := range decisions {
		alternatives := d.Alternatives
		if alternatives == nil {
			alternatives = []string{}
		}
		out = append(out, decisionDTO{
			ID: d.ID, Summary: d.Summary, Rationale: d.Rationale, Alternatives: alternatives, Source: d.Source, Weight: d.Weight,
			DecidedBy: actorRefFullDTO{Kind: d.DecidedBy.Kind, Name: d.DecidedBy.Name}, AskedBy: d.AskedBy,
			QuestionID: d.QuestionID, SupersedesID: d.SupersedesID, SupersededByID: d.SupersededByID, IsRemoved: d.IsRemoved, At: d.At,
		})
	}
	return out
}

func toRelations(related []app.RelatedTicket) []relationDTO {
	out := make([]relationDTO, 0, len(related))
	now := time.Now()
	for _, item := range related {
		direction := "incoming"
		if item.IsOutgoing {
			direction = "outgoing"
		}
		visibility := domain.VisibilityPublic
		if item.Other.Repo != nil {
			visibility = item.Other.Repo.Visibility(now)
		}
		out = append(out, relationDTO{
			Kind: item.Relation.Kind, Direction: direction, Label: item.Relation.Kind.Label(item.IsOutgoing),
			Ticket:  relatedTicketDTO{ID: item.Other.ID, Title: item.Other.Title, Lane: item.Other.Lane, Visibility: visibility},
			AddedBy: actorRefFullDTO{Kind: item.Relation.AddedBy.Kind, Name: item.Relation.AddedBy.Name}, AddedAt: item.Relation.AddedAt,
		})
	}
	return out
}
