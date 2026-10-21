package domain

// Kind classifies a ticket.
type Kind string

// The ticket kinds.
const (
	KindStory Kind = "story"
	KindBug   Kind = "bug"
)

// ParseKind validates a ticket kind.
func ParseKind(raw string) (Kind, error) {
	switch Kind(raw) {
	case KindStory, KindBug:
		return Kind(raw), nil
	default:
		return "", Invalidf("unknown kind %q, expected story or bug", raw)
	}
}
