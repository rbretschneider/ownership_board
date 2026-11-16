package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func seedTeamBuildJob(t *testing.T, c *apiClient, id string, ticketID, hostID int64, running bool) {
	t.Helper()
	now := time.Now().UTC()
	job := domain.BuildJob{ID: id, TicketID: ticketID, Repo: "acme/app", SHA: strings.Repeat("a", 40), Preset: "cmake",
		Target: domain.BuildJobTarget{OS: "windows", RequiredCapabilities: []string{}}, Status: "queued", Attempt: 1, CreatedAt: now.Add(-time.Hour), BundleBytes: 1, BundleSHA256: strings.Repeat("b", 64)}
	if err := c.store.Write(context.Background(), func(r app.Repos) error { return r.BuildJobs().Insert(context.Background(), job) }); err != nil {
		t.Fatal(err)
	}
	if running {
		if _, err := c.store.SQL().Exec(`UPDATE build_jobs SET status='running',host_id=?,started_at=?,phase='testing' WHERE id=?`,
			hostID, now.Add(-20*time.Minute).Format("2006-01-02T15:04:05.000Z"), id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTeamIsReadByPeopleAndAgents(t *testing.T) {
	browser, token := signedIn(t)
	browser.asAgent("").expect(http.StatusUnauthorized, "GET", "/api/v1/team", "")
	browser.asAgent(token).expect(http.StatusOK, "GET", "/api/v1/team", "")
	team := browser.expect(http.StatusOK, "GET", "/api/v1/team", "")
	for _, key := range []string{"runners", "tickets", "build_hosts"} {
		if list, ok := team[key].([]any); !ok || len(list) != 0 {
			t.Fatalf("%s = %v, want an empty list", key, team[key])
		}
	}
	manager := team["manager"].(map[string]any)
	if manager["open_flags"] != float64(0) || manager["latest_at"] != nil || len(manager["sessions"].([]any)) != 0 {
		t.Fatalf("manager = %v", manager)
	}
}

func TestTeamShowsRunnersTicketsHostsAndManager(t *testing.T) {
	browser, token := signedIn(t)
	browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"Release pipeline"}`)
	browser.expect(http.StatusCreated, "POST", "/api/v1/tickets", `{"kind":"story","title":"Brand scan"}`)
	worker := browser.asAgent(token)
	worker.worker = "codex_1"
	worker.expect(http.StatusOK, "POST", "/api/v1/agent/claim", "")
	worker.expect(http.StatusOK, "POST", "/api/v1/agent/tickets/1/model", `{"model":"gpt-test · refining the spec"}`)

	hostID := seedWindowsSettingsHost(t, browser, true)
	seedTeamBuildJob(t, browser, strings.Repeat("1", 64), 1, hostID, true)
	seedTeamBuildJob(t, browser, strings.Repeat("2", 64), 2, hostID, false)

	managerToken := browser.expect(http.StatusCreated, "POST", "/api/v1/tokens", `{"name":"manager","is_manager":true}`)["secret"].(string)
	browser.asAgent(managerToken).expect(http.StatusOK, "POST", "/api/v1/manager/usage", `{"cost_usd":0.25,"model":"sonnet","summary":"Reviewed the board"}`)

	team := browser.expect(http.StatusOK, "GET", "/api/v1/team", "")

	runners := team["runners"].([]any)
	if len(runners) != 1 {
		t.Fatalf("runners = %v", runners)
	}
	runner := runners[0].(map[string]any)
	if !strings.HasSuffix(runner["name"].(string), "/codex_1") || runner["state"] != "working" || runner["model"] != "gpt-test" ||
		runner["doing"] != "refining the spec" || runner["ticket_id"] != float64(1) || runner["profile_version"] != "" {
		t.Fatalf("runner = %v", runner)
	}

	tickets := team["tickets"].([]any)
	if len(tickets) != 2 {
		t.Fatalf("tickets in flight = %v", tickets)
	}
	first := tickets[0].(map[string]any)
	if first["id"] != float64(1) || first["holder"] != runner["name"] || first["build"].(map[string]any)["status"] != "running" {
		t.Fatalf("first ticket = %v", first)
	}
	if second := tickets[1].(map[string]any); second["holder"] != "" || second["build"].(map[string]any)["status"] != "queued" {
		t.Fatalf("second ticket = %v", second)
	}

	hosts := team["build_hosts"].([]any)
	if len(hosts) != 1 {
		t.Fatalf("hosts = %v", hosts)
	}
	host := hosts[0].(map[string]any)
	running, queued := host["running"].([]any), host["queued"].([]any)
	if host["os"] != "windows" || host["max_parallel"] != float64(3) || len(running) != 1 || len(queued) != 1 {
		t.Fatalf("host = %v", host)
	}
	if job := running[0].(map[string]any); job["ticket_id"] != float64(1) || job["phase"] != "testing" || job["started_at"] == nil {
		t.Fatalf("running job = %v", job)
	}
	if job := queued[0].(map[string]any); job["ticket_id"] != float64(2) || job["queued_at"] == nil {
		t.Fatalf("queued job = %v", job)
	}

	manager := team["manager"].(map[string]any)
	if manager["latest_summary"] != "Reviewed the board" || manager["latest_at"] == nil || len(manager["sessions"].([]any)) != 1 ||
		manager["today"].(map[string]any)["cost_usd"] != 0.25 {
		t.Fatalf("manager = %v", manager)
	}
}
