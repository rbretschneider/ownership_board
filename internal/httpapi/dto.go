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
	OverlapIgnore   []string                 `json:"overlap_ignore"`
	BuildTargets    []domain.RepoBuildTarget `json:"build_targets"`
	ID              int64                    `json:"id"`
	Provider        domain.Provider          `json:"provider"`
	FullName        string                   `json:"full_name"`
	Name            string                   `json:"name"`
	URL             string                   `json:"url"`
	Description     string                   `json:"description"`
	Color           string                   `json:"color"`
	DefaultBranch   string                   `json:"default_branch"`
	IsPrivate       bool                     `json:"is_private"`
	IsArchived      bool                     `json:"is_archived"`
	SyncedAt        *time.Time               `json:"synced_at"`
	MissingSince    *time.Time               `json:"missing_since"`
	TreatAsPublic   bool                     `json:"treat_as_public"`
	RequireSpec     bool                     `json:"require_spec_approval"`
	RequireDesign   bool                     `json:"require_design_approval"`
	DeployMode      domain.DeployMode        `json:"deploy_mode"`
	ReviewBlocking  domain.ReviewBlocking    `json:"review_blocking"`
	AllowAgentMerge bool                     `json:"allow_agent_merge"`
	Visibility      domain.Visibility        `json:"visibility"`

	HealthCheckEvery           int        `json:"health_check_every"`
	HealthCheckedThroughTicket *int64     `json:"health_checked_through_ticket"`
	HealthCheckedAt            *time.Time `json:"health_checked_at"`
}

type ticketSummaryDTO struct {
	ID             int64         `json:"id"`
	RepoID         *int64        `json:"repo_id"`
	Kind           domain.Kind   `json:"kind"`
	Title          string        `json:"title"`
	Lane           domain.Lane   `json:"lane"`
	Stage          domain.Stage  `json:"stage"`
	WorkModel      string        `json:"work_model"`
	NeedsOwner     bool          `json:"needs_owner"`
	BlockedReason  string        `json:"blocked_reason"`
	IsHealthCheck  bool          `json:"is_health_check"`
	IsAutonomous   bool          `json:"is_autonomous"`
	SpecRevision   int           `json:"spec_revision"`
	DesignRevision int           `json:"design_revision"`
	Owner          domain.Owner  `json:"owner"`
	PRURL          string        `json:"pr_url"`
	Branch         string        `json:"branch"`
	OpenQuestions  int           `json:"open_questions"`
	WaitingOn      []int64       `json:"waiting_on"`
	Lease          *domain.Lease `json:"lease"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	LaneChangedAt  time.Time     `json:"lane_changed_at"`
	Attempts       attemptsDTO   `json:"attempts"`
	Usage          usageDTO      `json:"usage"`
}

type ticketDetailDTO struct {
	ticketSummaryDTO
	Body                   string        `json:"body"`
	Spec                   string        `json:"spec"`
	Design                 string        `json:"design"`
	ApprovedSpecRevision   int           `json:"approved_spec_revision"`
	ApprovedDesignRevision int           `json:"approved_design_revision"`
	Gates                  gatesDTO      `json:"gates"`
	CreatedBy              string        `json:"created_by"`
	Questions              []questionDTO `json:"questions"`
	Timeline               []timelineDTO `json:"timeline"`
	WorkLinks              []workLinkDTO `json:"work_links"`
	Decisions              []decisionDTO `json:"decisions"`
	Relations              []relationDTO `json:"relations"`
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
	Stages  []domain.StageInfo `json:"stages"`
	Repos   []repoDTO          `json:"repos"`
	Tickets []ticketSummaryDTO `json:"tickets"`
}

type tokenDTO struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	IsDelegate bool       `json:"is_delegate"`
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
	if repo.OverlapIgnore == nil {
		repo.OverlapIgnore = []string{}
	}
	if repo.BuildTargets == nil {
		repo.BuildTargets = []domain.RepoBuildTarget{}
	}
	return repoDTO{
		OverlapIgnore: repo.OverlapIgnore, BuildTargets: repo.BuildTargets, ID: repo.ID, Provider: repo.Provider, FullName: repo.FullName, Name: repo.Name(), URL: repo.URL,
		Description: repo.Description, Color: repo.Color, DefaultBranch: repo.DefaultBranch,
		IsPrivate: repo.IsPrivate, IsArchived: repo.IsArchived, SyncedAt: repo.SyncedAt, MissingSince: repo.MissingSince,
		TreatAsPublic: repo.TreatAsPublic, Visibility: repo.Visibility(time.Now()),
		RequireSpec: repo.RequireSpecApproval, RequireDesign: repo.RequireDesignApproval,
		AllowAgentMerge:  repo.AllowAgentMerge,
		DeployMode:       repo.DeployMode,
		ReviewBlocking:   repo.ReviewBlocking,
		HealthCheckEvery: repo.HealthCheckEvery, HealthCheckedThroughTicket: repo.HealthCheckedThroughTicket,
		HealthCheckedAt: repo.HealthCheckedAt,
	}
}

func toRepos(repos []domain.Repo) []repoDTO {
	out := make([]repoDTO, 0, len(repos))
	for _, repo := range repos {
		out = append(out, toRepo(repo))
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
		ID: t.ID, RepoID: repoID, Kind: t.Kind, Title: t.Title, Lane: t.Lane, Stage: t.Stage,
		WorkModel: t.WorkModel, Owner: t.Owner(summary.OpenQuestions), NeedsOwner: t.NeedsOwner(summary.OpenQuestions), BlockedReason: t.BlockedReason,
		IsAutonomous: t.IsAutonomous, IsHealthCheck: t.IsHealthCheck,
		SpecRevision: t.SpecRevision, DesignRevision: t.DesignRevision,
		PRURL: t.PRURL, Branch: t.Branch, OpenQuestions: summary.OpenQuestions, WaitingOn: nonNilIDs(summary.WaitingOn), Lease: activeLease(t),
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, LaneChangedAt: t.LaneChangedAt,
		Attempts: attemptsDTO(summary.Attempts), Usage: toUsage(summary.Usage),
	}
}

// usageDTO is a ticket's reported model token usage. Weighted counts cache
// reads at a tenth, so Claude and Codex calls compare.
type usageDTO struct {
	Calls            int     `json:"calls"`
	InputTokens      int64   `json:"input_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Weighted         int64   `json:"weighted_tokens"`
}

func toUsage(u domain.TokenUsage) usageDTO {
	return usageDTO{
		Calls: u.Calls, InputTokens: u.InputTokens, CacheWriteTokens: u.CacheWriteTokens,
		CacheReadTokens: u.CacheReadTokens, OutputTokens: u.OutputTokens, CostUSD: u.CostUSD, Weighted: u.Weighted(),
	}
}

// nonNilIDs keeps an empty id list a JSON array rather than null.
func nonNilIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
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
		ticketSummaryDTO: toSummary(detail.TicketSummary), Body: t.Body, Spec: t.Spec, Design: t.Design,
		ApprovedSpecRevision: t.ApprovedSpecRevision, ApprovedDesignRevision: t.ApprovedDesignRevision, Gates: toGates(t.Gates(domain.GatesFor(detail.Repo))),
		CreatedBy: t.CreatedBy,
		Questions: questions, Timeline: timeline,
		WorkLinks: toWorkLinks(detail.WorkLinks), Decisions: toDecisions(detail.Decisions), Relations: toRelations(detail.Related),
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
	return boardDTO{Lanes: board.Lanes, Stages: board.Stages, Repos: toRepos(board.Repos), Tickets: tickets}
}

func toToken(token auth.APIToken) tokenDTO {
	return tokenDTO{ID: token.ID, Name: token.Name, Prefix: token.Prefix, CreatedAt: token.CreatedAt, LastUsedAt: token.LastUsedAt, IsDelegate: token.IsDelegate}
}

func toWork(work app.Work) workDTO {
	dto := workDTO{Action: work.Action, Ticket: toDetail(work.Ticket), Brief: work.Brief, Lease: work.Ticket.Ticket.Lease}
	if work.Repo != nil {
		repo := toRepo(*work.Repo)
		dto.Repo = &repo
	}
	return dto
}

type gatesDTO struct {
	Spec   bool `json:"spec"`
	Design bool `json:"design"`
}

func toGates(gates domain.Gates) gatesDTO {
	return gatesDTO{Spec: gates.Spec, Design: gates.Design}
}

// attemptsDTO counts passes through each delivery step, derived from the
// ticket's recorded stage moves and work-model reports.
type attemptsDTO struct {
	Build        int `json:"build"`
	Test         int `json:"test"`
	TestFailures int `json:"test_failures"`
	Review       int `json:"review"`
	ReviewFixes  int `json:"review_fixes"`
	PR           int `json:"pr"`
	PRRepairs    int `json:"pr_repairs"`
}
