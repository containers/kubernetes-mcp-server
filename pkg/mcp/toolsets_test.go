package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func TestFullToolsetTools(t *testing.T) {
	expectedNames := []string{
		"configuration_view",
		"events_list",
		"helm_install",
		"helm_list",
		"helm_uninstall",
		"namespaces_list",
		"pods_list",
		"pods_list_in_namespace",
		"pods_get",
		"pods_delete",
		"pods_top",
		"pods_log",
		"pods_run",
		"pods_exec",
		"resources_list",
		"resources_get",
		"resources_create_or_update",
		"resources_delete",
	}
	mcpCtx := &mcpContext{toolset: &Full{}}
	testCaseWithContext(t, mcpCtx, func(c *mcpContext) {
		tools, err := c.mcpClient.ListTools(c.ctx, mcp.ListToolsRequest{})
		t.Run("ListTools returns tools", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call ListTools failed %v", err)
				return
			}
		})
		nameSet := make(map[string]bool)
		for _, tool := range tools.Tools {
			nameSet[tool.Name] = true
		}
		for _, name := range expectedNames {
			t.Run("ListTools has "+name+" tool", func(t *testing.T) {
				if nameSet[name] != true {
					t.Fatalf("tool %s not found", name)
					return
				}
			})
		}
		t.Run("ListTools returns correct Tool metadata for toolset", func(t *testing.T) {
			_, file, _, _ := runtime.Caller(0)
			expectedMetadataPath := filepath.Join(filepath.Dir(file), "testdata", "toolsets-full-tools.json")
			expectedMetadataBytes, err := os.ReadFile(expectedMetadataPath)
			if err != nil {
				t.Fatalf("failed to read expected tools metadata file: %v", err)
			}
			metadata, err := json.MarshalIndent(tools.Tools, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal tools metadata: %v", err)
			}
			assert.JSONEqf(t, string(expectedMetadataBytes), string(metadata), "tools metadata does not match expected")
		})
	})
}

func TestFullToolsetToolsInOpenShift(t *testing.T) {
	mcpCtx := &mcpContext{
		toolset: &Full{},
		before:  inOpenShift,
		after:   inOpenShiftClear,
	}
	testCaseWithContext(t, mcpCtx, func(c *mcpContext) {
		tools, err := c.mcpClient.ListTools(c.ctx, mcp.ListToolsRequest{})
		t.Run("ListTools returns tools", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call ListTools failed %v", err)
			}
		})
		t.Run("ListTools contains projects_list tool", func(t *testing.T) {
			idx := slices.IndexFunc(tools.Tools, func(tool mcp.Tool) bool {
				return tool.Name == "projects_list"
			})
			if idx == -1 {
				t.Fatalf("tool projects_list not found")
			}
		})
		t.Run("ListTools has resources_list tool with OpenShift hint", func(t *testing.T) {
			idx := slices.IndexFunc(tools.Tools, func(tool mcp.Tool) bool {
				return tool.Name == "resources_list"
			})
			if idx == -1 {
				t.Fatalf("tool resources_list not found")
			}
			if !strings.Contains(tools.Tools[idx].Description, ", route.openshift.io/v1 Route") {
				t.Fatalf("tool resources_list does not have OpenShift hint, got %s", tools.Tools[9].Description)
			}
		})
		t.Run("ListTools returns correct Tool metadata for toolset", func(t *testing.T) {
			_, file, _, _ := runtime.Caller(0)
			expectedMetadataPath := filepath.Join(filepath.Dir(file), "testdata", "toolsets-full-tools-openshift.json")
			expectedMetadataBytes, err := os.ReadFile(expectedMetadataPath)
			if err != nil {
				t.Fatalf("failed to read expected tools metadata file: %v", err)
			}
			metadata, err := json.MarshalIndent(tools.Tools, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal tools metadata: %v", err)
			}
			assert.JSONEqf(t, string(expectedMetadataBytes), string(metadata), "tools metadata does not match expected")
		})
	})
}
