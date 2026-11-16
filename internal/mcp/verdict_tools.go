package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type getVerdicts struct{ s Services }

func (getVerdicts) Definition() Definition {
	return Definition{
		Name: "get_verdicts",
		Description: "Read the board's verification verdicts: whether a commit passed, failed, or passed as a clean merge of a passed commit, per build target. " +
			"The board is the only record of a pass; read it here instead of keeping your own. Give repo and sha, or ticket_id.",
		InputSchema: object(map[string]any{
			"repo":      prop("string", "The repo's full name, as on the ticket (owner/name)."),
			"sha":       prop("string", "The full commit hash."),
			"ticket_id": prop("integer", "List the ticket's verdicts instead, newest first."),
		}),
	}
}

func (t getVerdicts) Call(ctx context.Context, _ domain.Actor, raw json.RawMessage) (string, error) {
	var args struct {
		Repo     string `json:"repo"`
		SHA      string `json:"sha"`
		TicketID int64  `json:"ticket_id"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return "", err
	}
	var verdicts []domain.Verdict
	var err error
	switch {
	case args.TicketID != 0 && args.Repo == "" && args.SHA == "":
		verdicts, err = t.s.Verdicts.ForTicket(ctx, args.TicketID)
	case args.TicketID == 0 && args.Repo != "" && args.SHA != "":
		verdicts, err = t.s.Verdicts.ForCommit(ctx, args.Repo, args.SHA)
	default:
		return "", domain.Invalidf("give either repo and sha, or ticket_id")
	}
	if err != nil {
		return "", err
	}
	if len(verdicts) == 0 {
		return "No verdict recorded: the commit has not been verified on any build target.", nil
	}
	return renderVerdicts(verdicts), nil
}

type reportCleanMerge struct{ s Services }

func (reportCleanMerge) Definition() Definition {
	return Definition{
		Name: "report_clean_merge",
		Description: "Record that commit sha is a conflict-free merge of the default branch into merge_of, a commit that already passed. " +
			"sha then counts as passed without another build-host run. You must hold the lease on the ticket; the board records you as the reporter. " +
			"Refused when merge_of has no pass, or when sha already failed. Only for a merge with no conflicts and no other change.",
		InputSchema: object(map[string]any{
			"ticket_id": ticketIDProp(),
			"repo":      prop("string", "The ticket's repo full name (owner/name)."),
			"sha":       prop("string", "The merge commit's full hash."),
			"merge_of":  prop("string", "The full hash of the passed commit the default branch was merged into."),
			"target":    prop("string", "One build target, e.g. windows or windows:msvc. Omit to apply to every target merge_of passed."),
		}, "ticket_id", "repo", "sha", "merge_of"),
	}
}

func (t reportCleanMerge) Call(ctx context.Context, actor domain.Actor, raw json.RawMessage) (string, error) {
	var report domain.CleanMergeReport
	if err := decodeArguments(raw, &report); err != nil {
		return "", err
	}
	verdicts, err := t.s.Verdicts.ReportCleanMerge(ctx, actor, report)
	if err != nil {
		return "", err
	}
	return "Recorded.\n" + renderVerdicts(verdicts), nil
}

func renderVerdicts(verdicts []domain.Verdict) string {
	var b strings.Builder
	for _, v := range verdicts {
		status := string(v.Status)
		if v.Status == domain.VerdictCleanMerge {
			status = "passed (clean merge of " + v.MergeOf + ")"
		}
		fmt.Fprintf(&b, "- %s %s on %s: %s; ticket #%d, job %s, %s by %s at %s\n", v.Repo, v.SHA, v.Target, status,
			v.TicketID, v.JobID, v.Source, v.ReportedBy, v.RecordedAt.UTC().Format("2006-01-02 15:04Z"))
	}
	return b.String()
}
