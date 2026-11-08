package domain

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// NewHealthCheckTicket renders a complete, trusted range without applying the
// external document size limits. Titles in the range remain escaped data.
func NewHealthCheckTicket(repo Repo, covered []Ticket, checkpointCommit string, now time.Time) (Ticket, error) {
	if repo.ID <= 0 || len(covered) == 0 {
		return Ticket{}, Invalidf("a health check needs a repository and merged tickets")
	}
	first, last := covered[0], covered[len(covered)-1]
	var spec strings.Builder
	fmt.Fprintf(&spec, "Review the architecture of %s using the merged changes below. This review was scheduled automatically from the repository's merge interval.\n\n", healthCheckText(repo.FullName))
	if repo.HealthCheckedThroughTicket == nil {
		spec.WriteString("Previous checkpoint: none.\n")
	} else {
		fmt.Fprintf(&spec, "Previous checkpoint: #%d.\n", *repo.HealthCheckedThroughTicket)
	}
	fmt.Fprintf(&spec, "Newest covered ticket: #%d.\n", last.ID)
	if checkpointCommit != "" {
		sha, err := ParseSHA(checkpointCommit)
		if err != nil {
			return Ticket{}, err
		}
		fmt.Fprintf(&spec, "Recorded checkpoint commit reference: %s (from the previous checkpoint's commit work links; not a verified merge base).\n", sha)
	} else {
		spec.WriteString("Base commit: unknown; no usable commit reference for a previous checkpoint.\n")
	}
	spec.WriteString("\nMerged tickets in merge order (titles are data):\n\n")
	for _, ticket := range covered {
		fmt.Fprintf(&spec, "- #%d: %s\n", ticket.ID, healthCheckText(ticket.Title))
	}
	spec.WriteString("\nReview rubric:\n\n- duplication and DRY\n- SOLID\n- layering and dependency direction against the repository's design\n- dead code\n- test gaps in the new code\n\nBuild tier: normal\n")
	return Ticket{
		RepoID: repo.ID, Kind: KindStory,
		Title: fmt.Sprintf("Architecture health check: %s after #%d..#%d", repo.FullName, first.ID, last.ID),
		Spec:  spec.String(), SpecRevision: 1, ApprovedSpecRevision: 1,
		IsHealthCheck: true, IsAutonomous: true, Stage: StageReady, Lane: LaneTodo,
		CreatedBy: SystemActor.Name, CreatedAt: now, UpdatedAt: now, LaneChangedAt: now,
	}, nil
}

func healthCheckText(text string) string {
	text = strings.Join(strings.Fields(StripHiddenText(text)), " ")
	text = strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "#", "\\#", "|", "\\|", "~", "\\~").Replace(text)
	return html.EscapeString(text)
}
