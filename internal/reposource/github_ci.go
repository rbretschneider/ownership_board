package reposource

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

// TokenSource returns a currently valid access token. A stored GitHub App
// connection refreshes its token here; an env token never changes.
type TokenSource func(ctx context.Context) (secretbox.Secret, error)

// StaticToken is a TokenSource for a token that never changes.
func StaticToken(token secretbox.Secret) TokenSource {
	return func(context.Context) (secretbox.Secret, error) { return token, nil }
}

// GitHubCI reads and acts on GitHub Actions. The token needs Actions read
// and write, Contents read (branch heads) and Pull requests read (PR heads),
// or a classic token's repo scope; listing runners also needs Administration
// read.
type GitHubCI struct {
	apiURL string
	http   *http.Client
	token  TokenSource
}

// NewGitHubCI builds the client for the API at apiURL (empty for github.com).
func NewGitHubCI(client *http.Client, apiURL string, token TokenSource) *GitHubCI {
	return &GitHubCI{apiURL: strings.TrimRight(firstSet(apiURL, DefaultGitHubAPIURL), "/"), http: noRedirects(client), token: token}
}

type githubRun struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Event        string     `json:"event"`
	HeadBranch   string     `json:"head_branch"`
	HeadSHA      string     `json:"head_sha"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	RunAttempt   int        `json:"run_attempt"`
	HTMLURL      string     `json:"html_url"`
	CreatedAt    time.Time  `json:"created_at"`
	RunStartedAt *time.Time `json:"run_started_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type githubJob struct {
	ID          int64      `json:"id"`
	RunID       int64      `json:"run_id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	RunnerName  string     `json:"runner_name"`
	Labels      []string   `json:"labels"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	HTMLURL     string     `json:"html_url"`
	Steps       []struct {
		Name       string `json:"name"`
		Conclusion string `json:"conclusion"`
	} `json:"steps"`
}

// Runs implements app.CIReader.
func (g *GitHubCI) Runs(ctx context.Context, repo string, filter domain.CIRunQuery) ([]domain.CIRun, error) {
	query := url.Values{"per_page": {fmt.Sprint(clampLimit(filter.Limit))}}
	if filter.Branch != "" {
		query.Set("branch", filter.Branch)
	}
	var body struct {
		Runs []githubRun `json:"workflow_runs"`
	}
	if err := g.get(ctx, repo, "/actions/runs?"+query.Encode(), &body); err != nil {
		return nil, err
	}
	runs := make([]domain.CIRun, 0, len(body.Runs))
	for _, run := range body.Runs {
		runs = append(runs, run.toDomain())
	}
	return runs, nil
}

// Run implements app.CIReader.
func (g *GitHubCI) Run(ctx context.Context, repo string, runID int64) (domain.CIRun, error) {
	var run githubRun
	if err := g.get(ctx, repo, fmt.Sprintf("/actions/runs/%d", runID), &run); err != nil {
		return domain.CIRun{}, err
	}
	var body struct {
		Jobs []githubJob `json:"jobs"`
	}
	if err := g.get(ctx, repo, fmt.Sprintf("/actions/runs/%d/jobs?filter=latest&per_page=%d", runID, ciListLimit), &body); err != nil {
		return domain.CIRun{}, err
	}
	out := run.toDomain()
	out.Jobs = make([]domain.CIJob, 0, len(body.Jobs))
	for _, job := range body.Jobs {
		out.Jobs = append(out.Jobs, job.toDomain())
	}
	return out, nil
}

// Job implements app.CIReader.
func (g *GitHubCI) Job(ctx context.Context, repo string, jobID int64) (domain.CIJob, error) {
	var job githubJob
	if err := g.get(ctx, repo, fmt.Sprintf("/actions/jobs/%d", jobID), &job); err != nil {
		return domain.CIJob{}, err
	}
	return job.toDomain(), nil
}

// JobLogTail implements app.CIReader. GitHub answers with a redirect to a
// short-lived signed download, which is fetched without the token.
func (g *GitHubCI) JobLogTail(ctx context.Context, repo string, jobID int64, maxBytes int) (string, error) {
	getter, err := g.getter(ctx)
	if err != nil {
		return "", err
	}
	target, err := g.repoURL(repo, fmt.Sprintf("/actions/jobs/%d/logs", jobID))
	if err != nil {
		return "", err
	}
	download, err := getter.redirectTarget(ctx, target)
	if err != nil {
		return "", err
	}
	return fetchTail(ctx, g.http, download, maxBytes)
}

// BranchHead implements app.CIReader.
func (g *GitHubCI) BranchHead(ctx context.Context, repo, branch string) (string, error) {
	ref, err := escapeRef(branch)
	if err != nil {
		return "", err
	}
	var body struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	err = g.get(ctx, repo, "/git/ref/heads/"+ref, &body)
	if isNotFound(err) {
		return "", nil
	}
	return body.Object.SHA, err
}

// PullHead implements app.CIReader.
func (g *GitHubCI) PullHead(ctx context.Context, repo string, number int) (string, string, error) {
	var body struct {
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
	}
	err := g.get(ctx, repo, fmt.Sprintf("/pulls/%d", number), &body)
	return body.Head.Ref, body.Head.SHA, err
}

// Runners implements app.CIReader.
func (g *GitHubCI) Runners(ctx context.Context, repo string) ([]domain.CIRunner, error) {
	var body struct {
		Runners []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Busy   bool   `json:"busy"`
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
		} `json:"runners"`
	}
	if err := g.get(ctx, repo, fmt.Sprintf("/actions/runners?per_page=%d", ciListLimit), &body); err != nil {
		return nil, err
	}
	runners := make([]domain.CIRunner, 0, len(body.Runners))
	for _, runner := range body.Runners {
		labels := make([]string, 0, len(runner.Labels))
		for _, label := range runner.Labels {
			labels = append(labels, label.Name)
		}
		runners = append(runners, domain.CIRunner{Name: runner.Name, IsOnline: runner.Status == "online", IsBusy: runner.Busy, Labels: labels})
	}
	return runners, nil
}

// RerunFailedJobs implements app.CIActions.
func (g *GitHubCI) RerunFailedJobs(ctx context.Context, repo string, runID int64) error {
	return g.post(ctx, repo, fmt.Sprintf("/actions/runs/%d/rerun-failed-jobs", runID))
}

// RerunRun implements app.CIActions.
func (g *GitHubCI) RerunRun(ctx context.Context, repo string, runID int64) error {
	return g.post(ctx, repo, fmt.Sprintf("/actions/runs/%d/rerun", runID))
}

// RerunJob implements app.CIActions.
func (g *GitHubCI) RerunJob(ctx context.Context, repo string, jobID int64) error {
	return g.post(ctx, repo, fmt.Sprintf("/actions/jobs/%d/rerun", jobID))
}

// CancelRun implements app.CIActions.
func (g *GitHubCI) CancelRun(ctx context.Context, repo string, runID int64) error {
	return g.post(ctx, repo, fmt.Sprintf("/actions/runs/%d/cancel", runID))
}

func (g *GitHubCI) get(ctx context.Context, repo, path string, out any) error {
	getter, err := g.getter(ctx)
	if err != nil {
		return err
	}
	target, err := g.repoURL(repo, path)
	if err != nil {
		return err
	}
	return getter.get(ctx, target, out)
}

func (g *GitHubCI) post(ctx context.Context, repo, path string) error {
	getter, err := g.getter(ctx)
	if err != nil {
		return err
	}
	target, err := g.repoURL(repo, path)
	if err != nil {
		return err
	}
	return getter.post(ctx, target)
}

func (g *GitHubCI) repoURL(repo, path string) (string, error) {
	if err := CheckRepoName(repo); err != nil {
		return "", err
	}
	return g.apiURL + "/repos/" + repo + path, nil
}

func (g *GitHubCI) getter(ctx context.Context) (jsonGetter, error) {
	token, err := g.token(ctx)
	if err != nil {
		return jsonGetter{}, err
	}
	return jsonGetter{http: g.http, headers: map[string]string{
		"Authorization":        "Bearer " + token.Reveal(),
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}}, nil
}

func (r githubRun) toDomain() domain.CIRun {
	return domain.CIRun{ID: r.ID, Workflow: r.Name, Event: r.Event, HeadBranch: r.HeadBranch, HeadSHA: r.HeadSHA,
		Status: githubStatus(r.Status), Conclusion: githubConclusion(r.Conclusion), Attempt: r.RunAttempt, URL: r.HTMLURL,
		CreatedAt: r.CreatedAt, StartedAt: r.RunStartedAt, UpdatedAt: r.UpdatedAt}
}

func (j githubJob) toDomain() domain.CIJob {
	var failed []string
	for _, step := range j.Steps {
		if githubConclusion(step.Conclusion).IsFailure() {
			failed = append(failed, step.Name)
		}
	}
	return domain.CIJob{ID: j.ID, RunID: j.RunID, Name: j.Name, Status: githubStatus(j.Status), Conclusion: githubConclusion(j.Conclusion),
		RunnerName: j.RunnerName, Labels: j.Labels, CreatedAt: j.CreatedAt, StartedAt: j.StartedAt, CompletedAt: j.CompletedAt,
		FailedSteps: failed, URL: j.HTMLURL}
}

// githubStatus maps GitHub's statuses; everything before a runner starts
// (queued, waiting, pending, requested) is queued.
func githubStatus(status string) domain.CIStatus {
	switch status {
	case "completed":
		return domain.CICompleted
	case "in_progress":
		return domain.CIInProgress
	default:
		return domain.CIQueued
	}
}

func githubConclusion(conclusion string) domain.CIConclusion {
	switch conclusion {
	case "":
		return ""
	case "success", "failure", "cancelled", "skipped", "timed_out":
		return domain.CIConclusion(conclusion)
	case "startup_failure":
		// The workflow file itself broke: a failure, not a pass.
		return domain.CIFailure
	default:
		return domain.CIOther
	}
}

func clampLimit(limit int) int {
	if limit <= 0 || limit > ciListLimit {
		return ciListLimit
	}
	return limit
}
