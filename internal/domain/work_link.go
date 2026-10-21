package domain

import (
	"net/url"
	"regexp"
	"strings"
	"time"
)

// WorkLinkKind is what a work link points at.
type WorkLinkKind string

// The work link kinds.
const (
	WorkLinkPR     WorkLinkKind = "pr"
	WorkLinkCommit WorkLinkKind = "commit"
)

// Work link limits.
const (
	MaxAgentWorkLinksPerTicket = 100
	MinSHAPrefixLength         = 7
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// WorkLink is a PR or commit that implements a ticket.
type WorkLink struct {
	ID       int64
	TicketID int64
	Kind     WorkLinkKind
	URL      string
	SHA      string
	RepoID   int64
	AddedBy  Actor
	AddedAt  time.Time
}

// WorkLinkDraft is the input for a work link.
type WorkLinkDraft struct {
	Kind string
	URL  string
	SHA  string
}

// NewWorkLink validates a draft for ticket. The link's repo is always the
// ticket's, and when that repo has a URL the link must be on the same host,
// so a look-alike or internal address cannot pose as the repo's PR.
func NewWorkLink(draft WorkLinkDraft, ticket Ticket, repo *Repo, actor Actor, now time.Time) (WorkLink, error) {
	if !actor.IsHuman() && strings.TrimSpace(draft.URL) != "" && (repo == nil || repo.URL == "") {
		return WorkLink{}, Invalidf("an agent can link a URL only for a ticket whose repo has a known address, so the host can be checked; set the repo first")
	}
	link := WorkLink{TicketID: ticket.ID, RepoID: ticket.RepoID, AddedBy: actor, AddedAt: now}
	switch WorkLinkKind(draft.Kind) {
	case WorkLinkPR:
		link.Kind = WorkLinkPR
		normalized, err := normalizeLinkURL(draft.URL, repo, true)
		if err != nil {
			return WorkLink{}, err
		}
		link.URL = normalized
	case WorkLinkCommit:
		link.Kind = WorkLinkCommit
		sha, err := ParseSHA(draft.SHA)
		if err != nil {
			return WorkLink{}, err
		}
		normalized, err := normalizeLinkURL(draft.URL, repo, false)
		if err != nil {
			return WorkLink{}, err
		}
		link.SHA, link.URL = sha, normalized
	default:
		return WorkLink{}, Invalidf("kind must be pr or commit")
	}
	return link, nil
}

// ParseSHA validates a full or abbreviated commit hash.
func ParseSHA(raw string) (string, error) {
	sha := strings.ToLower(strings.TrimSpace(raw))
	if !shaPattern.MatchString(sha) {
		return "", Invalidf("sha must be 7 to 40 hexadecimal characters")
	}
	return sha, nil
}

// Host is the link's host, shown next to it so a person sees where it goes.
func (l WorkLink) Host() string {
	parsed, err := url.Parse(l.URL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// NormalizeLinkURL canonicalises a URL for lookup, matching how links are
// stored.
func NormalizeLinkURL(raw string) (string, error) {
	return normalizeLinkURL(raw, nil, true)
}

func normalizeLinkURL(raw string, repo *Repo, isRequired bool) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		if isRequired {
			return "", Invalidf("url is required")
		}
		return "", nil
	}
	checked, err := parseOptionalURL("url", value)
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(checked)
	parsed.Host, parsed.Fragment, parsed.RawFragment = strings.ToLower(parsed.Host), "", ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	if repo != nil && repo.URL != "" && !sameHost(parsed.Host, repo.URL) {
		return "", Invalidf("the link must be on the same host as the repo (%s)", hostOf(repo.URL))
	}
	return parsed.String(), nil
}

func sameHost(host, repoURL string) bool {
	return strings.EqualFold(host, hostOf(repoURL))
}

func hostOf(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Host)
}
