package reposource

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

// forgejoTaskPages bounds how far back a run or job is looked up: Forgejo's
// API lists action tasks but cannot fetch one by id.
const forgejoTaskPages = 3

// ForgejoCI reads Forgejo (and Gitea) Actions. Their API lists action tasks
// (one per job) and nothing more: logs, runners, re-runs and cancelling are
// web-UI only, and the board says so instead of pretending. Each task is
// shown as a run with one job.
type ForgejoCI struct {
	baseURL string
	getter  jsonGetter
}

// NewForgejoCI builds the client for the instance at baseURL.
func NewForgejoCI(client *http.Client, baseURL string, token secretbox.Secret) *ForgejoCI {
	headers := map[string]string{"Authorization": "token " + token.Reveal()}
	return &ForgejoCI{baseURL: baseURL, getter: jsonGetter{http: noRedirects(client), headers: headers}}
}

type forgejoTask struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	WorkflowID   string     `json:"workflow_id"`
	Event        string     `json:"event"`
	HeadBranch   string     `json:"head_branch"`
	HeadSHA      string     `json:"head_sha"`
	Status       string     `json:"status"`
	URL          string     `json:"url"`
	CreatedAt    time.Time  `json:"created_at"`
	RunStartedAt *time.Time `json:"run_started_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Runs implements app.CIReader.
func (f *ForgejoCI) Runs(ctx context.Context, repo string, query domain.CIRunQuery) ([]domain.CIRun, error) {
	tasks, err := f.tasks(ctx, repo, 1, clampLimit(query.Limit))
	if err != nil {
		return nil, err
	}
	var runs []domain.CIRun
	for _, task := range tasks {
		if query.Branch == "" || task.HeadBranch == query.Branch {
			runs = append(runs, task.toRun())
		}
	}
	return runs, nil
}

// Run implements app.CIReader.
func (f *ForgejoCI) Run(ctx context.Context, repo string, runID int64) (domain.CIRun, error) {
	task, err := f.find(ctx, repo, runID)
	if err != nil {
		return domain.CIRun{}, err
	}
	return task.toRun(), nil
}

// Job implements app.CIReader.
func (f *ForgejoCI) Job(ctx context.Context, repo string, jobID int64) (domain.CIJob, error) {
	run, err := f.Run(ctx, repo, jobID)
	if err != nil {
		return domain.CIJob{}, err
	}
	return run.Jobs[0], nil
}

// JobLogTail implements app.CIReader.
func (f *ForgejoCI) JobLogTail(context.Context, string, int64, int) (string, error) {
	return "", unsupportedOnForgejo("read job logs")
}

// BranchHead implements app.CIReader.
func (f *ForgejoCI) BranchHead(ctx context.Context, repo, branch string) (string, error) {
	ref, err := escapeRef(branch)
	if err != nil {
		return "", err
	}
	var body struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	err = f.get(ctx, repo, "/branches/"+ref, &body)
	if isNotFound(err) {
		return "", nil
	}
	return body.Commit.ID, err
}

// PullHead implements app.CIReader.
func (f *ForgejoCI) PullHead(ctx context.Context, repo string, number int) (string, string, error) {
	var body struct {
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
	}
	err := f.get(ctx, repo, fmt.Sprintf("/pulls/%d", number), &body)
	return body.Head.Ref, body.Head.SHA, err
}

// Runners implements app.CIReader.
func (f *ForgejoCI) Runners(context.Context, string) ([]domain.CIRunner, error) {
	return nil, unsupportedOnForgejo("list runners")
}

// RerunFailedJobs implements app.CIActions.
func (f *ForgejoCI) RerunFailedJobs(context.Context, string, int64) error {
	return unsupportedOnForgejo("re-run jobs")
}

// RerunRun implements app.CIActions.
func (f *ForgejoCI) RerunRun(context.Context, string, int64) error {
	return unsupportedOnForgejo("re-run runs")
}

// RerunJob implements app.CIActions.
func (f *ForgejoCI) RerunJob(context.Context, string, int64) error {
	return unsupportedOnForgejo("re-run jobs")
}

// CancelRun implements app.CIActions.
func (f *ForgejoCI) CancelRun(context.Context, string, int64) error {
	return unsupportedOnForgejo("cancel runs")
}

func (f *ForgejoCI) tasks(ctx context.Context, repo string, page, limit int) ([]forgejoTask, error) {
	var body struct {
		Tasks []forgejoTask `json:"workflow_runs"`
	}
	err := f.get(ctx, repo, fmt.Sprintf("/actions/tasks?page=%d&limit=%d", page, limit), &body)
	return body.Tasks, err
}

func (f *ForgejoCI) find(ctx context.Context, repo string, id int64) (forgejoTask, error) {
	for page := 1; page <= forgejoTaskPages; page++ {
		tasks, err := f.tasks(ctx, repo, page, forgejoPageSize)
		if err != nil {
			return forgejoTask{}, err
		}
		for _, task := range tasks {
			if task.ID == id {
				return task, nil
			}
		}
		if len(tasks) < forgejoPageSize {
			break
		}
	}
	return forgejoTask{}, domain.NotFoundf("no recent action task %d in %s", id, repo)
}

func (f *ForgejoCI) get(ctx context.Context, repo, path string, out any) error {
	if err := checkRepoName(repo); err != nil {
		return err
	}
	return f.getter.get(ctx, f.baseURL+"/api/v1/repos/"+repo+path, out)
}

func (t forgejoTask) toRun() domain.CIRun {
	status, conclusion := forgejoStatus(t.Status)
	job := domain.CIJob{ID: t.ID, RunID: t.ID, Name: t.Name, Status: status, Conclusion: conclusion,
		CreatedAt: t.CreatedAt, StartedAt: t.RunStartedAt, URL: t.URL}
	return domain.CIRun{ID: t.ID, Workflow: t.WorkflowID, Event: t.Event, HeadBranch: t.HeadBranch, HeadSHA: t.HeadSHA,
		Status: status, Conclusion: conclusion, URL: t.URL, CreatedAt: t.CreatedAt, StartedAt: t.RunStartedAt,
		UpdatedAt: t.UpdatedAt, Jobs: []domain.CIJob{job}}
}

// forgejoStatus maps a task status onto the board's status and conclusion.
func forgejoStatus(status string) (domain.CIStatus, domain.CIConclusion) {
	switch status {
	case "running":
		return domain.CIInProgress, ""
	case "success", "failure", "cancelled", "skipped":
		return domain.CICompleted, domain.CIConclusion(status)
	case "waiting", "blocked", "unknown", "":
		return domain.CIQueued, ""
	default:
		return domain.CICompleted, domain.CIOther
	}
}

func unsupportedOnForgejo(what string) error {
	return domain.Invalidf("Forgejo's API cannot %s; do it in the Forgejo web UI", what)
}
