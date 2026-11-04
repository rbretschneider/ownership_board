package stats

import (
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Report is the product owner's view of delivery over a window, split by who
// started the work: autonomous tickets (filed from an agreed chat and run
// with no stops) versus the owner's own tickets.
type Report struct {
	GeneratedAt time.Time       `json:"generated_at"`
	WindowDays  int             `json:"window_days"`
	Since       time.Time       `json:"since"`
	All         Segment         `json:"all"`
	Autonomous  Segment         `json:"autonomous"`
	Owner       Segment         `json:"owner"`
	Repos       []RepoBreakdown `json:"repos"`
}

// Segment holds the metrics for one group of tickets. Counts named "in the
// window" use the report's window; WIP, Blocked and NeedsOwner are as of now.
type Segment struct {
	// MergedRecent counts tickets that reached done in the last 24 hours.
	MergedRecent int `json:"merged_24h"`
	// Merged counts tickets that reached done in the window.
	Merged int `json:"merged"`
	// PRsOpened counts tickets that first reached review in the window.
	PRsOpened int `json:"prs_opened"`
	// Created counts tickets filed in the window.
	Created int        `json:"created"`
	Daily   []DayCount `json:"daily"`
	// LeadTime is created to merged, for tickets merged in the window.
	LeadTime Distribution `json:"lead_time"`
	// CycleTime is first in progress to merged, for tickets merged in the
	// window.
	CycleTime Distribution `json:"cycle_time"`
	// StageTimes summarises finished stays in each stage that ended in the
	// window, in workflow order.
	StageTimes []StageTime  `json:"stage_times"`
	WIP        []StageCount `json:"wip"`
	Blocked    int          `json:"blocked"`
	NeedsOwner int          `json:"needs_owner"`
	// RepairRounds counts returns from review to build in the window;
	// RepairedTickets counts the tickets they happened to.
	RepairRounds    int `json:"repair_rounds"`
	RepairedTickets int `json:"repaired_tickets"`
	// ChangesRequested counts the owner's checkpoint rejections in the window.
	ChangesRequested int `json:"changes_requested"`
	// Models counts tickets per working model agents reported in the window.
	Models []NamedCount `json:"models"`
	// Claims counts agent claims in the window per action.
	Claims []NamedCount `json:"claims"`
	// Agents counts agent claims in the window per worker.
	Agents []NamedCount `json:"agents"`
}

// DayCount is the number of tickets merged on one local day.
type DayCount struct {
	Date   string `json:"date"`
	Merged int    `json:"merged"`
}

// StageTime is how long tickets stayed in one stage.
type StageTime struct {
	Stage domain.Stage `json:"stage"`
	Distribution
}

// StageCount is the number of open tickets in one stage now.
type StageCount struct {
	Stage domain.Stage `json:"stage"`
	Lane  domain.Lane  `json:"lane"`
	Count int          `json:"count"`
}

// NamedCount is a tally for one name, such as a model or an action.
type NamedCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// RepoBreakdown is one repo's delivery, split like the report.
type RepoBreakdown struct {
	// RepoID is zero for tickets without a repo.
	RepoID     int64      `json:"repo_id"`
	Name       string     `json:"name"`
	Autonomous RepoCounts `json:"autonomous"`
	Owner      RepoCounts `json:"owner"`
}

// RepoCounts is one repo's delivery for one group of tickets.
type RepoCounts struct {
	Merged    int `json:"merged"`
	PRsOpened int `json:"prs_opened"`
	Open      int `json:"open"`
}
