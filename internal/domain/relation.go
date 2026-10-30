package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// RelationKind is how one ticket relates to another.
type RelationKind string

// The relation kinds, read as "<from> <kind> <to>".
const (
	RelationCausedBy    RelationKind = "caused_by"
	RelationFollowUpOf  RelationKind = "follow_up_of"
	RelationDuplicateOf RelationKind = "duplicate_of"
	RelationRelatesTo   RelationKind = "relates_to"
	// RelationDependsOn means the "from" ticket waits until the "to" ticket
	// is merged: agents cannot claim it before then.
	RelationDependsOn RelationKind = "depends_on"
)

// MaxOutgoingRelations caps the links a ticket can make to others.
const MaxOutgoingRelations = 50

// MaxDependencies caps the tickets one ticket can depend on.
const MaxDependencies = 20

// relationLabels are the phrases for each kind, seen from each end.
var relationLabels = map[RelationKind][2]string{
	RelationCausedBy:    {"caused by", "caused"},
	RelationFollowUpOf:  {"follow-up of", "has follow-up"},
	RelationDuplicateOf: {"duplicate of", "duplicated by"},
	RelationRelatesTo:   {"relates to", "relates to"},
	RelationDependsOn:   {"depends on", "needed by"},
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
		return "", Invalidf("relation must be caused_by, follow_up_of, duplicate_of, relates_to or depends_on")
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

// ParseDependencies validates the ids a new ticket depends on and drops
// repeats, keeping the first-seen order.
func ParseDependencies(ids []int64) ([]int64, error) {
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, Invalidf("depends_on: ticket ids must be positive")
		}
		if !slices.Contains(unique, id) {
			unique = append(unique, id)
		}
	}
	if len(unique) > MaxDependencies {
		return nil, Invalidf("depends_on: a ticket can depend on at most %d tickets", MaxDependencies)
	}
	return unique, nil
}

// DependencyCycle reports the cycle that adding "from depends_on to" would
// close, given every existing depends_on edge (dependant id to the ids it
// depends on). It returns nil when there is none, or the path from, to, ...,
// from. The search is breadth-first with neighbours in id order, so the
// shortest cycle is found and the result is the same on every run.
func DependencyCycle(edges map[int64][]int64, from, to int64) []int64 {
	if from == to {
		return []int64{from, to}
	}
	previous := map[int64]int64{to: 0}
	queue := []int64{to}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		next := slices.Clone(edges[current])
		slices.Sort(next)
		for _, neighbour := range next {
			if _, isSeen := previous[neighbour]; isSeen {
				continue
			}
			previous[neighbour] = current
			if neighbour == from {
				return cyclePath(previous, from, to)
			}
			queue = append(queue, neighbour)
		}
	}
	return nil
}

// cyclePath walks the search back from "from" to "to" and closes the loop.
func cyclePath(previous map[int64]int64, from, to int64) []int64 {
	reversed := []int64{from}
	for step := previous[from]; step != to; step = previous[step] {
		reversed = append(reversed, step)
	}
	reversed = append(reversed, to)
	path := []int64{from}
	for i := len(reversed) - 1; i >= 0; i-- {
		path = append(path, reversed[i])
	}
	return path
}

// FormatCyclePath writes a dependency path as "#1 -> #2 -> #1".
func FormatCyclePath(path []int64) string {
	parts := make([]string, len(path))
	for i, id := range path {
		parts[i] = fmt.Sprintf("#%d", id)
	}
	return strings.Join(parts, " -> ")
}
