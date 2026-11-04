package domain

import (
	"testing"
	"time"
)

var worker = Actor{Kind: ActorAgent, ID: 7, Name: "codex_1"}

func stageMove(from, to Stage) TimelineEntry {
	return NewEntry(1, TimelineStage, SystemActor, time.Time{}).With("from", string(from)).With("to", string(to))
}

func modelReport(model string) TimelineEntry {
	return NewEntry(1, TimelineEdited, worker, time.Time{}).With("fields", []string{"working model"}).With("model", model)
}

func queued(sha string) TimelineEntry {
	return NewEntry(1, TimelineComment, worker, time.Time{}).
		WithBody("Windows verification queued for commit " + sha + " (dotnet). Uses the shared Windows PC, not hosted CI minutes.")
}

const author = "gpt-6.1-sol"

// windowsRun is one verification of commit sha as the worker reports it.
func windowsRun(sha, verdict string) []TimelineEntry {
	return []TimelineEntry{
		queued(sha),
		modelReport(author + " · build-server:queued:Windows x64 · awaiting Windows"),
		modelReport(author + " · build-server:building:Windows x64 · awaiting Windows"),
		modelReport(author + " · build-server:testing:Windows x64 · awaiting Windows"),
		modelReport(author + " · " + verdict),
	}
}

func join(parts ...[]TimelineEntry) []TimelineEntry {
	var out []TimelineEntry
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func TestCountStageAttempts(t *testing.T) {
	toBuild := []TimelineEntry{stageMove(StageDesign, StageBuild)}
	review := []TimelineEntry{modelReport("gpt-6.1-sol-review · adversarial review")}
	authoring := []TimelineEntry{modelReport(author)}
	cases := map[string]struct {
		entries []TimelineEntry
		want    StageAttempts
	}{
		"no history": {nil, StageAttempts{}},
		"refining only": {
			[]TimelineEntry{stageMove(StageRefine, StageSpecReview), modelReport(author)},
			StageAttempts{},
		},
		"clean run to merge": {
			join(toBuild, []TimelineEntry{modelReport(author + " · preparing workspace")},
				windowsRun("aaaaaaa", "awaiting review"), review,
				[]TimelineEntry{stageMove(StageBuild, StagePR), stageMove(StagePR, StageDone)}),
			StageAttempts{Build: 1, Test: 1, Review: 1, PR: 1},
		},
		"windows failures then review fixes": {
			join(toBuild,
				windowsRun("aaaaaaa", "awaiting repair"), authoring,
				windowsRun("bbbbbbb", "awaiting repair"), authoring,
				windowsRun("ccccccc", "awaiting review"), review, authoring,
				windowsRun("ddddddd", "awaiting review"), review),
			StageAttempts{Build: 4, Test: 4, TestFailures: 2, Review: 2, ReviewFixes: 1},
		},
		"re-sending an unchanged commit is the same run": {
			join(toBuild,
				windowsRun("aaaaaaa", "awaiting repair"), authoring,
				windowsRun("aaaaaaa", "awaiting repair"), authoring,
				windowsRun("bbbbbbb", "awaiting review"),
				windowsRun("bbbbbbb", "awaiting review"),
				windowsRun("bbbbbbb", "awaiting review")),
			StageAttempts{Build: 2, Test: 2, TestFailures: 1},
		},
		"a run in flight counts": {
			join(toBuild, windowsRun("aaaaaaa", "awaiting repair"), authoring,
				[]TimelineEntry{queued("bbbbbbb"), modelReport(author + " · build-server:queued:Windows x64 · awaiting Windows")}),
			StageAttempts{Build: 2, Test: 2, TestFailures: 1},
		},
		"a failure with no commit sent is not counted": {
			join(toBuild, []TimelineEntry{modelReport(author + " · awaiting repair")}),
			StageAttempts{Build: 1},
		},
		"review in flight is not a fix": {
			join(toBuild, windowsRun("aaaaaaa", "awaiting review"), review),
			StageAttempts{Build: 1, Test: 1, Review: 1},
		},
		"review model change mid-round is one round": {
			join(toBuild, review, []TimelineEntry{modelReport("Claude Sonnet 4.6 · adversarial review")}),
			StageAttempts{Build: 1, Review: 1},
		},
		"CI repair rebuilds and reopens the PR": {
			join(toBuild, review,
				[]TimelineEntry{stageMove(StageBuild, StagePR), stageMove(StagePR, StageBuild)}, authoring, review,
				[]TimelineEntry{stageMove(StageBuild, StagePR)}),
			StageAttempts{Build: 2, Review: 2, PR: 2, PRRepairs: 1},
		},
		"other entries are ignored": {
			[]TimelineEntry{
				NewEntry(1, TimelineComment, worker, time.Time{}).WithBody("Pull request repair."),
				NewEntry(1, TimelineComment, Actor{Kind: ActorHuman, Name: "Owner"}, time.Time{}).
					WithBody("Windows verification queued for commit aaaaaaa (dotnet)."),
				NewEntry(1, TimelineEdited, worker, time.Time{}).With("fields", []string{"title"}),
			},
			StageAttempts{},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := CountStageAttempts(tc.entries); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
