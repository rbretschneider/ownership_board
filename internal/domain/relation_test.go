package domain_test

import (
	"slices"
	"testing"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

func TestDependsOnIsARelationKindWithLabelsFromBothEnds(t *testing.T) {
	kind, err := domain.ParseRelationKind("depends_on")
	if err != nil || kind != domain.RelationDependsOn {
		t.Fatalf("ParseRelationKind(depends_on) = %q, %v", kind, err)
	}
	if got := kind.Label(true); got != "depends on" {
		t.Errorf("outgoing label %q, want %q", got, "depends on")
	}
	if got := kind.Label(false); got != "needed by" {
		t.Errorf("incoming label %q, want %q", got, "needed by")
	}
	if _, err := domain.ParseRelationKind("blocks"); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("unknown kind: %v", err)
	}
}

func TestParseDependenciesDropsRepeatsAndRefusesBadIDs(t *testing.T) {
	ids, err := domain.ParseDependencies([]int64{4, 2, 4})
	if err != nil || !slices.Equal(ids, []int64{4, 2}) {
		t.Fatalf("ParseDependencies = %v, %v, want [4 2]", ids, err)
	}
	for name, bad := range map[string][]int64{"zero": {0}, "negative": {3, -1}} {
		if _, err := domain.ParseDependencies(bad); domain.KindOf(err) != domain.KindInvalid {
			t.Errorf("%s: %v, want invalid", name, err)
		}
	}
	tooMany := make([]int64, domain.MaxDependencies+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	if _, err := domain.ParseDependencies(tooMany); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("%d ids: %v, want invalid", len(tooMany), err)
	}
	if _, err := domain.ParseDependencies(tooMany[:domain.MaxDependencies]); err != nil {
		t.Fatalf("%d ids: %v", domain.MaxDependencies, err)
	}
}

func TestDependencyCycle(t *testing.T) {
	cases := map[string]struct {
		edges    map[int64][]int64
		from, to int64
		want     []int64
	}{
		"two tickets":       {edges: map[int64][]int64{2: {1}}, from: 1, to: 2, want: []int64{1, 2, 1}},
		"three tickets":     {edges: map[int64][]int64{2: {3}, 3: {1}}, from: 1, to: 2, want: []int64{1, 2, 3, 1}},
		"self":              {edges: nil, from: 5, to: 5, want: []int64{5, 5}},
		"chain, no cycle":   {edges: map[int64][]int64{2: {3}, 3: {4}}, from: 1, to: 2},
		"diamond, no cycle": {edges: map[int64][]int64{1: {2, 3}, 2: {4}, 3: {4}}, from: 5, to: 1},
		"diamond closed by the new edge": {
			edges: map[int64][]int64{1: {2, 3}, 2: {4}, 3: {4}}, from: 4, to: 1, want: []int64{4, 1, 2, 4},
		},
		"disconnected graph": {edges: map[int64][]int64{7: {8}, 8: {9}}, from: 1, to: 2},
		"shortest of two cycles": {
			edges: map[int64][]int64{2: {3, 1}, 3: {1}}, from: 1, to: 2, want: []int64{1, 2, 1},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := domain.DependencyCycle(tc.edges, tc.from, tc.to)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("DependencyCycle(%v, %d, %d) = %v, want %v", tc.edges, tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestFormatCyclePath(t *testing.T) {
	if got := domain.FormatCyclePath([]int64{1, 2, 3, 1}); got != "#1 -> #2 -> #3 -> #1" {
		t.Fatalf("FormatCyclePath = %q", got)
	}
}
