package httpapi

import (
	"net/http"
	"strconv"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// repoNone is the board filter value for tickets without a repo.
const repoNone = "none"

func (s *server) boardRoutes(rt *router) {
	rt.anyone("GET /api/v1/lanes", s.lanes)
	rt.anyone("GET /api/v1/board", s.board)
	rt.anyone("POST /api/v1/tickets", s.createTicket)
	rt.anyone("GET /api/v1/tickets/{id}", s.ticket)
	rt.anyone("PATCH /api/v1/tickets/{id}", s.editTicket)
	rt.anyone("POST /api/v1/tickets/{id}/move", s.moveTicket)
	rt.anyone("POST /api/v1/tickets/{id}/comments", s.comment)
	rt.anyone("PUT /api/v1/tickets/{id}/spec", s.setSpec)
	rt.anyone("POST /api/v1/tickets/{id}/questions", s.ask)
	rt.human("POST /api/v1/tickets/{id}/answers", s.answer)
	rt.human("DELETE /api/v1/tickets/{id}", s.deleteTicket)
}

func (s *server) lanes(w http.ResponseWriter, _ *http.Request, _ caller) error {
	writeJSON(w, http.StatusOK, domain.Lanes())
	return nil
}

func (s *server) board(w http.ResponseWriter, r *http.Request, _ caller) error {
	scope, err := parseRepoScope(r.URL.Query().Get("repo"))
	if err != nil {
		return err
	}
	board, err := s.deps.Board.Board(r.Context(), scope)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toBoard(board))
	return nil
}

func (s *server) createTicket(w http.ResponseWriter, r *http.Request, who caller) error {
	var body struct {
		RepoID *int64 `json:"repo_id"`
		Kind   string `json:"kind"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		Lane   string `json:"lane"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	draft := domain.TicketDraft{RepoID: valueOrZero(body.RepoID), Kind: body.Kind, Title: body.Title, Body: body.Body, Lane: body.Lane}
	detail, err := s.deps.Board.Create(r.Context(), who.actor, draft)
	return respondDetail(w, http.StatusCreated, detail, err)
}

func (s *server) ticket(w http.ResponseWriter, r *http.Request, _ caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	detail, err := s.deps.Board.Ticket(r.Context(), id)
	return respondDetail(w, http.StatusOK, detail, err)
}

// editBody uses a raw value for repo_id so an explicit null (clear the repo)
// is told apart from an absent field (leave it).
type editBody struct {
	RepoID nullableID `json:"repo_id"`
	Kind   *string    `json:"kind"`
	Title  *string    `json:"title"`
	Body   *string    `json:"body"`
	PRURL  *string    `json:"pr_url"`
	Branch *string    `json:"branch"`
}

func (s *server) editTicket(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body editBody
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	edit := domain.TicketEdit{Kind: body.Kind, Title: body.Title, Body: body.Body, PRURL: body.PRURL, Branch: body.Branch}
	if body.RepoID.isSet {
		repoID := body.RepoID.value
		edit.RepoID = &repoID
	}
	detail, err := s.deps.Board.Edit(r.Context(), who.actor, id, edit)
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) moveTicket(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Lane    string `json:"lane"`
		AfterID *int64 `json:"after_id"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	detail, err := s.deps.Board.Move(r.Context(), who.actor, id, app.MoveInput{Lane: body.Lane, AfterID: body.AfterID})
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) comment(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	detail, err := s.deps.Board.Comment(r.Context(), who.actor, id, body.Body)
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) setSpec(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Spec string `json:"spec"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	detail, err := s.deps.Board.SetSpec(r.Context(), who.actor, id, body.Spec)
	return respondDetail(w, http.StatusOK, detail, err)
}

type askBody struct {
	Summary   string `json:"summary"`
	Questions []struct {
		Prompt      string   `json:"prompt"`
		Options     []string `json:"options"`
		Recommended *int     `json:"recommended"`
	} `json:"questions"`
}

func (b askBody) input() app.AskInput {
	input := app.AskInput{Summary: b.Summary}
	for _, question := range b.Questions {
		input.Questions = append(input.Questions, domain.QuestionDraft{
			Prompt: question.Prompt, Options: question.Options, Recommended: question.Recommended,
		})
	}
	return input
}

func (s *server) ask(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body askBody
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	detail, err := s.deps.Board.Ask(r.Context(), who.actor, id, body.input())
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) answer(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Answers []struct {
			QuestionID int64  `json:"question_id"`
			Choice     *int   `json:"choice"`
			Text       string `json:"text"`
			IsDeferred bool   `json:"deferred"`
		} `json:"answers"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	drafts := make([]domain.AnswerDraft, 0, len(body.Answers))
	for _, answer := range body.Answers {
		drafts = append(drafts, domain.AnswerDraft{QuestionID: answer.QuestionID, Choice: answer.Choice, Text: answer.Text, IsDeferred: answer.IsDeferred})
	}
	detail, err := s.deps.Board.Answer(r.Context(), who.actor, id, drafts)
	return respondDetail(w, http.StatusOK, detail, err)
}

func (s *server) deleteTicket(w http.ResponseWriter, r *http.Request, who caller) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.deps.Board.Delete(r.Context(), who.actor, id); err != nil {
		return err
	}
	writeNoContent(w)
	return nil
}

func respondDetail(w http.ResponseWriter, status int, detail app.TicketDetail, err error) error {
	if err != nil {
		return err
	}
	writeJSON(w, status, toDetail(detail))
	return nil
}

func parseRepoScope(raw string) (app.RepoScope, error) {
	switch raw {
	case "":
		return app.RepoScope{}, nil
	case repoNone:
		return app.RepoScope{IsUnassigned: true}, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return app.RepoScope{}, domain.Invalidf("repo must be a repo id or %q", repoNone)
	}
	return app.RepoScope{RepoID: id}, nil
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
