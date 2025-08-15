package mcp

import (
	"testing"
)

func TestInitOlmTools(t *testing.T) {
	s := &Server{}
	tools := s.initOlm()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, t := range tools {
		names[t.Tool.Name] = true
	}
	if !names["olm_install"] || !names["olm_list"] || !names["olm_uninstall"] || !names["olm_upgrade"] {
		t.Fatalf("missing expected olm tool names: %v", names)
	}
}
