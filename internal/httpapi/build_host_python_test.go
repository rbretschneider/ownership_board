package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestBuildHostPythonInteropResultBudget(t *testing.T) {
	python := os.Getenv("BUILD_HOST_PYTHON")
	if python == "" {
		t.Skip("set BUILD_HOST_PYTHON to run the native client integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	source, err := filepath.Abs("../../agent/build_host")
	if err != nil {
		t.Fatal(err)
	}
	// Feed actual Python completion bytes through the server's normalization and
	// validation, including escaping that expands small UTF-8 logs substantially.
	program := `import sys
sys.path.insert(0, sys.argv[1])
import build_host as host
for text in ('<', '>', '&', '\u2028', '\u2029', '🌍\\"\x00'):
    commands = [{'label': str(i) + '<>&\u2028\u2029', 'exit_code': 0, 'log': text * 14000} for i in range(128)]
    print(host.completion_bytes(1, commands).decode())
`
	raw, err := exec.CommandContext(ctx, python, "-c", program, source).Output()
	if err != nil {
		t.Fatalf("Python completion generation: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	if len(lines) != 6 {
		t.Fatalf("got %d completion cases, want 6", len(lines))
	}
	for i, line := range lines {
		var completion domain.BuildJobCompletion
		if err := json.Unmarshal(line, &completion); err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if err := completion.Validate(); err != nil {
			t.Fatalf("case %d: server rejected completion: %v", i, err)
		}
		if len(completion.Result) >= domain.BuildJobResultLimit {
			t.Fatalf("case %d: normalized result exceeds budget: %d", i, len(completion.Result))
		}
		var result domain.BuildJobResult
		if err := json.Unmarshal(completion.Result, &result); err != nil {
			t.Fatal(err)
		}
		if completion.Status != "passed" || len(result.Commands) != 128 {
			t.Fatalf("case %d: lost command outcomes", i)
		}
	}
}

// BUILD_HOST_PYTHON is mandatory in the agent CI job. Other Go jobs stay independent
// of Python installation. The subprocess sees only its own enrollment identity.
func TestBuildHostPythonInterop(t *testing.T) {
	python := os.Getenv("BUILD_HOST_PYTHON")
	if python == "" {
		t.Skip("set BUILD_HOST_PYTHON to run the native client integration")
	}
	if runtime.GOOS != "linux" {
		t.Skip("Linux integration fixture; native Windows tests run separately")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	browser, token := signedIn(t)
	root := t.TempDir()
	source, err := filepath.Abs("../../agent/build_host/build_host.py")
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "state", "config.json")
	code := browser.expect(201, "POST", "/api/v1/build-hosts/pairing-codes", `{}`)["code"].(string)
	var pairLog bytes.Buffer
	pair := exec.CommandContext(ctx, python, source, "pair", "--board", browser.base, "--code", code, "--config", config, "--build-root", filepath.Join(root, "builds"), "--cache-root", filepath.Join(root, "cache"), "--reserve-gib", "0")
	pair.Stdout = &pairLog
	pair.Stderr = &pairLog
	if err := pair.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if pair.Process != nil {
			_ = pair.Process.Kill()
		}
	})
	pairDone := make(chan error, 1)
	go func() { pairDone <- pair.Wait() }()
	var id int64
	deadline := time.Now().Add(50 * time.Second)
	for time.Now().Before(deadline) {
		raw, _ := os.ReadFile(config)
		var saved struct {
			HostID int64 `json:"host_id"`
		}
		_ = json.Unmarshal(raw, &saved)
		if saved.HostID > 0 {
			id = saved.HostID
			break
		}
		select {
		case err := <-pairDone:
			t.Fatalf("enrollment exited before approval: %v: %s", err, pairLog.String())
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	if id == 0 {
		_ = pair.Process.Kill()
		<-pairDone
		t.Fatalf("no pending enrollment: %s", pairLog.String())
	}
	hostPath := "/api/v1/build-hosts/" + strconv.FormatInt(id, 10)
	pending := browser.expect(200, "GET", hostPath, "")
	if pending["status"] != "pending" {
		t.Fatal(pending)
	}
	facts := pending["facts"].(map[string]any)
	if facts["os"] != "linux" || facts["build_user_is_admin"] != false {
		t.Fatal(facts)
	}
	// Pair must remain waiting before human approval.
	select {
	case err := <-pairDone:
		t.Fatalf("enrollment returned while pending: %v: %s", err, pairLog.String())
	case <-time.After(200 * time.Millisecond):
	}
	browser.expect(200, "POST", hostPath+"/approve", `{}`)
	if err := <-pairDone; err != nil {
		t.Fatalf("pair: %v: %s", err, pairLog.String())
	}
	repo := browser.expect(201, "POST", "/api/v1/repos", `{"full_name":"example/fixture"}`)
	ticket := browser.expect(201, "POST", "/api/v1/tickets", fmt.Sprintf(`{"title":"Fixture","kind":"story","repo_id":%v}`, repo["id"]))

	submit := func(script string) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".ownership"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".ownership", "verify.sh"), []byte(script), 0600); err != nil {
			t.Fatal(err)
		}
		git := func(args ...string) string {
			t.Helper()
			cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
			raw, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("git: %v: %s", err, raw)
			}
			return string(bytes.TrimSpace(raw))
		}
		git("init")
		git("add", ".")
		git("commit", "-m", "Fixture")
		sha := git("rev-parse", "HEAD")
		bundle := filepath.Join(dir, "commit.bundle")
		git("bundle", "create", bundle, "HEAD")
		raw, err := os.ReadFile(bundle)
		if err != nil {
			t.Fatal(err)
		}
		meta := fmt.Sprintf(`{"ticket_id":%v,"repo":"example/fixture","sha":"%s","preset":"script","target":{"os":"linux","required_capabilities":[]},"bundle_bytes":%d,"bundle_sha256":"%x"}`, ticket["id"], sha, len(raw), sha256.Sum256(raw))
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, _ := mw.CreateFormField("metadata")
		_, _ = io.WriteString(part, meta)
		part, _ = mw.CreateFormFile("bundle", "commit.bundle")
		_, _ = part.Write(raw)
		_ = mw.Close()
		req, _ := http.NewRequestWithContext(ctx, "POST", browser.base+"/api/v1/build-jobs", &body)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var job map[string]any
		_ = json.NewDecoder(res.Body).Decode(&job)
		if res.StatusCode != 201 {
			t.Fatalf("submit %d: %v", res.StatusCode, job)
		}
		return job["id"].(string)
	}
	id1 := submit("printf 'verified exact commit\\n'\n")
	var runLog bytes.Buffer
	run := exec.CommandContext(ctx, python, source, "run", "--config", config)
	run.Stdout = &runLog
	run.Stderr = &runLog
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.Process.Kill(); _ = run.Wait() })
	agent := browser.asAgent(token)
	waitStatus := func(id, want string) map[string]any {
		t.Helper()
		for until := time.Now().Add(30 * time.Second); time.Now().Before(until); {
			job := agent.expect(200, "GET", "/api/v1/build-jobs/"+id, "")
			if job["status"] == want {
				return job
			}
			if job["status"] == "failed" {
				t.Fatalf("unexpected failure: %v", job)
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("job did not become %s", want)
		return nil
	}
	passed := waitStatus(id1, "passed")
	result := passed["result"].(map[string]any)
	if len(result["commands"].([]any)) < 7 {
		t.Fatal(result)
	}
	id2 := submit("(trap '' TERM; sleep 90) &\nwait\n")
	waitStatus(id2, "running")
	// Wait until the script has actually started, rather than cancelling checkout.
	scriptStarted := false
	for until := time.Now().Add(15 * time.Second); time.Now().Before(until); {
		job := agent.expect(200, "GET", "/api/v1/build-jobs/"+id2, "")
		if job["phase"] == "building" {
			scriptStarted = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !scriptStarted {
		t.Fatal("verification script did not start")
	}
	browser.expect(200, "POST", "/api/v1/build-jobs/"+id2+"/cancel", `{}`)
	for until := time.Now().Add(10 * time.Second); time.Now().Before(until); {
		var released *string
		err := browser.store.SQL().QueryRow(`SELECT assignment_released_at FROM build_jobs WHERE id=?`, id2).Scan(&released)
		if err != nil {
			t.Fatal(err)
		}
		if released != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("cancelled assignment was not released within ten seconds")
}
