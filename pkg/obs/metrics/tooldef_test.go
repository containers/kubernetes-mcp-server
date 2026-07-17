package metrics

import (
	"testing"
)

type testOutput struct {
	Result string `json:"result"`
}

func TestToServerTool_AdditionalFieldsFlatInMeta(t *testing.T) {
	def := ToolDef[testOutput]{
		Name:        "test_tool",
		Description: "A test tool",
		AdditionalFields: map[string]any{
			"olsUi": map[string]any{
				"id": "mcp-obs/show-timeseries",
			},
		},
	}

	serverTool := def.ToServerTool(nil)

	if serverTool.Tool.Meta == nil {
		t.Fatal("expected Meta to be set, got nil")
	}

	olsUi, ok := serverTool.Tool.Meta["olsUi"]
	if !ok {
		t.Fatal("expected 'olsUi' key directly in Meta, not nested under 'AdditionalFields'")
	}

	olsUiMap, ok := olsUi.(map[string]any)
	if !ok {
		t.Fatalf("expected olsUi to be map[string]any, got %T", olsUi)
	}

	if id := olsUiMap["id"]; id != "mcp-obs/show-timeseries" {
		t.Errorf("expected id 'mcp-obs/show-timeseries', got %v", id)
	}

	if _, ok := serverTool.Tool.Meta["AdditionalFields"]; ok {
		t.Error("Meta should not contain an 'AdditionalFields' wrapper key")
	}
}

func TestToServerTool_NilAdditionalFields(t *testing.T) {
	def := ToolDef[testOutput]{
		Name:        "test_tool",
		Description: "A test tool",
	}

	serverTool := def.ToServerTool(nil)

	if serverTool.Tool.Meta != nil {
		t.Errorf("expected Meta to be nil when AdditionalFields is nil, got %v", serverTool.Tool.Meta)
	}
}

func TestToMCPTool_AdditionalFieldsFlatInMeta(t *testing.T) {
	def := ToolDef[testOutput]{
		Name:        "test_tool",
		Description: "A test tool",
		AdditionalFields: map[string]any{
			"olsUi": map[string]any{
				"id": "mcp-obs/show-timeseries",
			},
		},
	}

	tool := def.ToMCPTool()

	if tool.Meta == nil {
		t.Fatal("expected Meta to be set, got nil")
	}

	olsUi, ok := tool.Meta["olsUi"]
	if !ok {
		t.Fatal("expected 'olsUi' key directly in Meta")
	}

	olsUiMap, ok := olsUi.(map[string]any)
	if !ok {
		t.Fatalf("expected olsUi to be map[string]any, got %T", olsUi)
	}

	if id := olsUiMap["id"]; id != "mcp-obs/show-timeseries" {
		t.Errorf("expected id 'mcp-obs/show-timeseries', got %v", id)
	}

	if _, ok := tool.Meta["AdditionalFields"]; ok {
		t.Error("Meta should not contain an 'AdditionalFields' wrapper key")
	}
}
