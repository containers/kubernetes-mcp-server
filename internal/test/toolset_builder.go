package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToolsetBuilderOptions configures how to build the expected tool list for tests
type ToolsetBuilderOptions struct {
	// Toolsets to include (e.g., "core", "config", "helm")
	Toolsets []string

	// Environment flags
	IsOpenShift   bool
	IsMultiCluster bool

	// MultiCluster configuration
	Contexts       []string
	DefaultContext string
	TargetParamName string // e.g., "context" for kubeconfig contexts
}

// testMetadata contains test-specific metadata for conditional tool inclusion/modification
type testMetadata struct {
	RequiresOpenShift   bool  `json:"requires_openshift,omitempty"`
	RequiresMultiCluster bool  `json:"requires_multicluster,omitempty"`
	ClusterAware        *bool `json:"cluster_aware,omitempty"`
	TargetListProvider  bool  `json:"target_list_provider,omitempty"`
}

// toolWithMetadata extends mcp.Tool with test metadata for conditional inclusion
type toolWithMetadata struct {
	mcp.Tool
	TestMetadata *testMetadata `json:"test_metadata,omitempty"`
}

// BuildExpectedToolsJSON constructs the expected tool list JSON based on options
// This mirrors the runtime behavior of toolset registration, filtering, and mutation
// Returns a JSON string that can be compared with actual tools using JSONEq
func BuildExpectedToolsJSON(opts ToolsetBuilderOptions) string {
	// Use runtime.Caller to find where this function was called from (the test file)
	// Skip 1 frame to get the caller (the test function)
	_, callerFile, _, _ := runtime.Caller(1)
	testdataDir := filepath.Join(filepath.Dir(callerFile), "testdata")

	tools := buildExpectedTools(opts, testdataDir)

	// Marshal to JSON with indentation to match test.ReadFile format
	jsonBytes := Must(json.MarshalIndent(tools, "", "  "))
	return string(jsonBytes)
}

// buildExpectedTools constructs the expected tool list based on options
func buildExpectedTools(opts ToolsetBuilderOptions, testdataDir string) []mcp.Tool {
	// Set defaults
	if opts.TargetParamName == "" {
		opts.TargetParamName = "context"
	}

	// Load and merge toolset JSONs
	toolsWithMeta := loadToolsets(opts.Toolsets, testdataDir)

	// Apply filters
	toolsWithMeta = filterTools(toolsWithMeta, opts)

	// Apply mutations and extract clean tools
	tools := mutateTools(toolsWithMeta, opts)

	// Sort tools by name to match server output
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	return tools
}

// loadToolsets loads and merges JSON files for the specified toolsets from the given testdata directory
func loadToolsets(toolsets []string, testdataDir string) []toolWithMetadata {
	var allTools []toolWithMetadata

	for _, toolset := range toolsets {
		filename := "toolsets-" + toolset + "-tools.json"
		testdataPath := filepath.Join(testdataDir, filename)

		content := Must(os.ReadFile(testdataPath))

		var tools []toolWithMetadata
		Must(tools, json.Unmarshal(content, &tools))

		allTools = append(allTools, tools...)
	}

	return allTools
}

// filterTools removes tools that don't match the environment conditions
func filterTools(tools []toolWithMetadata, opts ToolsetBuilderOptions) []toolWithMetadata {
	var filtered []toolWithMetadata

	for _, tool := range tools {
		meta := tool.TestMetadata
		if meta == nil {
			// No metadata means no conditions - always include
			filtered = append(filtered, tool)
			continue
		}

		// Skip if requires OpenShift but not in OpenShift
		if meta.RequiresOpenShift && !opts.IsOpenShift {
			continue
		}

		// Skip if requires multicluster but not in multicluster
		if meta.RequiresMultiCluster && !opts.IsMultiCluster {
			continue
		}

		// Skip target list providers if only one target
		// (mirrors ShouldIncludeTargetListTool filter)
		if meta.TargetListProvider && len(opts.Contexts) <= 1 {
			continue
		}

		filtered = append(filtered, tool)
	}

	return filtered
}

// mutateTools applies transformations like adding context parameters and modifying descriptions
func mutateTools(toolsWithMeta []toolWithMetadata, opts ToolsetBuilderOptions) []mcp.Tool {
	tools := make([]mcp.Tool, len(toolsWithMeta))

	for i, toolMeta := range toolsWithMeta {
		tool := toolMeta.Tool

		// Add context parameter for cluster-aware tools in multicluster mode
		if opts.IsMultiCluster && isClusterAware(toolMeta) {
			addContextParameter(&tool.InputSchema, opts.TargetParamName, opts.DefaultContext, opts.Contexts)
		}

		// Add OpenShift-specific text to resource tool descriptions
		if opts.IsOpenShift && isResourceTool(tool.Name) {
			tool.Description = addOpenShiftToDescription(tool.Description)
		}

		tools[i] = tool
	}

	return tools
}

// isResourceTool checks if a tool is one of the generic resource tools that get OpenShift descriptions
func isResourceTool(name string) bool {
	return strings.HasPrefix(name, "resources_")
}

// addOpenShiftToDescription adds OpenShift-specific resource types to tool descriptions
// Mirrors the logic in pkg/toolsets/core/resources.go initResources function
func addOpenShiftToDescription(description string) string {
	// Replace the closing parenthesis with OpenShift route type
	return strings.Replace(description,
		"networking.k8s.io/v1 Ingress)",
		"networking.k8s.io/v1 Ingress, route.openshift.io/v1 Route)",
		1)
}

// isClusterAware checks if a tool should receive the context parameter
func isClusterAware(tool toolWithMetadata) bool {
	// If explicitly set in metadata, use that value
	if tool.TestMetadata != nil && tool.TestMetadata.ClusterAware != nil {
		return *tool.TestMetadata.ClusterAware
	}

	// Default to true (mirrors api.ServerTool.IsClusterAware)
	return true
}

// addContextParameter adds a context/cluster parameter to the tool's input schema (mutates in place)
// This mirrors the WithTargetParameter ToolMutator
func addContextParameter(schema *mcp.ToolInputSchema, paramName, defaultContext string, contexts []string) {
	// Don't add if only one context
	if len(contexts) <= 1 {
		return
	}

	// Ensure schema has properties map
	if schema.Properties == nil {
		schema.Properties = make(map[string]any)
	}

	// Create the context property
	contextProp := map[string]any{
		"type": "string",
		"description": "Optional parameter selecting which " + paramName +
			" to run the tool in. Defaults to " + defaultContext + " if not set",
	}

	// Add enum if <= 5 contexts (mirrors maxTargetsInEnum constant)
	if len(contexts) <= 5 {
		// Sort contexts to ensure consistent enum ordering
		sorted := make([]string, len(contexts))
		copy(sorted, contexts)
		sort.Strings(sorted)

		enumValues := make([]any, len(sorted))
		for i, c := range sorted {
			enumValues[i] = c
		}
		contextProp["enum"] = enumValues
	}

	schema.Properties[paramName] = contextProp
}
