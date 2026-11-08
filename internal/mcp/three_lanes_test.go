package mcp

import (
	"reflect"
	"strings"
	"testing"
)

func TestLaneSchemasAndInstructions(t *testing.T) {
	for _, tc := range []struct {
		tool Tool
		want []string
	}{
		{moveTicket{}, []string{"in_progress"}},
		{listTickets{}, []string{"todo", "in_progress", "merged", "deployed"}},
	} {
		def := tc.tool.Definition()
		got := def.InputSchema["properties"].(map[string]any)["lane"].(map[string]any)["enum"]
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s lanes: %v", def.Name, got)
		}
	}
	if !strings.Contains(serverInstructions, "todo -> in_progress (design, build, PR open) -> merged -> deployed") || strings.Contains(serverInstructions, "-> review") {
		t.Fatal(serverInstructions)
	}
}
