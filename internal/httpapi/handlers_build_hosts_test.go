package httpapi_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func seedBuildHost(t *testing.T, c *apiClient) int64 {
	t.Helper()
	h, err := domain.NewBuildHost(domain.BuildHostDraft{Name: "Builder", OS: "linux", Arch: "amd64", PublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.store.Write(context.Background(), func(r app.Repos) error { return r.BuildHosts().Insert(context.Background(), &h) }); err != nil {
		t.Fatal(err)
	}
	return h.ID
}

func TestBuildHostPermissions(t *testing.T) {
	browser, token := signedIn(t)
	id := jsonNumber(float64(seedBuildHost(t, browser)))
	delegate := browser.expect(201, "POST", "/api/v1/tokens", `{"name":"delegate","is_delegate":true}`)["secret"].(string)
	anonymous := browser.asAgent("")
	invalid := browser.asAgent("invalid")
	agent := browser.asAgent(token)
	worker := browser.asAgent(token)
	worker.worker = "one"
	routes := [][2]string{{"GET", "/api/v1/build-hosts"}, {"GET", "/api/v1/build-hosts/" + id}, {"POST", "/api/v1/build-hosts/pairing-codes"}, {"POST", "/api/v1/build-hosts/" + id + "/approve"}, {"PATCH", "/api/v1/build-hosts/" + id}, {"POST", "/api/v1/build-hosts/" + id + "/revoke"}}
	for _, route := range routes {
		anonymous.expect(401, route[0], route[1], `{}`)
		invalid.expect(401, route[0], route[1], `{}`)
		for _, client := range []*apiClient{agent, browser.asAgent(delegate), worker} {
			client.expect(403, route[0], route[1], `{}`)
		}
		if route[0] != "GET" {
			browser.withOrigin("").expect(403, route[0], route[1], `{}`)
			browser.withOrigin("https://foreign.example").expect(403, route[0], route[1], `{}`)
		}
	}
	// Rejected human-only writes must never spend or be masked by the agent limiter.
	for i := 0; i < 150; i++ {
		agent.expect(403, "POST", "/api/v1/build-hosts/pairing-codes", `{}`)
	}
}

func TestBuildHostAPI(t *testing.T) {
	browser, _ := signedIn(t)
	id := jsonNumber(float64(seedBuildHost(t, browser)))
	path := "/api/v1/build-hosts/" + id
	host := browser.expect(200, "GET", path, "")
	for _, field := range []string{"id", "name", "os", "arch", "status", "capabilities", "max_parallel", "enabled", "public_key", "facts", "agent_version", "last_seen_at", "created_at", "approved_at", "approved_by", "revoked_at"} {
		if _, ok := host[field]; !ok {
			t.Fatal("missing", field)
		}
	}
	if host["status"] != "pending" || host["approved_by"] != nil || len(host["capabilities"].([]any)) != 0 {
		t.Fatal(host)
	}
	if _, ok := host["facts"].(map[string]any); !ok {
		t.Fatal(host)
	}
	code := browser.expect(201, "POST", "/api/v1/build-hosts/pairing-codes", "")
	if len(code) != 2 || len(code["code"].(string)) != 26 || code["expires_at"] == nil {
		t.Fatal(code)
	}
	for _, route := range []struct{ method, path string }{{"POST", "/api/v1/build-hosts/pairing-codes"}, {"POST", path + "/approve"}, {"PATCH", path}, {"POST", path + "/revoke"}} {
		for _, body := range []string{`null`, `[]`, `true`, `{} {}`, `{"unknown":1}`} {
			browser.expect(400, route.method, route.path, body)
		}
	}
	for _, body := range []string{`{"name":null}`, `{"capabilities":null}`, `{"max_parallel":null}`, `{"enabled":null}`, `{"max_parallel":0}`, `{"max_parallel":9}`, `{"capabilities":["GPU"]}`, `{"capabilities":[1]}`, `{"capabilities":[null]}`, `{"status":"approved"}`, `{"os":"linux"}`, `{"facts":{}}`, `{"name":""}`} {
		browser.expect(400, "PATCH", path, body)
	}
	browser.expect(400, "POST", path+"/approve", `{"enabled":false}`)
	host = browser.expect(200, "POST", path+"/approve", `{"name":"Approved","capabilities":["msvc","gpu","gpu"],"max_parallel":8}`)
	if host["status"] != "approved" || host["approved_at"] == nil || host["approved_by"] != float64(1) || host["name"] != "Approved" || len(host["capabilities"].([]any)) != 2 {
		t.Fatal(host)
	}
	browser.expect(409, "POST", path+"/approve", `{}`)
	host = browser.expect(200, "PATCH", path, `{"enabled":false,"capabilities":[]}`)
	if host["enabled"] != false || len(host["capabilities"].([]any)) != 0 {
		t.Fatal(host)
	}
	browser.expect(200, "PATCH", path, `{}`)
	host = browser.expect(200, "POST", path+"/revoke", "")
	stamp := host["revoked_at"]
	if host["status"] != "revoked" || host["enabled"] != false || stamp == nil {
		t.Fatal(host)
	}
	if browser.expect(200, "POST", path+"/revoke", `{}`)["revoked_at"] != stamp {
		t.Fatal("revocation changed")
	}
	browser.expect(409, "POST", path+"/approve", `{}`)
	browser.expect(409, "PATCH", path, `{"enabled":true}`)
	browser.expect(200, "PATCH", path, `{"name":"Retired"}`)
	for _, method := range []string{"GET", "PATCH"} {
		browser.expect(400, method, "/api/v1/build-hosts/bad", `{}`)
		browser.expect(404, method, "/api/v1/build-hosts/999", `{}`)
	}
	browser.expect(404, "POST", "/api/v1/build-hosts/999/approve", `{}`)
	browser.expect(404, "POST", "/api/v1/build-hosts/999/revoke", `{}`)
	req, _ := http.NewRequest("GET", browser.base+"/api/v1/build-hosts", nil)
	response, err := browser.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	var hosts []map[string]any
	if err := json.Unmarshal(raw, &hosts); err != nil || len(hosts) != 1 || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal(string(raw), err)
	}
	if strings.Contains(string(raw), "code_hash") || strings.Contains(string(raw), code["code"].(string)) {
		t.Fatal("pairing secret in registry")
	}
}

func TestBuildHostEmptyRegistryAndDefaultApproval(t *testing.T) {
	browser, _ := signedIn(t)
	req, _ := http.NewRequest("GET", browser.base+"/api/v1/build-hosts", nil)
	res, err := browser.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 || strings.TrimSpace(string(raw)) != "[]" {
		t.Fatal("empty listing", string(raw), err)
	}
	path := "/api/v1/build-hosts/" + jsonNumber(float64(seedBuildHost(t, browser)))
	for _, body := range []string{`{"name":"A","name":"B"}`, `{"max_parallel":1.5}`, `{"name":42}`, `{"enabled":"false"}`, `{"capabilities":{}}`, `{"capabilities":["x!"]}`, strings.Repeat(" ", 1<<20) + `{}`} {
		browser.expect(400, "PATCH", path, body)
	}
	host := browser.expect(200, "POST", path+"/approve", "")
	if host["name"] != "Builder" || host["status"] != "approved" || host["max_parallel"] != float64(1) || host["enabled"] != true {
		t.Fatal(host)
	}
	browser.expect(201, "POST", "/api/v1/build-hosts/pairing-codes", `{}`)
}
