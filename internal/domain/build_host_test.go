package domain

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testBuildHost(t *testing.T) BuildHost {
	t.Helper()
	h, err := NewBuildHost(BuildHostDraft{Name: "Builder", OS: "linux", Arch: "amd64", PublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestBuildHostValidation(t *testing.T) {
	for _, os := range []string{"linux", "windows", "macos"} {
		h := testBuildHost(t)
		h.OS = BuildHostOS(os)
		if err := h.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name string
		edit func(*BuildHost)
	}{
		{"os", func(h *BuildHost) { h.OS = "other" }},
		{"zero", func(h *BuildHost) { h.MaxParallel = 0 }}, {"nine", func(h *BuildHost) { h.MaxParallel = 9 }},
		{"count", func(h *BuildHost) { h.Capabilities = make([]string, 33) }},
		{"uppercase", func(h *BuildHost) { h.Capabilities = []string{"GPU"} }},
		{"empty", func(h *BuildHost) { h.Capabilities = []string{""} }},
		{"space", func(h *BuildHost) { h.Capabilities = []string{"a b"} }},
		{"long", func(h *BuildHost) { h.Capabilities = []string{strings.Repeat("a", 33)} }},
		{"name", func(h *BuildHost) { h.Name = strings.Repeat("é", 201) }},
		{"blank", func(h *BuildHost) { h.Name = " " }},
		{"arch", func(h *BuildHost) { h.Arch = "AMD64" }},
		{"key", func(h *BuildHost) { h.PublicKey = "abc" }},
		{"facts", func(h *BuildHost) { h.Facts = json.RawMessage(`[]`) }},
		{"large facts", func(h *BuildHost) { h.Facts = json.RawMessage(`{"x":"` + strings.Repeat("a", 65536) + `"}`) }},
		{"version", func(h *BuildHost) { h.AgentVersion = strings.Repeat("a", 65) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := testBuildHost(t)
			c.edit(&h)
			if h.Validate() == nil {
				t.Fatal("accepted invalid host")
			}
		})
	}
	h := testBuildHost(t)
	h.MaxParallel = 8
	h.Capabilities = []string{"gpu", strings.Repeat("a", 32), "gpu"}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(h.Capabilities) != 2 || h.Capabilities[1] != "gpu" {
		t.Fatal(h.Capabilities)
	}
}

func TestBuildHostLifecycle(t *testing.T) {
	h := testBuildHost(t)
	now := time.Now().UTC()
	name := "Changed"
	n := 8
	if err := h.Approve(7, now, BuildHostEdit{Name: &name, MaxParallel: &n}); err != nil {
		t.Fatal(err)
	}
	if h.Status != "approved" || *h.ApprovedBy != 7 || !h.ApprovedAt.Equal(now) || h.MaxParallel != 8 {
		t.Fatal(h)
	}
	if KindOf(h.Approve(8, now, BuildHostEdit{})) != KindConflict {
		t.Fatal("approved twice")
	}
	h.Revoke(now)
	h.Revoke(now.Add(time.Hour))
	if !h.RevokedAt.Equal(now) || h.Enabled || *h.ApprovedBy != 7 {
		t.Fatal(h)
	}
	if KindOf(h.Approve(8, now, BuildHostEdit{})) != KindConflict {
		t.Fatal("revived")
	}
	enabled := true
	if KindOf(h.ApplyEdit(BuildHostEdit{Enabled: &enabled})) != KindConflict {
		t.Fatal("enabled revoked")
	}
	pending := testBuildHost(t)
	pending.Revoke(now)
	if pending.ApprovedAt != nil {
		t.Fatal("invented approval")
	}
}

func TestBuildHostBoundariesAndAtomicEdits(t *testing.T) {
	for _, n := range []int{1, 8} {
		h := testBuildHost(t)
		h.MaxParallel = n
		if err := h.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	h := testBuildHost(t)
	h.Name = strings.Repeat("é", 200)
	h.Arch = "0" + strings.Repeat("a", 63)
	h.AgentVersion = strings.Repeat("é", 64)
	h.Capabilities = make([]string, 32)
	for i := range h.Capabilities {
		h.Capabilities[i] = "a"
	}
	h.Capabilities[0] = strings.Repeat("a", 32)
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(h.Capabilities) != 2 {
		t.Fatal("not canonicalized")
	}
	h.Arch = strings.Repeat("a", 65)
	if h.Validate() == nil {
		t.Fatal("long arch accepted")
	}
	for _, key := range []string{base64.StdEncoding.EncodeToString(make([]byte, 31)), base64.StdEncoding.EncodeToString(make([]byte, 33)), "!!!!"} {
		h := testBuildHost(t)
		h.PublicKey = key
		if h.Validate() == nil {
			t.Fatal("bad key accepted")
		}
	}
	for _, facts := range []string{`null`, `1`, `"text"`, `{broken`} {
		h := testBuildHost(t)
		h.Facts = json.RawMessage(facts)
		if h.Validate() == nil {
			t.Fatal("bad facts accepted")
		}
	}
	h = testBuildHost(t)
	name := "  Renamed  "
	zero := 0
	before := h.Name
	if h.ApplyEdit(BuildHostEdit{Name: &name, MaxParallel: &zero}) == nil || h.Name != before {
		t.Fatal("failed edit partially applied")
	}
	disabled := false
	if err := h.ApplyEdit(BuildHostEdit{Name: &name, Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if h.Name != "Renamed" || h.Enabled {
		t.Fatal(h)
	}
	if err := h.Approve(1, time.Now(), BuildHostEdit{}); err != nil || h.Enabled {
		t.Fatal("approval changed enabled", err)
	}
	caps := []string{}
	if err := h.ApplyEdit(BuildHostEdit{Capabilities: &caps}); err != nil || h.Capabilities == nil || len(h.Capabilities) != 0 {
		t.Fatal("cannot clear capabilities")
	}
	zeroDraft := 0
	_, err := NewBuildHost(BuildHostDraft{Name: "Builder", OS: "linux", Arch: "amd64", PublicKey: h.PublicKey, MaxParallel: &zeroDraft}, time.Now())
	if KindOf(err) != KindInvalid {
		t.Fatal("explicit zero defaulted")
	}
}
