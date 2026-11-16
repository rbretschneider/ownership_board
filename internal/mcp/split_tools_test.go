package mcp

import (
	"reflect"
	"testing"
)

func TestSplitTicketContract(t *testing.T) {
	var tool Tool
	for _, candidate := range BoardTools(Services{}) {
		if candidate.Definition().Name == "split_ticket" {
			tool = candidate
		}
	}
	if tool == nil {
		t.Fatal("split_ticket is not registered")
	}
	schema := tool.Definition().InputSchema
	if !reflect.DeepEqual(schema["required"], []string{"id", "reason", "children"}) {
		t.Fatalf("required: %v", schema["required"])
	}
	children := schema["properties"].(map[string]any)["children"].(map[string]any)
	child := children["items"].(map[string]any)
	if !reflect.DeepEqual(child["required"], []string{"title", "spec"}) {
		t.Fatalf("child required: %v", child["required"])
	}
	for _, name := range []string{"title", "kind", "body", "spec", "after"} {
		if _, ok := child["properties"].(map[string]any)[name]; !ok {
			t.Errorf("child is missing %q", name)
		}
	}
}
