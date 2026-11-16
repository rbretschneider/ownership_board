package reposource_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/reposource"
	"github.com/rbretschneider/ownership_board/internal/secretbox"
)

// githubCIServer answers like GitHub's Actions API for acme/api.
func githubCIServer(t *testing.T, posts *[]string, blobAuth *string) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/acme/api/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("branch") != "feat/x" {
			http.Error(w, "branch filter missing", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"workflow_runs":[{"id":5,"name":"CI","event":"pull_request","head_branch":"feat/x","head_sha":"abc1234def","status":"waiting",
			"conclusion":null,"run_attempt":1,"created_at":"2026-11-16T10:00:00Z","updated_at":"2026-11-16T10:00:00Z"},
			{"id":4,"name":"CI","head_branch":"feat/x","status":"completed","conclusion":"startup_failure","created_at":"2026-11-16T09:00:00Z","updated_at":"2026-11-16T09:00:00Z"}]}`)
	})
	mux.HandleFunc("GET /repos/acme/api/actions/runs/5", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":5,"name":"CI","head_branch":"feat/x","status":"in_progress","created_at":"2026-11-16T10:00:00Z","updated_at":"2026-11-16T10:00:00Z"}`)
	})
	mux.HandleFunc("GET /repos/acme/api/actions/runs/5/jobs", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"jobs":[{"id":50,"run_id":5,"name":"windows","status":"completed","conclusion":"failure","labels":["self-hosted"],
			"created_at":"2026-11-16T10:00:00Z","steps":[{"name":"checkout","conclusion":"success"},{"name":"verify","conclusion":"failure"}]}]}`)
	})
	mux.HandleFunc("GET /repos/acme/api/git/ref/heads/feat/x", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"object":{"sha":"abc1234def"}}`)
	})
	mux.HandleFunc("GET /repos/acme/api/actions/jobs/50/logs", func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, server.URL+"/blob/log?sig=1", http.StatusFound)
	})
	mux.HandleFunc("GET /blob/log", func(w http.ResponseWriter, r *http.Request) {
		*blobAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, strings.Repeat("2026-11-16T10:01:00.0000000Z noise line\n", 500)+"2026-11-16T10:02:00.1234567Z \x1b[31merror: verify failed\x1b[0m\n")
	})
	mux.HandleFunc("POST /repos/acme/api/actions/runs/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		*posts = append(*posts, r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
	})
	server = httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	return server
}

func newGitHubCI(server *httptest.Server) *reposource.GitHubCI {
	return reposource.NewGitHubCI(server.Client(), server.URL, reposource.StaticToken(secretbox.NewSecret("gh-token")))
}

func TestGitHubCIMapsRunsAndJobsOntoTheBoardsModel(t *testing.T) {
	var posts []string
	var blobAuth string
	ci := newGitHubCI(githubCIServer(t, &posts, &blobAuth))
	runs, err := ci.Runs(context.Background(), "acme/api", domain.CIRunQuery{Branch: "feat/x", Limit: 10})
	if err != nil {
		t.Fatalf("runs: %v", err)
	}
	if len(runs) != 2 || runs[0].Status != domain.CIQueued || runs[1].Conclusion != domain.CIFailure {
		t.Fatalf("runs = %+v, want waiting as queued and startup_failure as a failure", runs)
	}
	run, err := ci.Run(context.Background(), "acme/api", 5)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(run.Jobs) != 1 || run.Jobs[0].FailedSteps[0] != "verify" || run.Jobs[0].Labels[0] != "self-hosted" {
		t.Fatalf("run = %+v, want job 50 with its failed step and labels", run)
	}
}

func TestGitHubCILogTailDropsTheTokenAndTheNoise(t *testing.T) {
	var posts []string
	var blobAuth string
	ci := newGitHubCI(githubCIServer(t, &posts, &blobAuth))
	tail, err := ci.JobLogTail(context.Background(), "acme/api", 50, 200)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if blobAuth != "" {
		t.Fatalf("the signed log download got Authorization %q; the token must never follow a redirect", blobAuth)
	}
	if !strings.HasSuffix(tail, "error: verify failed") || strings.Contains(tail, "2026-11-16T") || strings.Contains(tail, "\x1b") || len(tail) > 200 {
		t.Fatalf("tail = %q, want the last lines without timestamps or colour codes", tail)
	}
}

func TestGitHubCIActionsPostToTheRunAndBranchHeadsResolve(t *testing.T) {
	var posts []string
	var blobAuth string
	ci := newGitHubCI(githubCIServer(t, &posts, &blobAuth))
	if err := ci.CancelRun(context.Background(), "acme/api", 4); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := ci.RerunFailedJobs(context.Background(), "acme/api", 4); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if strings.Join(posts, ",") != "/repos/acme/api/actions/runs/4/cancel,/repos/acme/api/actions/runs/4/rerun-failed-jobs" {
		t.Fatalf("posts = %v", posts)
	}
	head, err := ci.BranchHead(context.Background(), "acme/api", "feat/x")
	if err != nil || head != "abc1234def" {
		t.Fatalf("head = %q, %v", head, err)
	}
	if missing, err := ci.BranchHead(context.Background(), "acme/api", "gone"); err != nil || missing != "" {
		t.Fatalf("missing branch = %q, %v; want empty and no error", missing, err)
	}
}

func TestGitHubCIRefusesNamesThatWalkOffTheAPI(t *testing.T) {
	var posts []string
	var blobAuth string
	ci := newGitHubCI(githubCIServer(t, &posts, &blobAuth))
	if _, err := ci.BranchHead(context.Background(), "acme/api", "../../user"); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("err = %v, want a dot-dot branch refused", err)
	}
	if _, err := ci.Runs(context.Background(), "acme/../api", domain.CIRunQuery{}); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("err = %v, want a dot-dot repo refused", err)
	}
}
