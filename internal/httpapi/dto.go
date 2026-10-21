package httpapi

import (
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

// The JSON shapes of the API, documented in docs/api.md.

type userDTO struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	IsAdmin     bool   `json:"is_admin"`
	IsSSOLinked bool   `json:"sso_linked"`
}

type authStatusDTO struct {
	IsSetupRequired bool     `json:"setup_required"`
	IsPasswordLogin bool     `json:"password_login"`
	OIDC            oidcDTO  `json:"oidc"`
	User            *userDTO `json:"user"`
}

type oidcDTO struct {
	IsEnabled bool   `json:"enabled"`
	Label     string `json:"label"`
}

type repoDTO struct {
	ID            int64           `json:"id"`
	Provider      domain.Provider `json:"provider"`
	FullName      string          `json:"full_name"`
	Name          string          `json:"name"`
	URL           string          `json:"url"`
	Description   string          `json:"description"`
	Color         string          `json:"color"`
	DefaultBranch string          `json:"default_branch"`
	IsPrivate     bool            `json:"is_private"`
	IsArchived    bool            `json:"is_archived"`
	SyncedAt      *time.Time      `json:"synced_at"`
}

type sourceDTO struct {
	Provider     domain.Provider `json:"provider"`
	Label        string          `json:"label"`
	IsConfigured bool            `json:"is_configured"`
	RepoCount    int             `json:"repo_count"`
	LastSyncedAt *time.Time      `json:"last_synced_at"`
	LastError    string          `json:"last_error"`
}

type ticketSummaryDTO struct {
	ID            int64         `json:"id"`
	RepoID        *int64        `json:"repo_id"`
	Kind          domain.Kind   `json:"kind"`
	Title         string        `json:"title"`
	Lane          domain.Lane   `json:"lane"`
	Owner         domain.Owner  `json:"owner"`
	PRURL         string        `json:"pr_url"`
	Branch        string        `json:"branch"`
	OpenQuestions int           `json:"open_questions"`
	Lease         *domain.Lease `json:"lease"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	LaneChangedAt time.Time     `json:"lane_changed_at"`
}

type ticketDetailDTO struct {
	ticketSummaryDTO
	Body      string        `json:"body"`
	Spec      string        `json:"spec"`
	CreatedBy string        `json:"created_by"`
	Questions []questionDTO `json:"questions"`
	Timeline  []timelineDTO `json:"timeline"`
}

type questionDTO struct {
	ID          int64      `json:"id"`
	Round       int        `json:"round"`
	Prompt      string     `json:"prompt"`
	Options     []string   `json:"options"`
	Recommended *int       `json:"recommended"`
	AskedBy     string     `json:"asked_by"`
	AskedAt     time.Time  `json:"asked_at"`
	Answer      *answerDTO `json:"answer"`
}

type answerDTO struct {
	Choice     *int      `json:"choice"`
	Text       string    `json:"text"`
	IsDeferred bool      `json:"deferred"`
	By         string    `json:"by"`
	At         time.Time `json:"at"`
}

type timelineDTO struct {
	ID    int64               `json:"id"`
	Type  domain.TimelineType `json:"type"`
	At    time.Time           `json:"at"`
	Actor actorRefDTO         `json:"actor"`
	Body  string              `json:"body"`
	Data  map[string]any      `json:"data"`
}

type actorRefDTO struct {
	Kind domain.ActorKind `json:"kind"`
	Name string           `json:"name"`
}

type boardDTO struct {
	Lanes   []domain.LaneInfo  `json:"lanes"`
	Repos   []repoDTO          `json:"repos"`
	Tickets []ticketSummaryDTO `json:"tickets"`
}

type tokenDTO struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type createdTokenDTO struct {
	tokenDTO
	Secret string `json:"secret"`
}

type workDTO struct {
	Action domain.WorkAction `json:"action"`
	Ticket ticketDetailDTO   `json:"ticket"`
	Repo   *repoDTO          `json:"repo"`
	Brief  string            `json:"brief"`
	Lease  *domain.Lease     `json:"lease"`
}

func toUser(user auth.User) userDTO {
	return userDTO{ID: user.ID, Name: user.Name, Email: user.Email, IsAdmin: user.IsAdmin, IsSSOLinked: user.IsSSOLinked}
}

func toRepo(repo domain.Repo) repoDTO {
	return repoDTO{
		ID: repo.ID, Provider: repo.Provider, FullName: repo.FullName, Name: repo.Name(), URL: repo.URL,
		Description: repo.Description, Color: repo.Color, DefaultBranch: repo.DefaultBranch,
		IsPrivate: repo.IsPrivate, IsArchived: repo.IsArchived, SyncedAt: repo.SyncedAt,
	}
}

func toRepos(repos []domain.Repo) []repoDTO {
	out := make([]repoDTO, 0, len(repos))
	for _, repo := range repos {
		out = append(out, toRepo(repo))
	}
	return out
}

func toSources(statuses []app.SourceStatus) []sourceDTO {
	out := make([]sourceDTO, 0, len(statuses))
	for _, status := range statuses {
		source := sourceDTO{Provider: status.Provider, Label: status.Label, IsConfigured: status.IsConfigured, RepoCount: status.RepoCount}
		if status.LastSync != nil {
			at := status.LastSync.At
			source.LastSyncedAt, source.LastError = &at, status.LastSync.Error
		}
		out = append(out, source)
	}
	return out
}

func toSummary(summary app.TicketSummary) ticketSummaryDTO {
	t := summary.Ticket
	var repoID *int64
	if t.RepoID != 0 {
		id := t.RepoID
		repoID = &id
	}
	return ticketSummaryDTO{
		ID: t.ID, RepoID: repoID, Kind: t.Kind, Title: t.Title, Lane: t.Lane, Owner: t.Owner(),
		PRURL: t.PRURL, Branch: t.Branch, OpenQuestions: summary.OpenQuestions, Lease: activeLease(t),
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, LaneChangedAt: t.LaneChangedAt,
	}
}

// activeLease hides an expired lease, which no longer means anything.
func activeLease(t domain.Ticket) *domain.Lease {
	if !t.IsLeased(time.Now()) {
		return nil
	}
	return t.Lease
}

func toDetail(detail app.TicketDetail) ticketDetailDTO {
	questions := make([]questionDTO, 0, len(detail.Questions))
	for _, question := range detail.Questions {
		questions = append(questions, toQuestion(question))
	}
	timeline := make([]timelineDTO, 0, len(detail.Timeline))
	for _, entry := range detail.Timeline {
		timeline = append(timeline, timelineDTO{
			ID: entry.ID, Type: entry.Type, At: entry.At, Body: entry.Body, Data: entry.Data,
			Actor: actorRefDTO{Kind: entry.Actor.Kind, Name: entry.Actor.Name},
		})
	}
	t := detail.Ticket
	return ticketDetailDTO{
		ticketSummaryDTO: toSummary(detail.TicketSummary), Body: t.Body, Spec: t.Spec, CreatedBy: t.CreatedBy,
		Questions: questions, Timeline: timeline,
	}
}

func toQuestion(question domain.Question) questionDTO {
	dto := questionDTO{
		ID: question.ID, Round: question.Round, Prompt: question.Prompt, Options: question.Options,
		Recommended: question.Recommended, AskedBy: question.AskedBy, AskedAt: question.AskedAt,
	}
	if dto.Options == nil {
		dto.Options = []string{}
	}
	if answer := question.Answer; answer != nil {
		dto.Answer = &answerDTO{Choice: answer.Choice, Text: answer.Text, IsDeferred: answer.IsDeferred, By: answer.By, At: answer.At}
	}
	return dto
}

func toBoard(board app.Board) boardDTO {
	tickets := make([]ticketSummaryDTO, 0, len(board.Tickets))
	for _, summary := range board.Tickets {
		tickets = append(tickets, toSummary(summary))
	}
	return boardDTO{Lanes: board.Lanes, Repos: toRepos(board.Repos), Tickets: tickets}
}

func toToken(token auth.APIToken) tokenDTO {
	return tokenDTO{ID: token.ID, Name: token.Name, Prefix: token.Prefix, CreatedAt: token.CreatedAt, LastUsedAt: token.LastUsedAt}
}

func toWork(work app.Work) workDTO {
	dto := workDTO{Action: work.Action, Ticket: toDetail(work.Ticket), Brief: work.Brief, Lease: work.Ticket.Ticket.Lease}
	if work.Repo != nil {
		repo := toRepo(*work.Repo)
		dto.Repo = &repo
	}
	return dto
}
