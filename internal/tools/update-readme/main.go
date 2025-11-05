package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"

	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali"
)

type OpenShift struct{}

func (o *OpenShift) IsOpenShift(ctx context.Context) bool {
	return true
}

var _ internalk8s.Openshift = (*OpenShift)(nil)

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
	maxNameLen, maxDescLen := len("Toolset"), len("Description")
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
	availableToolsets.WriteString(fmt.Sprintf("| %-*s | %-*s |\n", maxNameLen, "Toolset", maxDescLen, "Description"))
	availableToolsets.WriteString(fmt.Sprintf("|-%s-|-%s-|\n", strings.Repeat("-", maxNameLen), strings.Repeat("-", maxDescLen)))
	for _, toolset := range toolsetsList {
		availableToolsets.WriteString(fmt.Sprintf("| %-*s | %-*s |\n", maxNameLen, toolset.GetName(), maxDescLen, toolset.GetDescription()))
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
