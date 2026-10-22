package domain

import (
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Ticket is a story or bug moving across the board. Its Stage is the source of
// truth for where it is; Lane is derived from the stage and only ever set by
// setStage.
type Ticket struct {
	ID            int64
	RepoID        int64
	Kind          Kind
	Title         string
	Body          string
	Spec          string
	Design        string
	Lane          Lane
	Stage         Stage
	Rank          float64
	PRURL         string
	Branch        string
	Lease         *Lease
	BlockedReason string
	// Revisions count saves of the spec and design; approvals record which
	// revision the owner signed off, so a later edit needs a new approval.
	SpecRevision           int
	DesignRevision         int
	ApprovedSpecRevision   int
	ApprovedDesignRevision int
	CreatedBy              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	LaneChangedAt          time.Time
}

// TicketDraft is the input for a new ticket. Every ticket starts in Todo,
// being refined: nobody, agent or person, can create one further along.
type TicketDraft struct {
	RepoID int64
	Kind   string
	Title  string
	Body   string
}

// TicketEdit lists the fields to change; nil leaves a field as it is.
type TicketEdit struct {
	RepoID *int64
	Kind   *string
	Title  *string
	Body   *string
	PRURL  *string
	Branch *string
}

// NewTicket validates a draft and returns an unsaved ticket in todo/refine.
func NewTicket(draft TicketDraft, actor Actor, now time.Time) (Ticket, error) {
	if draft.RepoID < 0 {
		return Ticket{}, Invalidf("repo_id must be positive")
	}
	kind, err := ParseKind(draft.Kind)
	if err != nil {
		return Ticket{}, err
	}
	title, err := RequireText("title", draft.Title, MaxTitleLength)
	if err != nil {
		return Ticket{}, err
	}
	body, err := LimitText("body", strings.TrimSpace(draft.Body), MaxBodyLength)
	if err != nil {
		return Ticket{}, err
	}
	return Ticket{
		RepoID: draft.RepoID, Kind: kind, Title: title, Body: body, Lane: LaneTodo, Stage: StageRefine,
		CreatedBy: actor.Name, CreatedAt: now, UpdatedAt: now, LaneChangedAt: now,
	}, nil
}

// IsLeased reports whether an agent currently holds the ticket.
func (t *Ticket) IsLeased(now time.Time) bool {
	return t.Lease.IsActive(now)
}

// IsBlocked reports whether an agent stopped and a person must decide.
func (t *Ticket) IsBlocked() bool {
	return t.BlockedReason != ""
}

// NeedsOwner reports whether the ball is with the owner: questions to answer,
// a block to resolve, or a checkpoint or PR to review.
func (t *Ticket) NeedsOwner(openQuestions int) bool {
	return openQuestions > 0 || t.IsBlocked() || t.Stage.IsOwnerReview()
}

// Owner says who acts next.
func (t *Ticket) Owner(openQuestions int) Owner {
	switch {
	case t.NeedsOwner(openQuestions):
		return OwnerHuman
	case t.Stage == StageDone:
		return OwnerNobody
	default:
		return OwnerAgent
	}
}

// IsClaimable reports whether an agent may take the ticket now: an agent stage,
// nothing waiting on the owner, and no live lease.
func (t *Ticket) IsClaimable(openQuestions int, now time.Time) bool {
	return t.Stage.IsAgentStage() && openQuestions == 0 && !t.IsBlocked() && !t.IsLeased(now)
}

// Reorder places the ticket at a new rank within its lane. Nothing else
// changes: reordering is never a way to move a ticket past a checkpoint.
func (t *Ticket) Reorder(rank float64, now time.Time) {
	t.Rank = rank
	t.UpdatedAt = now
}

// setStage is the only place a ticket's stage and lane change. Every stage
// change is a hand-off, so it always drops the lease.
func (t *Ticket) setStage(stage Stage, rank float64, now time.Time) bool {
	isLaneChange := t.Lane != stage.Lane()
	t.Stage, t.Lane, t.Rank = stage, stage.Lane(), rank
	t.Lease = nil
	t.UpdatedAt = now
	if isLaneChange {
		t.LaneChangedAt = now
	}
	return isLaneChange
}

// Claim gives the ticket to an agent until the lease expires.
func (t *Ticket) Claim(holder string, duration time.Duration, now time.Time) error {
	if t.IsLeased(now) && t.Lease.Holder != holder {
		return Conflictf("ticket %d is leased by %s", t.ID, t.Lease.Holder)
	}
	t.Lease = &Lease{Holder: holder, ExpiresAt: now.Add(duration)}
	t.UpdatedAt = now
	return nil
}

// Release drops the lease so another agent may claim the ticket.
func (t *Ticket) Release(now time.Time) {
	t.Lease = nil
	t.UpdatedAt = now
}

// Apply changes the given fields and returns the names of those that changed.
func (t *Ticket) Apply(edit TicketEdit, now time.Time) ([]string, error) {
	next := *t
	steps := []func(*Ticket, TicketEdit) (string, error){
		applyRepo, applyKind, applyTitle, applyBody, applyPRURL, applyBranch,
	}
	var changed []string
	for _, step := range steps {
		field, err := step(&next, edit)
		if err != nil {
			return nil, err
		}
		if field != "" {
			changed = append(changed, field)
		}
	}
	if len(changed) > 0 {
		next.UpdatedAt = now
		*t = next
	}
	return changed, nil
}

func applyRepo(t *Ticket, edit TicketEdit) (string, error) {
	if edit.RepoID == nil || *edit.RepoID == t.RepoID {
		return "", nil
	}
	if *edit.RepoID < 0 {
		return "", Invalidf("repo_id must be positive")
	}
	t.RepoID = *edit.RepoID
	return "repo", nil
}

func applyKind(t *Ticket, edit TicketEdit) (string, error) {
	if edit.Kind == nil {
		return "", nil
	}
	kind, err := ParseKind(*edit.Kind)
	if err != nil || kind == t.Kind {
		return "", err
	}
	t.Kind = kind
	return "kind", nil
}

func applyTitle(t *Ticket, edit TicketEdit) (string, error) {
	if edit.Title == nil {
		return "", nil
	}
	title, err := RequireText("title", *edit.Title, MaxTitleLength)
	if err != nil || title == t.Title {
		return "", err
	}
	t.Title = title
	return "title", nil
}

func applyBody(t *Ticket, edit TicketEdit) (string, error) {
	if edit.Body == nil {
		return "", nil
	}
	body, err := LimitText("body", strings.TrimSpace(*edit.Body), MaxBodyLength)
	if err != nil || body == t.Body {
		return "", err
	}
	t.Body = body
	return "body", nil
}

func applyPRURL(t *Ticket, edit TicketEdit) (string, error) {
	if edit.PRURL == nil {
		return "", nil
	}
	prURL, err := parseOptionalURL("pr_url", *edit.PRURL)
	if err != nil || prURL == t.PRURL {
		return "", err
	}
	t.PRURL = prURL
	return "pr_url", nil
}

func applyBranch(t *Ticket, edit TicketEdit) (string, error) {
	if edit.Branch == nil {
		return "", nil
	}
	branch, err := LimitText("branch", strings.TrimSpace(*edit.Branch), MaxBranchLength)
	if err != nil || branch == t.Branch {
		return "", err
	}
	t.Branch = branch
	return "branch", nil
}

// urlCharacters is the RFC 3986 unreserved, reserved and percent characters.
var urlCharacters = regexp.MustCompile(`^[A-Za-z0-9\-._~:/?#\[\]@!$&'()*+,;=%]+$`)

// parseOptionalURL accepts empty or an absolute http(s) URL, so a link stored
// on a ticket can never become a javascript: href in the UI.
func parseOptionalURL(field, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if _, err := LimitText(field, value, MaxURLLength); err != nil {
		return "", err
	}
	// Only characters RFC 3986 allows: no spaces, angle brackets, quotes or
	// backticks, which could break out of where a URL is shown.
	if !urlCharacters.MatchString(value) {
		return "", Invalidf("%s contains characters a URL may not contain; percent-encode them", field)
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		return "", Invalidf("%s must be an absolute http or https URL without a user name or password", field)
	}
	return value, nil
}
