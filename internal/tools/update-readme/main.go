package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"

	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/code"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kcp"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt"
)

type OpenShift struct{}

func (o *OpenShift) IsOpenShift(_ context.Context) bool {
	return true
}

var _ api.Openshift = (*OpenShift)(nil)

func main() {
	// Snyk reports false positive unless we flow the args through filepath.Clean and filepath.Localize in this specific order
	var err error
	localReadmePath := filepath.Clean(os.Args[1])
	localReadmePath, err = filepath.Localize(localReadmePath)
	if err != nil {
		panic(err)
	}
	readme, err := os.ReadFile(localReadmePath)
	if err != nil {
		panic(err)
	}
	// Available Toolsets
	toolsetsList := toolsets.Toolsets()

	// Get default enabled toolsets
	defaultConfig := config.Default()
	defaultToolsetsMap := make(map[string]bool)
	for _, toolsetName := range defaultConfig.Toolsets {
		defaultToolsetsMap[toolsetName] = true
	}

	maxNameLen, maxDescLen := len("Toolset"), len("Description")
	defaultHeaderLen := len("Default")
	for _, toolset := range toolsetsList {
		nameLen := len(toolset.GetName())
		descLen := len(toolset.GetDescription())
		if nameLen > maxNameLen {
			maxNameLen = nameLen
		}
		if descLen > maxDescLen {
			maxDescLen = descLen
		}
	}
	availableToolsets := strings.Builder{}
	availableToolsets.WriteString(fmt.Sprintf("| %-*s | %-*s | %-*s |\n", maxNameLen, "Toolset", maxDescLen, "Description", defaultHeaderLen, "Default"))
	availableToolsets.WriteString(fmt.Sprintf("|-%s-|-%s-|-%s-|\n", strings.Repeat("-", maxNameLen), strings.Repeat("-", maxDescLen), strings.Repeat("-", defaultHeaderLen)))
	for _, toolset := range toolsetsList {
		defaultIndicator := ""
		if defaultToolsetsMap[toolset.GetName()] {
			defaultIndicator = "✓"
		}
		availableToolsets.WriteString(fmt.Sprintf("| %-*s | %-*s | %-*s |\n", maxNameLen, toolset.GetName(), maxDescLen, toolset.GetDescription(), defaultHeaderLen, defaultIndicator))
	}
	updated := replaceBetweenMarkers(
		string(readme),
		"<!-- AVAILABLE-TOOLSETS-START -->",
		"<!-- AVAILABLE-TOOLSETS-END -->",
		availableToolsets.String(),
	)

	// Available Toolset Tools
	toolsetTools := strings.Builder{}
	for _, toolset := range toolsetsList {
		toolsetTools.WriteString("<details>\n\n<summary>" + toolset.GetName() + "</summary>\n\n")
		tools := toolset.GetTools(&OpenShift{})
		for _, tool := range tools {
			toolsetTools.WriteString(fmt.Sprintf("- **%s** - %s\n", tool.Tool.Name, tool.Tool.Description))
			for _, propName := range slices.Sorted(maps.Keys(tool.Tool.InputSchema.Properties)) {
				property := tool.Tool.InputSchema.Properties[propName]
				toolsetTools.WriteString(fmt.Sprintf("  - `%s` (`%s`)", propName, property.Type))
				if slices.Contains(tool.Tool.InputSchema.Required, propName) {
					toolsetTools.WriteString(" **(required)**")
				}
				toolsetTools.WriteString(fmt.Sprintf(" - %s\n", property.Description))
			}
			toolsetTools.WriteString("\n")
		}
		toolsetTools.WriteString("</details>\n\n")
	}
	updated = replaceBetweenMarkers(
		updated,
		"<!-- AVAILABLE-TOOLSETS-TOOLS-START -->",
		"<!-- AVAILABLE-TOOLSETS-TOOLS-END -->",
		toolsetTools.String(),
	)

	// Available Toolset Prompts
	toolsetPrompts := strings.Builder{}
	for _, toolset := range toolsetsList {
		prompts := toolset.GetPrompts()
		if len(prompts) == 0 {
			continue
		}
		toolsetPrompts.WriteString("<details>\n\n<summary>" + toolset.GetName() + "</summary>\n\n")
		for _, prompt := range prompts {
			toolsetPrompts.WriteString(fmt.Sprintf("- **%s** - %s\n", prompt.Prompt.Name, prompt.Prompt.Description))
			for _, arg := range prompt.Prompt.Arguments {
				toolsetPrompts.WriteString(fmt.Sprintf("  - `%s` (`string`)", arg.Name))
				if arg.Required {
					toolsetPrompts.WriteString(" **(required)**")
				}
				toolsetPrompts.WriteString(fmt.Sprintf(" - %s\n", arg.Description))
			}
			toolsetPrompts.WriteString("\n")
		}
		toolsetPrompts.WriteString("</details>\n\n")
	}
	updated = replaceBetweenMarkers(
		updated,
		"<!-- AVAILABLE-TOOLSETS-PROMPTS-START -->",
		"<!-- AVAILABLE-TOOLSETS-PROMPTS-END -->",
		toolsetPrompts.String(),
	)

	if err := os.WriteFile(localReadmePath, []byte(updated), 0o644); err != nil {
		panic(err)
	}
}

func replaceBetweenMarkers(content, startMarker, endMarker, replacement string) string {
	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return content
	}
	endIdx := strings.Index(content, endMarker)
	if endIdx == -1 || endIdx <= startIdx {
		return content
	}
	return content[:startIdx+len(startMarker)] + "\n\n" + replacement + "\n" + content[endIdx:]
}
