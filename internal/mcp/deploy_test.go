package mcp

import (
	"reflect"
	"testing"
)

func TestReportDeployContract(t *testing.T) {
	var tool Tool
	for _, candidate := range BoardTools(Services{}) {
		if candidate.Definition().Name == "report_deploy" {
			tool = candidate
		}
	}
	if tool == nil {
		t.Fatal("report_deploy is not registered")
	}
	schema := tool.Definition().InputSchema
	if !reflect.DeepEqual(schema["required"], []string{"id", "outcome", "summary"}) {
		t.Fatalf("required: %v", schema)
	}
	properties := schema["properties"].(map[string]any)
	if len(properties) != 3 || !reflect.DeepEqual(properties["outcome"].(map[string]any)["enum"], []string{"deployed", "failed"}) {
		t.Fatalf("properties: %v", properties)
	}
}
