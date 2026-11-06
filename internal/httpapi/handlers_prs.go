package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"
)

// What each open board pull request is waiting for, written by the merge
// sweep as a small sanitized file (like the Windows runners). The board only
// reads it; nothing here can merge, repair or change a pull request.

type prCheck struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

type prStatus struct {
	TicketID    int64     `json:"ticket_id"`
	URL         string    `json:"url"`
	Verdict     string    `json:"verdict"`
	Reason      string    `json:"reason"`
	MayMerge    bool      `json:"may_merge"`
	Acts        bool      `json:"acts"`
	GLWaived    bool      `json:"gl_waived"`
	Draft       bool      `json:"draft"`
	Mergeable   string    `json:"mergeable"`
	Review      string    `json:"review"`
	Head        string    `json:"head"`
	Unresolved  bool      `json:"unresolved"`
	ManualCheck bool      `json:"manual_check"`
	Checks      []prCheck `json:"checks"`
}

type prStatusSnapshot struct {
	ObservedAt *time.Time `json:"observed_at"`
	PRs        []prStatus `json:"prs"`
}

// The file's own shape: epoch seconds for the sweep time, ISO times for checks.
type prStatusFile struct {
	At  float64 `json:"at"`
	PRs []struct {
		Ticket      int64  `json:"ticket"`
		URL         string `json:"url"`
		Verdict     string `json:"verdict"`
		Reason      string `json:"reason"`
		MayMerge    bool   `json:"may_merge"`
		Acts        bool   `json:"acts"`
		GLWaived    bool   `json:"gl_waived"`
		Draft       bool   `json:"draft"`
		Mergeable   string `json:"mergeable"`
		Review      string `json:"review"`
		Head        string `json:"head"`
		Unresolved  bool   `json:"unresolved"`
		ManualCheck bool   `json:"manual_check"`
		Checks      []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			Started    string `json:"started"`
			Completed  string `json:"completed"`
		} `json:"checks"`
	} `json:"prs"`
}

const (
	maxPRStatuses = 100
	maxPRChecks   = 30
)

var (
	prURLPattern       = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[0-9]{1,9}$`)
	prCheckNamePattern = regexp.MustCompile(`^[A-Za-z0-9 _.,:;()/#+&'\[\]-]{1,80}$`)
	prHeadPattern      = regexp.MustCompile(`^[0-9a-f]{7,12}$`)
	prVerdicts         = map[string]bool{"running": true, "no_checks": true, "draft": true, "checking_mergeable": true,
		"checks_incomplete": true, "green_owner": true, "merged": true, "merge_failed": true, "closed": true, "repair": true}
	prReasons       = map[string]bool{"": true, "conflict": true, "changes_requested": true, "checks_failed": true, "unresolved": true}
	prMergeables    = map[string]bool{"": true, "MERGEABLE": true, "CONFLICTING": true, "UNKNOWN": true}
	prReviews       = map[string]bool{"": true, "APPROVED": true, "CHANGES_REQUESTED": true, "REVIEW_REQUIRED": true}
	prCheckStatuses = map[string]bool{"QUEUED": true, "IN_PROGRESS": true, "COMPLETED": true, "WAITING": true, "PENDING": true, "REQUESTED": true}
	prCheckOutcomes = map[string]bool{"": true, "SUCCESS": true, "FAILURE": true, "NEUTRAL": true, "CANCELLED": true, "SKIPPED": true,
		"TIMED_OUT": true, "ACTION_REQUIRED": true, "STALE": true, "STARTUP_FAILURE": true, "ERROR": true, "EXPECTED": true}
)

// Only reads a small sanitized cache written by the merge sweep.
func (s *server) prStatus(w http.ResponseWriter, _ *http.Request, _ caller) error {
	writeJSON(w, http.StatusOK, readPRStatus(s.deps.PRStatusFile))
	return nil
}

func readPRStatus(path string) prStatusSnapshot {
	empty := prStatusSnapshot{PRs: []prStatus{}}
	if path == "" {
		return empty
	}
	file, err := os.Open(path)
	if err != nil {
		return empty
	}
	defer file.Close()
	var raw prStatusFile
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20+1))
	if err := decoder.Decode(&raw); err != nil {
		return empty
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return empty
	}
	clean := empty
	clean.ObservedAt = epoch(raw.At)
	if clean.ObservedAt == nil {
		return empty
	}
	seen := map[int64]bool{}
	for _, pr := range raw.PRs {
		if len(clean.PRs) == maxPRStatuses {
			break
		}
		if pr.Ticket <= 0 || seen[pr.Ticket] || !prURLPattern.MatchString(pr.URL) || !prVerdicts[pr.Verdict] || !prReasons[pr.Reason] ||
			(pr.Verdict == "repair") != (pr.Reason != "") {
			continue
		}
		seen[pr.Ticket] = true
		item := prStatus{
			TicketID: pr.Ticket, URL: pr.URL, Verdict: pr.Verdict, Reason: pr.Reason,
			MayMerge: pr.MayMerge, Acts: pr.Acts, GLWaived: pr.GLWaived, Draft: pr.Draft, Unresolved: pr.Unresolved, ManualCheck: pr.ManualCheck,
			Checks: []prCheck{},
		}
		if prMergeables[pr.Mergeable] {
			item.Mergeable = pr.Mergeable
		}
		if prReviews[pr.Review] {
			item.Review = pr.Review
		}
		if prHeadPattern.MatchString(pr.Head) {
			item.Head = pr.Head
		}
		for _, check := range pr.Checks {
			if len(item.Checks) == maxPRChecks {
				break
			}
			if !prCheckNamePattern.MatchString(check.Name) || !prCheckStatuses[check.Status] || !prCheckOutcomes[check.Conclusion] {
				continue
			}
			item.Checks = append(item.Checks, prCheck{
				Name: check.Name, Status: check.Status, Conclusion: check.Conclusion,
				StartedAt: isoTime(check.Started), CompletedAt: isoTime(check.Completed),
			})
		}
		clean.PRs = append(clean.PRs, item)
	}
	return clean
}

func isoTime(value string) *time.Time {
	if value == "" || len(value) > 40 {
		return nil
	}
	at, err := time.Parse(time.RFC3339, value)
	if err != nil || at.Year() < 2000 || at.Year() > 2200 {
		return nil
	}
	at = at.UTC()
	return &at
}
