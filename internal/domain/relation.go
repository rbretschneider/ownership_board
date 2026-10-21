package domain

import "time"

// RelationKind is how one ticket relates to another.
type RelationKind string

// The relation kinds, read as "<from> <kind> <to>".
const (
	RelationCausedBy    RelationKind = "caused_by"
	RelationFollowUpOf  RelationKind = "follow_up_of"
	RelationDuplicateOf RelationKind = "duplicate_of"
	RelationRelatesTo   RelationKind = "relates_to"
)

// MaxOutgoingRelations caps the links a ticket can make to others.
const MaxOutgoingRelations = 50

// relationLabels are the phrases for each kind, seen from each end.
var relationLabels = map[RelationKind][2]string{
	RelationCausedBy:    {"caused by", "caused"},
	RelationFollowUpOf:  {"follow-up of", "has follow-up"},
	RelationDuplicateOf: {"duplicate of", "duplicated by"},
	RelationRelatesTo:   {"relates to", "relates to"},
}

// Relation is a directed link from one ticket to another.
type Relation struct {
	FromID  int64
	ToID    int64
	Kind    RelationKind
	AddedBy Actor
	AddedAt time.Time
}

// ParseRelationKind validates a relation kind.
func ParseRelationKind(raw string) (RelationKind, error) {
	kind := RelationKind(raw)
	if _, isKnown := relationLabels[kind]; !isKnown {
		return "", Invalidf("relation must be caused_by, follow_up_of, duplicate_of or relates_to")
	}
	return kind, nil
}

// NewRelation validates a link between two tickets.
func NewRelation(fromID, toID int64, kind RelationKind, actor Actor, now time.Time) (Relation, error) {
	if fromID == toID {
		return Relation{}, Invalidf("a ticket cannot be related to itself")
	}
	return Relation{FromID: fromID, ToID: toID, Kind: kind, AddedBy: actor, AddedAt: now}, nil
}

// Label is the phrase seen from the "from" ticket when isOutgoing, or from
// the "to" ticket otherwise.
func (k RelationKind) Label(isOutgoing bool) string {
	labels := relationLabels[k]
	if isOutgoing {
		return labels[0]
	}
	return labels[1]
}
