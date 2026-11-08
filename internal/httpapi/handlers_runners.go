package httpapi

import (
	"encoding/json"
	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// The Windows build host's live state, written by the verification queue on
// the server as a small sanitized file (like account usage). The board only
// reads it; nothing here can start, stop or change a build.

type runnerSlot struct {
	Slot      int        `json:"slot"`
	TicketID  *int64     `json:"ticket_id"`
	Repo      string     `json:"repo"`
	SHA       string     `json:"sha"`
	Preset    string     `json:"preset"`
	Phase     string     `json:"phase"`
	Detail    string     `json:"detail"`
	Since     *time.Time `json:"since"`
	UpdatedAt *time.Time `json:"updated_at"`
	// Orphan: a newer commit of the same ticket replaced this run; the host stops it.
	Orphan bool `json:"orphan"`
}

type runnerQueued struct {
	Position      int        `json:"position"`
	TicketID      *int64     `json:"ticket_id"`
	Repo          string     `json:"repo"`
	SHA           string     `json:"sha"`
	Preset        string     `json:"preset"`
	QueuedAt      *time.Time `json:"queued_at"`
	DeferredUntil *time.Time `json:"deferred_until"`
}

type registryRunnerHost struct {
	runnersSnapshot
	ID     int64              `json:"id"`
	OS     domain.BuildHostOS `json:"os"`
	Online bool               `json:"online"`
}

type runnersSnapshot struct {
	Hosts          []registryRunnerHost `json:"hosts,omitempty"`
	ObservedAt     *time.Time           `json:"observed_at"`
	Host           string               `json:"host"`
	Enabled        bool                 `json:"enabled"`
	ParallelBuilds int                  `json:"parallel_builds"`
	// Blocked says why slots are not taking board work; empty when they are.
	Blocked      string         `json:"blocked"`
	BlockedSince *time.Time     `json:"blocked_since"`
	Slots        []runnerSlot   `json:"slots"`
	Queue        []runnerQueued `json:"queue"`
}

// The file's own shape: epoch seconds, as the queue writes them.
type runnersFile struct {
	At             float64 `json:"at"`
	Host           string  `json:"host"`
	Enabled        bool    `json:"enabled"`
	ParallelBuilds int     `json:"parallel_builds"`
	Blocked        string  `json:"blocked"`
	BlockedSince   float64 `json:"blocked_since"`
	Slots          []struct {
		Slot    int     `json:"slot"`
		Ticket  *int64  `json:"ticket"`
		Repo    string  `json:"repo"`
		SHA     string  `json:"sha"`
		Preset  string  `json:"preset"`
		Phase   string  `json:"phase"`
		Detail  string  `json:"detail"`
		Since   float64 `json:"since"`
		Updated float64 `json:"updated"`
		Orphan  bool    `json:"orphan"`
	} `json:"slots"`
	Queue []struct {
		Ticket     *int64  `json:"ticket"`
		Repo       string  `json:"repo"`
		SHA        string  `json:"sha"`
		Preset     string  `json:"preset"`
		Queued     float64 `json:"queued"`
		RetryAfter float64 `json:"retry_after"`
	} `json:"queue"`
}

const (
	maxRunnerSlots  = 8
	maxRunnerQueue  = 40
	maxRunnerDetail = 60
)

var (
	runnerRepoPattern   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	runnerSHAPattern    = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	runnerHostPattern   = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	runnerDetailPattern = regexp.MustCompile(`^[A-Za-z0-9 .·:()/#+-]*$`)
	runnerPhases        = map[string]bool{"queued": true, "preparing": true, "building": true, "testing": true, "verifying": true}
	runnerPresets       = map[string]bool{"dotnet": true, "cmake": true, "script": true}
	runnerBlocks        = map[string]bool{"github-actions": true, "disabled": true, "low-disk": true, "no-settings": true, "drained": true}
)

// Prefer approved registry identities; preserve the file snapshot during migration.
func (s *server) windowsRunners(w http.ResponseWriter, r *http.Request, who caller) error {
	snapshot, err := s.deps.BuildHosts.CompatibilityRunners(r.Context(), who.actor)
	if err != nil {
		return err
	}
	if len(snapshot.Hosts) == 0 {
		writeJSON(w, http.StatusOK, readRunnersSnapshot(s.deps.RunnersFile))
		return nil
	}
	hosts := make([]registryRunnerHost, 0, len(snapshot.Hosts))
	selected := 0
	windowsSelected := false
	for i, row := range snapshot.Hosts {
		hosts = append(hosts, toCompatibilityRunner(row, snapshot.ObservedAt))
		if row.Host.OS == "windows" && !windowsSelected {
			selected = i
			windowsSelected = true
		}
	}
	out := hosts[selected].runnersSnapshot
	out.Hosts = hosts
	writeJSON(w, http.StatusOK, out)
	return nil
}

func toCompatibilityRunner(row app.BuildHostRunner, now time.Time) registryRunnerHost {
	h := row.Host
	snapshot := runnersSnapshot{ObservedAt: h.LastSeenAt, Host: h.Name, Enabled: h.Enabled, ParallelBuilds: h.MaxParallel, Slots: []runnerSlot{}, Queue: []runnerQueued{}}
	if !h.Enabled {
		snapshot.Blocked = "disabled"
	}
	for i, a := range row.Jobs {
		phase := a.Phase
		if !runnerPhases[phase] {
			phase = "verifying"
		}
		snapshot.Slots = append(snapshot.Slots, runnerSlot{Slot: i + 1, TicketID: positiveID(&a.TicketID), Repo: a.Repo, SHA: a.SHA[:min(len(a.SHA), 12)], Preset: a.Preset, Phase: phase, Detail: cleanDetail(a.Detail), Since: a.StartedAt, UpdatedAt: h.LastSeenAt, Orphan: a.Status == "cancelled" || a.Status == "superseded"})
	}
	for i, j := range row.Queue {
		snapshot.Queue = append(snapshot.Queue, runnerQueued{Position: i + 1, TicketID: positiveID(&j.TicketID), Repo: j.Repo, SHA: j.SHA[:min(len(j.SHA), 12)], Preset: j.Preset, QueuedAt: &j.CreatedAt})
	}
	return registryRunnerHost{runnersSnapshot: snapshot, ID: h.ID, OS: h.OS, Online: h.Online(now)}
}

func readRunnersSnapshot(path string) runnersSnapshot {
	empty := runnersSnapshot{Slots: []runnerSlot{}, Queue: []runnerQueued{}}
	if path == "" {
		return empty
	}
	file, err := os.Open(path)
	if err != nil {
		return empty
	}
	defer file.Close()
	var raw runnersFile
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
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
	if runnerHostPattern.MatchString(raw.Host) {
		clean.Host = raw.Host
	}
	clean.Enabled = raw.Enabled
	if runnerBlocks[raw.Blocked] {
		clean.Blocked = raw.Blocked
		clean.BlockedSince = epoch(raw.BlockedSince)
	}
	if raw.ParallelBuilds >= 0 && raw.ParallelBuilds <= maxRunnerSlots {
		clean.ParallelBuilds = raw.ParallelBuilds
	}
	seen := map[int]bool{}
	for _, slot := range raw.Slots {
		if len(clean.Slots) == maxRunnerSlots {
			break
		}
		if slot.Slot < 1 || slot.Slot > maxRunnerSlots || seen[slot.Slot] || !runnerRepoPattern.MatchString(slot.Repo) ||
			!runnerSHAPattern.MatchString(slot.SHA) || !runnerPresets[slot.Preset] || !runnerPhases[slot.Phase] {
			continue
		}
		seen[slot.Slot] = true
		clean.Slots = append(clean.Slots, runnerSlot{
			Slot: slot.Slot, TicketID: positiveID(slot.Ticket), Repo: slot.Repo, SHA: slot.SHA[:min(len(slot.SHA), 12)],
			Preset: slot.Preset, Phase: slot.Phase, Detail: cleanDetail(slot.Detail),
			Since: epoch(slot.Since), UpdatedAt: epoch(slot.Updated), Orphan: slot.Orphan,
		})
	}
	for _, queued := range raw.Queue {
		if len(clean.Queue) == maxRunnerQueue {
			break
		}
		if !runnerRepoPattern.MatchString(queued.Repo) || !runnerSHAPattern.MatchString(queued.SHA) || !runnerPresets[queued.Preset] {
			continue
		}
		clean.Queue = append(clean.Queue, runnerQueued{
			Position: len(clean.Queue) + 1, TicketID: positiveID(queued.Ticket), Repo: queued.Repo,
			SHA: queued.SHA[:min(len(queued.SHA), 12)], Preset: queued.Preset,
			QueuedAt: epoch(queued.Queued), DeferredUntil: epoch(queued.RetryAfter),
		})
	}
	return clean
}

func epoch(seconds float64) *time.Time {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds > 1e11 {
		return nil
	}
	at := time.Unix(0, int64(seconds*1e9)).UTC()
	return &at
}

func positiveID(id *int64) *int64 {
	if id == nil || *id <= 0 {
		return nil
	}
	return id
}

func cleanDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len([]rune(detail)) > maxRunnerDetail || !runnerDetailPattern.MatchString(detail) {
		return ""
	}
	return detail
}
