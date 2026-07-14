package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"

	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kcp"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/tekton"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type FilterProvider struct{}

func (p *FilterProvider) AnyTargetHasGVKs(_ context.Context, _ []schema.GroupVersionKind) bool {
	return true
}

func (p *FilterProvider) IsTargetCompatibilityToolFiltersEnabled() bool {
	return false
}

var _ api.FilteringProvider = (*FilterProvider)(nil)

type evalTask struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
}

type projectInfo struct {
	Name     string
	URL      string
	Toolsets map[string]bool
	Count    int
}

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

	// Validated Projects
	updated := generateValidatedProjects(string(readme))

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
	fmt.Fprintf(&availableToolsets, "| %-*s | %-*s | %-*s |\n", maxNameLen, "Toolset", maxDescLen, "Description", defaultHeaderLen, "Default")
	fmt.Fprintf(&availableToolsets, "|-%s-|-%s-|-%s-|\n", strings.Repeat("-", maxNameLen), strings.Repeat("-", maxDescLen), strings.Repeat("-", defaultHeaderLen))
	for _, toolset := range toolsetsList {
		defaultIndicator := ""
		if defaultToolsetsMap[toolset.GetName()] {
			defaultIndicator = "✓"
		}
		fmt.Fprintf(&availableToolsets, "| %-*s | %-*s | %-*s |\n", maxNameLen, toolset.GetName(), maxDescLen, toolset.GetDescription(), defaultHeaderLen, defaultIndicator)
	}
	updated = replaceBetweenMarkers(
		updated,
		"<!-- AVAILABLE-TOOLSETS-START -->",
		"<!-- AVAILABLE-TOOLSETS-END -->",
		availableToolsets.String(),
	)

	// Available Toolset Tools
	toolsetTools := strings.Builder{}
	for _, toolset := range toolsetsList {
		toolsetTools.WriteString("<details>\n\n<summary>" + toolset.GetName() + "</summary>\n\n")
		tools := toolset.GetTools(&FilterProvider{})
		for _, tool := range tools {
			fmt.Fprintf(&toolsetTools, "- **%s** - %s\n", tool.Tool.Name, tool.Tool.Description)
			for _, propName := range slices.Sorted(maps.Keys(tool.Tool.InputSchema.Properties)) {
				property := tool.Tool.InputSchema.Properties[propName]
				fmt.Fprintf(&toolsetTools, "  - `%s` (`%s`)", propName, property.Type)
				if slices.Contains(tool.Tool.InputSchema.Required, propName) {
					toolsetTools.WriteString(" **(required)**")
				}
				fmt.Fprintf(&toolsetTools, " - %s\n", property.Description)
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
			fmt.Fprintf(&toolsetPrompts, "- **%s** - %s\n", prompt.Prompt.Name, prompt.Prompt.Description)
			for _, arg := range prompt.Prompt.Arguments {
				fmt.Fprintf(&toolsetPrompts, "  - `%s` (`string`)", arg.Name)
				if arg.Required {
					toolsetPrompts.WriteString(" **(required)**")
				}
				fmt.Fprintf(&toolsetPrompts, " - %s\n", arg.Description)
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

	// Available Toolset Resources
	toolsetResources := strings.Builder{}
	for _, toolset := range toolsetsList {
		resources := toolset.GetResources()
		if len(resources) == 0 {
			continue
		}
		toolsetResources.WriteString("<details>\n\n<summary>" + toolset.GetName() + "</summary>\n\n")
		for _, resource := range resources {
			fmt.Fprintf(&toolsetResources, "- **%s** - %s\n", resource.Resource.Name, resource.Resource.Description)
			fmt.Fprintf(&toolsetResources, "  - URI: `%s`\n", resource.Resource.URI)
			fmt.Fprintf(&toolsetResources, "  - MIME Type: `%s`\n", resource.Resource.MIMEType)
		}
		toolsetResources.WriteString("</details>\n\n")
	}
	updated = replaceBetweenMarkers(
		updated,
		"<!-- AVAILABLE-TOOLSETS-RESOURCES-START -->",
		"<!-- AVAILABLE-TOOLSETS-RESOURCES-END -->",
		toolsetResources.String(),
	)

	// Available Toolset Resource Templates
	toolsetResourceTemplates := strings.Builder{}
	for _, toolset := range toolsetsList {
		templates := toolset.GetResourceTemplates()
		if len(templates) == 0 {
			continue
		}
		toolsetResourceTemplates.WriteString("<details>\n\n<summary>" + toolset.GetName() + "</summary>\n\n")
		for _, template := range templates {
			fmt.Fprintf(&toolsetResourceTemplates, "- **%s** - %s\n", template.ResourceTemplate.Name, template.ResourceTemplate.Description)
			fmt.Fprintf(&toolsetResourceTemplates, "  - URI Template: `%s`\n", template.ResourceTemplate.URITemplate)
			fmt.Fprintf(&toolsetResourceTemplates, "  - MIME Type: `%s`\n", template.ResourceTemplate.MIMEType)
		}
		toolsetResourceTemplates.WriteString("</details>\n\n")
	}
	updated = replaceBetweenMarkers(
		updated,
		"<!-- AVAILABLE-TOOLSETS-RESOURCES-TEMPLATES-START -->",
		"<!-- AVAILABLE-TOOLSETS-RESOURCES-TEMPLATES-END -->",
		toolsetResourceTemplates.String(),
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

func generateValidatedProjects(content string) string {
	evalsDir := "evals/tasks"
	projects := make(map[string]*projectInfo)
	displayNameToKey := make(map[string]string)

	err := filepath.Walk(evalsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".yaml" {
			return nil
		}
		if strings.Contains(path, "/artifacts/") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var task evalTask
		if err := yaml.Unmarshal(data, &task); err != nil {
			panic(fmt.Sprintf("failed to parse %q: %v", path, err))
		}

		if task.Kind != "Task" && task.Kind != "Prompt" {
			return nil
		}

		projectKey := task.Metadata.Labels["project"]
		if projectKey == "" {
			return nil
		}

		projectName := task.Metadata.Annotations["project-name"]
		projectURL := task.Metadata.Annotations["project-url"]

		if projectName == "" || projectURL == "" {
			panic(fmt.Sprintf("task %q has project label %q but is missing project-name or project-url annotations", path, projectKey))
		}

		if existingKey, ok := displayNameToKey[projectName]; ok && existingKey != projectKey {
			panic(fmt.Sprintf("display name %q is used by both project keys %q and %q", projectName, existingKey, projectKey))
		}
		displayNameToKey[projectName] = projectKey

		p, exists := projects[projectKey]
		if !exists {
			p = &projectInfo{
				Name:     projectName,
				URL:      projectURL,
				Toolsets: map[string]bool{},
			}
			projects[projectKey] = p
		} else {
			if p.Name != projectName {
				panic(fmt.Sprintf("project %q has conflicting names: %q vs %q", projectKey, p.Name, projectName))
			}
			if p.URL != projectURL {
				panic(fmt.Sprintf("project %q has conflicting URLs: %q vs %q", projectKey, p.URL, projectURL))
			}
		}

		p.Count++

		if requires := task.Metadata.Labels["requires"]; requires != "" {
			for _, ts := range strings.Split(requires, ",") {
				if ts = strings.TrimSpace(ts); ts != "" {
					p.Toolsets[ts] = true
				}
			}
		}

		return nil
	})
	if err != nil {
		panic(err)
	}

	type row struct {
		name     string
		url      string
		toolsets string
		count    int
	}
	var rows []row
	for _, p := range projects {
		extras := slices.Sorted(maps.Keys(p.Toolsets))
		var toolsetsStr string
		if len(extras) == 0 {
			toolsetsStr = "-"
		} else {
			toolsetsStr = "`" + strings.Join(extras, "`, `") + "`"
		}
		rows = append(rows, row{name: p.Name, url: p.URL, toolsets: toolsetsStr, count: p.Count})
	}
	slices.SortFunc(rows, func(a, b row) int {
		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})

	table := strings.Builder{}
	table.WriteString("| Project | Optional toolset(s) | Eval scenarios |\n")
	table.WriteString("|---------|---------------------|----------------|\n")
	for _, r := range rows {
		fmt.Fprintf(&table, "| [%s](%s) | %s | %d |\n", r.name, r.url, r.toolsets, r.count)
	}

	return replaceBetweenMarkers(content, "<!-- VALIDATED-PROJECTS-START -->", "<!-- VALIDATED-PROJECTS-END -->", table.String())
}
