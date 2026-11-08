package httpapi_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestBuildQueueHTTP(t *testing.T) {
	browser, token := signedIn(t)
	agent := browser.asAgent(token)
	repo := browser.expect(201, "POST", "/api/v1/repos", `{"full_name":"example/tools"}`)
	ticket := browser.expect(201, "POST", "/api/v1/tickets", fmt.Sprintf(`{"title":"Example","kind":"story","repo_id":%v}`, repo["id"]))
	tid := ticket["id"]
	metadata := fmt.Sprintf(`{"ticket_id":%v,"repo":"example/tools","sha":"%s","preset":"script","target":{"os":"linux","required_capabilities":[]},"bundle_bytes":3,"bundle_sha256":"%x"}`, tid, strings.Repeat("a", 40), sha256.Sum256([]byte("abc")))
	upload := func(raw, bundle string, extra bool, want int, truncated ...bool) map[string]any {
		t.Helper()
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		p, _ := mw.CreateFormField("metadata")
		io.WriteString(p, raw)
		p, _ = mw.CreateFormFile("bundle", "commit.bundle")
		io.WriteString(p, bundle)
		if extra {
			p, _ = mw.CreateFormField("extra")
			io.WriteString(p, "x")
		}
		if len(truncated) == 0 || !truncated[0] {
			mw.Close()
		}
		req, _ := http.NewRequest("POST", browser.base+"/api/v1/build-jobs", &body)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var out map[string]any
		json.NewDecoder(res.Body).Decode(&out)
		if res.StatusCode != want {
			t.Fatalf("upload %d: %v want %d", res.StatusCode, out, want)
		}
		return out
	}
	upload(metadata, "abc", false, 400, true)
	upload(metadata, "abd", false, 400)
	upload(metadata, "abc", true, 400)
	job := upload(metadata, "abc", false, 201)
	id := job["id"].(string)
	path := "/api/v1/build-jobs/" + id
	upload(metadata, "different", false, 200)
	agent.expect(200, "GET", path, "")
	browser.expect(403, "GET", path, "")
	browser.asAgent("").expect(401, "GET", path, "")
	browser.expect(403, "POST", "/api/v1/build-jobs", `{}`)
	hostID := seedBuildHost(t, browser)
	hid := strconv.FormatInt(hostID, 10)
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	public := base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	if _, e := browser.store.SQL().Exec(`UPDATE build_hosts SET public_key=? WHERE id=?`, public, hostID); e != nil {
		t.Fatal(e)
	}
	send := func(method, path, body string, want int, mutate func(*http.Request)) map[string]any {
		t.Helper()
		req, _ := http.NewRequest(method, browser.base+path, strings.NewReader(body))
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		req.Header.Set("X-Build-Host", hid)
		req.Header.Set("X-Timestamp", ts)
		req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(key, auth.HostSignatureMessage(method, path, ts, []byte(body)))))
		if mutate != nil {
			mutate(req)
		}
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		var out map[string]any
		json.Unmarshal(raw, &out)
		if res.StatusCode != want {
			t.Fatalf("%s %s = %d want %d %s", method, path, res.StatusCode, want, raw)
		}
		if strings.HasSuffix(path, "/bundle") && want == 200 && (string(raw) != "abc" || res.Header.Get("Content-Length") != "3") {
			t.Fatal("bundle response", string(raw), res.Header)
		}
		return out
	}
	claim := "/api/v1/build-host/claim"
	base := "/api/v1/build-host/jobs/" + id
	send("POST", claim, `{}`, 401, nil)
	browser.expect(200, "POST", "/api/v1/build-hosts/"+hid+"/approve", `{}`)
	for _, route := range []struct{ method, path string }{{"POST", claim}, {"GET", base + "/bundle"}, {"POST", base + "/progress"}, {"POST", base + "/result"}} {
		agent.expect(401, route.method, route.path, `{}`)
		browser.expect(401, route.method, route.path, `{}`)
	}
	send("POST", "/api/v1/build-jobs", `{}`, 401, nil)
	send("GET", path, "", 401, nil)
	send("POST", path+"/cancel", `{}`, 401, nil)
	send("POST", claim+"?x=1", `{}`, 401, nil)
	for _, bad := range []string{"bad", strings.ToUpper(id)} {
		send("POST", "/api/v1/build-host/jobs/"+bad+"/progress", `{}`, 401, nil)
	}
	send("POST", claim, `{}`, 200, nil)
	send("POST", claim, `{}`, 204, nil)
	send("GET", base+"/bundle", "", 200, nil)
	send("GET", base+"/bundle?x=1", "", 401, nil)
	send("GET", strings.Replace(base, "/jobs/", "//jobs/", 1)+"/bundle", "", 401, nil)
	send("POST", base+"/progress", `{"attempt":1,"phase":"build","detail":"working"}`, 200, nil)
	send("POST", base+"/progress", `{"attempt":1,"phase":"build","detail":"working"}`, 401, func(r *http.Request) { r.Header.Add("X-Signature", r.Header.Get("X-Signature")) })
	send("POST", base+"/result", strings.Repeat(" ", 260*1024+1), 413, nil)
	send("POST", base+"/result", `{"attempt":1,"status":"passed","result":{"commands":[{"label":"build","exit_code":1,"log":""}]}}`, 400, nil)
	browser.withOrigin("").expect(403, "POST", path+"/cancel", `{}`)
	agent.expect(403, "POST", path+"/cancel", `{}`)
	if _, e := browser.store.SQL().Exec(`UPDATE users SET is_admin=0 WHERE id=1`); e != nil {
		t.Fatal(e)
	}
	browser.expect(403, "POST", path+"/cancel", `{}`)
	if _, e := browser.store.SQL().Exec(`UPDATE users SET is_admin=1 WHERE id=1`); e != nil {
		t.Fatal(e)
	}
	browser.expect(200, "POST", path+"/cancel", `{}`)
	reply := send("POST", base+"/progress", `{"attempt":1,"phase":"stopped","detail":""}`, 200, nil)
	if reply["stop"] != true {
		t.Fatal(reply)
	}
	send("POST", claim, `{}`, 204, nil)
	// Exercise the result-specific allowance with a valid object at the exact cap.
	second := upload(strings.Replace(metadata, `"preset":"script"`, `"preset":"cmake"`, 1), "abc", false, 201)
	secondID := second["id"].(string)
	resultPath := "/api/v1/build-host/jobs/" + secondID + "/result"
	send("POST", claim, `{}`, 200, nil)
	result := domain.BuildJobResult{Commands: make([]domain.BuildJobCommand, 16)}
	for i := range result.Commands {
		result.Commands[i] = domain.BuildJobCommand{Label: "build", Log: strings.Repeat("x", 16000)}
	}
	rawResult, _ := json.Marshal(result)
	remaining := domain.BuildJobResultLimit - len(rawResult)
	for i := range result.Commands {
		add := min(remaining, 16384-len(result.Commands[i].Log))
		result.Commands[i].Log += strings.Repeat("x", add)
		remaining -= add
	}
	rawResult, _ = json.Marshal(result)
	if len(rawResult) != domain.BuildJobResultLimit {
		t.Fatal("invalid boundary fixture")
	}
	result.Commands[15].Log += "x"
	over, _ := json.Marshal(result)
	send("POST", resultPath, `{"attempt":1,"status":"passed","result":`+string(over)+`}`, 413, nil)
	final := send("POST", resultPath, `{"attempt":1,"status":"passed","result":`+string(rawResult)+`}`, 200, nil)
	if final["status"] != "passed" {
		t.Fatal("large result did not complete")
	}
	browser.expect(409, "POST", "/api/v1/build-jobs/"+secondID+"/cancel", `{}`)
	send("POST", resultPath, `{"attempt":1,"status":"passed","result":`+string(rawResult)+`}`, 200, nil)

}
