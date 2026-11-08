package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildJobIdentity(t *testing.T) {
	a := BuildJobSubmission{TicketID: 1, Repo: "org/repo", SHA: strings.Repeat("A", 40), Preset: "cmake", Target: BuildJobTarget{OS: "linux", RequiredCapabilities: []string{"x", "a", "x"}}, BundleBytes: 1, BundleSHA256: strings.Repeat("a", 64)}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	b := a
	b.Target.RequiredCapabilities = []string{"a", "x"}
	b.TicketID = 2
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	if a.ID() != b.ID() || a.SHA != strings.Repeat("a", 40) {
		t.Fatal("identity is not canonical")
	}
	b.Preset = "script"
	if a.ID() == b.ID() {
		t.Fatal("preset excluded from identity")
	}
	for _, sha := range []string{"", strings.Repeat("a", 39), strings.Repeat("g", 40)} {
		b = a
		b.SHA = sha
		if b.Validate() == nil {
			t.Fatalf("accepted %q", sha)
		}
	}
}
func TestBuildJobResultValidation(t *testing.T) {
	good := BuildJobResult{Commands: []BuildJobCommand{{Label: "build", ExitCode: 0, Log: "ok"}}}
	if err := good.Validate("passed"); err != nil {
		t.Fatal(err)
	}
	good.Commands[0].ExitCode = 1
	if good.Validate("passed") == nil {
		t.Fatal("accepted failing passed result")
	}
	good.Commands[0].Log = strings.Repeat("x", 16385)
	if KindOf(good.Validate("failed")) != KindTooLarge {
		t.Fatal("missing size error")
	}
}
func TestBuildHostJobIDJSON(t *testing.T) {
	for _, raw := range []string{`[1,42]`, `["` + strings.Repeat("a", 64) + `",2]`} {
		var ids []BuildHostJobID
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			t.Fatal(err)
		}
		if err := ValidateBuildHostTelemetry(ids, nil); err != nil {
			t.Fatal(err)
		}
		out, _ := json.Marshal(ids)
		if string(out) != raw {
			t.Fatalf("round trip %s", out)
		}
	}
	for _, raw := range []string{`[0]`, `[-1]`, `[1.5]`, `["12"]`, `[null]`, `["` + strings.Repeat("A", 64) + `"]`, `[1,1]`} {
		var ids []BuildHostJobID
		err := json.Unmarshal([]byte(raw), &ids)
		if err == nil {
			err = ValidateBuildHostTelemetry(ids, nil)
		}
		if err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestBuildJobResultSizeBoundary(t *testing.T) {
	for _, size := range []int{BuildJobResultLimit, BuildJobResultLimit + 1} {
		commands := make([]BuildJobCommand, 16)
		for i := range commands {
			commands[i] = BuildJobCommand{Label: "build", Log: strings.Repeat("x", 16000)}
		}
		r := BuildJobResult{Commands: commands}
		raw, _ := json.Marshal(r)
		// Fill the remaining bytes across several bounded excerpts.
		remaining := size - len(raw)
		for i := range r.Commands {
			add := min(remaining, 16384-len(r.Commands[i].Log))
			if add > 0 {
				r.Commands[i].Log += strings.Repeat("x", add)
				remaining -= add
			}
		}
		if remaining != 0 {
			t.Fatal("invalid test sizing", remaining)
		}
		raw, _ = json.Marshal(r)
		if len(raw) != size {
			t.Fatal(len(raw))
		}
		c := BuildJobCompletion{Attempt: 1, Status: "passed", Result: raw}
		err := c.Validate()
		if size == BuildJobResultLimit && err != nil || size > BuildJobResultLimit && KindOf(err) != KindTooLarge {
			t.Fatal(size, err)
		}
	}
}
func TestBuildJobResultRequiredCommandFields(t *testing.T) {
	for _, raw := range []string{`{"commands":[{"label":"build","log":"ok"}]}`, `{"commands":[{"label":"build","exit_code":0}]}`} {
		c := BuildJobCompletion{Attempt: 1, Status: "passed", Result: []byte(raw)}
		if c.Validate() == nil {
			t.Fatal("accepted missing command field", raw)
		}
	}
}
