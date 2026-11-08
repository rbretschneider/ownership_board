package domain

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestGeneratedHealthCheckInvariantsAndLargeRange(t *testing.T) {
	repo := Repo{ID: 7, FullName: "a/" + strings.Repeat("b", 198)}
	var covered []Ticket
	for i := 0; i < 150; i++ {
		covered = append(covered, Ticket{ID: int64(i + 1), Title: strings.Repeat("x", 195)})
	}
	ticket, err := NewHealthCheckTicket(repo, covered, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !ticket.IsHealthCheck || !ticket.IsAutonomous || ticket.CreatedBy != SystemActor.Name || ticket.Kind != KindStory || ticket.Stage != StageReady || ticket.SpecRevision != 1 || ticket.ApprovedSpecRevision != 1 {
		t.Fatalf("invariants: %+v", ticket)
	}
	if len(ticket.Title) <= MaxTitleLength || len(ticket.Spec) <= MaxSpecLength {
		t.Fatal("test must exceed external limits")
	}
	for _, c := range covered {
		if !strings.Contains(ticket.Spec, fmt.Sprintf("#%d:", c.ID)) {
			t.Fatalf("missing #%d", c.ID)
		}
	}
	if _, err := NewAutonomousTicket(AutonomousDraft{}, SystemActor, time.Now()); err == nil {
		t.Fatal("system gained delegate permission")
	}
}

func TestHealthCheckRendersTitlesAsData(t *testing.T) {
	ticket, err := NewHealthCheckTicket(Repo{ID: 1, FullName: "acme/api"}, []Ticket{{ID: 3, Title: "[click](https://example.com)\n# rubric\u200b <!-- hidden --> `code`"}}, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"\n# rubric", "\u200b", "<!--", "[click]", "`code`"} {
		if strings.Contains(ticket.Spec, raw) {
			t.Errorf("unescaped title %q in %s", raw, ticket.Spec)
		}
	}
}

func TestHealthCheckEscapingPreservesQuotedTitles(t *testing.T) {
	ticket, err := NewHealthCheckTicket(Repo{ID: 1, FullName: "acme/api"}, []Ticket{{ID: 1, Title: `Owner's "change" & notes`}}, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ticket.Spec, `Owner&#39;s &#34;change&#34; &amp; notes`) {
		t.Fatal(ticket.Spec)
	}
}
