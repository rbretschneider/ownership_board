package httpapi_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/auth"
)

const apiHostFacts = `{"os":"linux","os_version":"","arch":"amd64","hostname":"builder","lan_ip":"","cpu_model":"","cores":2,"threads":4,"ram_bytes":0,"disks":[],"gpus":[],"toolchains":{},"interactive_session":false,"build_user_is_admin":false,"agent_version":"1"}`

func TestMachineHostFlow(t *testing.T) {
	browser, token := signedIn(t)
	machine := browser.asAgent("")
	code := browser.expect(201, "POST", "/api/v1/build-hosts/pairing-codes", `{}`)["code"].(string)
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	raw, _ := json.Marshal(map[string]any{"code": code, "name": "Builder", "public_key": base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), "facts": json.RawMessage(apiHostFacts)})
	h := machine.expect(201, "POST", "/api/v1/build-host/pair", string(raw))
	id := strconv.FormatInt(int64(h["id"].(float64)), 10)
	machine.expect(401, "POST", "/api/v1/build-host/pair", string(raw))
	send := func(method, path, body string, want int, mutate func(*http.Request)) {
		t.Helper()
		req, _ := http.NewRequest(method, browser.base+path, strings.NewReader(body))
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		req.Header.Set("X-Build-Host", id)
		req.Header.Set("X-Timestamp", ts)
		req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(key, auth.HostSignatureMessage(method, path, ts, []byte(body)))))
		if mutate != nil {
			mutate(req)
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			var v any
			json.NewDecoder(res.Body).Decode(&v)
			t.Fatalf("%s %s = %d want %d: %v", method, path, res.StatusCode, want, v)
		}
	}
	status := "/api/v1/build-host/status"
	heartbeat := "/api/v1/build-host/heartbeat"
	report := `{"facts":` + apiHostFacts + `,"running_job_ids":[],"free_disk_bytes":100}`
	send("GET", status, "", 200, nil)
	send("POST", heartbeat, report, 401, nil)
	browser.withOrigin("").expect(401, "GET", status, "")
	browser.asAgent(token).expect(401, "GET", status, "")
	browser.expect(200, "POST", "/api/v1/build-hosts/"+id+"/approve", `{}`)
	send("POST", heartbeat, report, 200, nil)
	for _, header := range []string{"X-Build-Host", "X-Timestamp", "X-Signature"} {
		send("POST", heartbeat, report, 401, func(r *http.Request) { r.Header.Del(header) })
		send("POST", heartbeat, report, 401, func(r *http.Request) { r.Header.Add(header, r.Header.Get(header)) })
		send("POST", heartbeat, report, 401, func(r *http.Request) { r.Header.Set(header, r.Header.Get(header)+",extra") })
		send("POST", heartbeat, report, 401, func(r *http.Request) { r.Header.Set(header, strings.Repeat("a", 129)) })
	}
	send("POST", heartbeat, report, 401, func(r *http.Request) {
		r.Body = io.NopCloser(strings.NewReader(strings.Replace(report, "100", "101", 1)))
	})
	send("POST", heartbeat, report, 200, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer invalid")
		r.Header.Set("Origin", "https://foreign.example")
	})
	machine.expect(401, "POST", heartbeat, report)
	host := browser.expect(200, "GET", "/api/v1/build-hosts/"+id, "")
	if host["online"] != true || host["free_disk_bytes"] != float64(100) {
		t.Fatal(host)
	}
	send("POST", heartbeat, `{"facts":`+apiHostFacts+`,"running_job_ids":[1,1],"free_disk_bytes":0}`, 400, nil)
	send("POST", heartbeat, `{"facts":`+apiHostFacts+`,"running_job_ids":[],"free_disk_bytes":-1}`, 400, nil)
	send("POST", heartbeat, `{"facts":`+apiHostFacts+`,"running_job_ids":[]}`, 400, nil)
	send("POST", heartbeat, strings.Repeat(" ", 96*1024+1), 400, nil)

	send("POST", heartbeat, report, 401, func(r *http.Request) { r.Header.Add("X-Timestamp", r.Header.Get("X-Timestamp")) })
	send("GET", status, "{}", 400, nil)
	send("GET", status+"?x=1", "", 401, nil)
	send("GET", "/api/v1/build-hosts/"+id, "", 401, nil)
	browser.expect(200, "POST", "/api/v1/build-hosts/"+id+"/revoke", `{}`)
	send("GET", status, "", 401, nil)
}

func TestHostStatusRejectsHEAD(t *testing.T) {
	browser, _ := signedIn(t)
	id := seedBuildHost(t, browser)
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	publicKey := base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	if _, e := browser.store.SQL().Exec(`UPDATE build_hosts SET public_key=? WHERE id=?`, publicKey, id); e != nil {
		t.Fatal(e)
	}
	req, _ := http.NewRequest("HEAD", browser.base+"/api/v1/build-host/status", nil)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("X-Build-Host", strconv.FormatInt(id, 10))
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(key, auth.HostSignatureMessage("HEAD", "/api/v1/build-host/status", ts, nil))))
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal(res.StatusCode)
	}
}

func TestMachinePairValidationAndThrottle(t *testing.T) {
	browser, _ := signedIn(t)
	machine := browser.asAgent("").withOrigin("https://foreign.example")
	code := browser.expect(201, "POST", "/api/v1/build-hosts/pairing-codes", `{}`)["code"].(string)
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	input := map[string]any{"code": code, "name": "Builder", "public_key": base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), "facts": json.RawMessage(apiHostFacts)}
	raw, _ := json.Marshal(input)
	machine.expect(400, "POST", "/api/v1/build-host/pair", strings.Replace(string(raw), `"cores":2`, `"cores":2,"cores":3`, 1))
	machine.expect(400, "POST", "/api/v1/build-host/pair", strings.Repeat(" ", 96*1024+1))
	machine.expect(401, "POST", "/api/v1/build-host/pair", strings.Replace(string(raw), code, "unknown", 1))
	h := machine.expect(201, "POST", "/api/v1/build-host/pair", string(raw))
	if len(h) != 2 || h["status"] != "pending" {
		t.Fatal(h)
	}
	machine.expect(401, "POST", "/api/v1/build-host/pair", string(raw))
	req, _ := http.NewRequest("POST", browser.base+"/api/v1/build-host/pair", strings.NewReader(string(raw)))
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != 429 || res.Header.Get("Retry-After") == "" {
		t.Fatal(res.StatusCode, res.Header)
	}
}

func TestBuildHostHistoryHTTP(t *testing.T) {
	browser, token := signedIn(t)
	id := jsonNumber(float64(seedBuildHost(t, browser)))
	path := "/api/v1/build-hosts/" + id + "/events"
	for _, query := range []string{"?after_id=-1", "?limit=0", "?limit=201", "?limit=abc", "?limit=1&limit=2", "?extra=1"} {
		browser.expect(400, "GET", path+query, "")
	}
	browser.asAgent("").expect(401, "GET", path, "")
	browser.asAgent(token).expect(403, "GET", path, "")
	req, _ := http.NewRequest("GET", browser.base+path, nil)
	res, e := browser.http.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	var events []any
	if e = json.NewDecoder(res.Body).Decode(&events); e != nil || res.StatusCode != 200 || events == nil || len(events) != 0 {
		t.Fatal(events, e, res.StatusCode)
	}
}

func TestMachinePathsAreNotRedirected(t *testing.T) {
	browser, _ := signedIn(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, path := range []string{"/api/v1/build-host//status", "/api/v1/build-host/./status", "/api/v1/x/../build-host/status", "/api/v1/build-host/status/"} {
		res, e := client.Get(browser.base + path)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != 401 {
			t.Fatal(path, res.StatusCode)
		}
	}
}
