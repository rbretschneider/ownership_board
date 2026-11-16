package domain

import (
	"strings"
	"testing"
)

func TestManagerHeadline(t *testing.T) {
	cases := []struct{ name, summary, want string }{
		{"headline line wins", "MANAGER SUMMARY\nHEADLINE: Merged #12 and parked #13.\nMoving: #12", "Merged #12 and parked #13."},
		{"markdown headline", "## Summary\n**HEADLINE:** Answered #64's question.\n- details", "Answered #64's question."},
		{"case-insensitive", "headline: Nothing to do.", "Nothing to do."},
		{"old summary: first sentence", "MANAGER SUMMARY\nMoving:\nRepeated #64 directions. Then more text follows here.", "Repeated #64 directions."},
		{"empty", "", ""},
		{"ticket refs keep their hash", "MANAGER SUMMARY\n- #64 is waiting on #55.", "#64 is waiting on #55."},
		{"legacy line: the ticket and its action", "MANAGER SUMMARY\n- #64 cycling:64:failure:4074394c:2: why: same Windows failure | action: parked it for a fix | expect: x",
			"#64: parked it for a fix"},
	}
	for _, c := range cases {
		if got := ManagerHeadline(c.summary); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	long := ManagerHeadline("HEADLINE: " + strings.Repeat("word ", 40))
	if len([]rune(long)) != MaxManagerHeadlineLength || !strings.HasSuffix(long, "…") {
		t.Errorf("long headline = %q (%d runes)", long, len([]rune(long)))
	}
}

func TestWithManagerHeadline(t *testing.T) {
	if got := WithManagerHeadline("Did things.", "  Merged   #12. "); got != "HEADLINE: Merged #12.\nDid things." {
		t.Errorf("prepend: %q", got)
	}
	kept := "HEADLINE: Own line.\nDid things."
	if got := WithManagerHeadline(kept, "Other"); got != kept {
		t.Errorf("kept: %q", got)
	}
	if got := WithManagerHeadline("Did things.", ""); got != "Did things." {
		t.Errorf("no headline: %q", got)
	}
	if got := WithManagerHeadline("", "Only this"); got != "HEADLINE: Only this" {
		t.Errorf("empty summary: %q", got)
	}
}
