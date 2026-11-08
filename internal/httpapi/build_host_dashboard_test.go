package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/app"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestBuildHostDashboard(t *testing.T) {
	c, token := signedIn(t)
	path := "/api/v1/build-hosts/runners"
	c.asAgent("").expect(401, "GET", path, "")
	c.asAgent(token).expect(403, "GET", path, "")
	id := seedBuildHost(t, c)
	snapshot := c.expect(200, "GET", path, "")
	if len(snapshot["hosts"].([]any)) != 0 {
		t.Fatal(snapshot)
	}
	c.expect(200, "POST", fmt.Sprintf("/api/v1/build-hosts/%d/approve", id), `{}`)
	snapshot = c.expect(200, "GET", path, "")
	host := snapshot["hosts"].([]any)[0].(map[string]any)
	if host["online"] != false || host["last_seen_at"] != nil || len(host["jobs"].([]any)) != 0 {
		t.Fatal(host)
	}
	counts := snapshot["queued_by_os"].(map[string]any)
	for _, os := range []string{"linux", "windows", "macos"} {
		if counts[os] != float64(0) {
			t.Fatal(counts)
		}
	}
	if err := c.store.Write(context.Background(), func(r app.Repos) error {
		for i := 0; i < 25; i++ {
			if err := r.BuildHostEvents().Append(context.Background(), &domain.BuildHostEvent{HostID: id, Type: "toolchains_changed", Data: []byte(`{"before":{},"after":{}}`), CreatedAt: time.Now()}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	eventsPath := fmt.Sprintf("/api/v1/build-hosts/%d/events", id)
	response, err := c.http.Get(c.base + eventsPath + "?latest=true&limit=20")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var events []domain.BuildHostEvent
	if err = json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 20 || events[0].ID != 25 || events[19].ID != 6 {
		t.Fatal(events)
	}
	for _, query := range []string{"latest=true&after_id=0", "latest=bad", "latest=true&latest=true", "limit=201"} {
		c.expect(400, "GET", eventsPath+"?"+query, "")
	}
	code := c.expect(201, "POST", "/api/v1/build-hosts/pairing-codes", `{}`)
	if _, ok := code["public_base_url"]; !ok {
		t.Fatal("missing configured URL")
	}
}

func TestBuildHostPairingMetadata(t *testing.T) {
	c := newServerWithPublicBaseURL(t, "https://configured.example").withOrigin("https://configured.example")
	c.expect(200, "POST", "/api/v1/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"a long enough password"}`)
	request, err := http.NewRequest("POST", c.base+"/api/v1/build-hosts/pairing-codes", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://configured.example")
	response, err := c.http.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 201 || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal(response.StatusCode, response.Header)
	}
	var body map[string]any
	if err = json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["public_base_url"] != "https://configured.example" || body["code"] == nil {
		t.Fatal(body)
	}
}
