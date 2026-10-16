package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/google/jsonschema-go/jsonschema"
)

type ToolMutator func(tool api.ServerTool) api.ServerTool

// ComposeMutators combines multiple mutators into a single mutator that applies them in order
func ComposeMutators(mutators ...ToolMutator) ToolMutator {
	return func(tool api.ServerTool) api.ServerTool {
		for _, m := range mutators {
			tool = m(tool)
		}
		return tool
	}
}

// TargetsListToolName is the base name for the generic targets list tool before mutation
const TargetsListToolName = "targets_list"

// WithTargetParameter adds a target selection parameter to the tool's input schema if the tool is cluster-aware
func WithTargetParameter(defaultCluster, targetParameterName string, isMultiTarget bool) ToolMutator {
	return func(tool api.ServerTool) api.ServerTool {
		if !tool.IsClusterAware() {
			return tool
		}

		if tool.Tool.InputSchema == nil {
			tool.Tool.InputSchema = &jsonschema.Schema{Type: "object"}
		}

		if tool.Tool.InputSchema.Properties == nil {
			tool.Tool.InputSchema.Properties = make(map[string]*jsonschema.Schema)
		}

		if isMultiTarget {
			tool.Tool.InputSchema.Properties[targetParameterName] = createTargetProperty(
				defaultCluster,
				targetParameterName,
			)
		}

		return tool
	}
}

func createTargetProperty(defaultCluster, targetName string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "string",
		Description: fmt.Sprintf("The %s to run against, as listed by %s_list. Defaults to %s.", targetName, targetName, defaultCluster),
	}
}

// targetLister is a minimal interface for listing available targets.
// This reduces coupling with the kubernetes package.
type targetLister interface {
	GetTargets(ctx context.Context) ([]string, error)
}

// TargetEntry is one target in the result of the <target>_list tool.
//
// This is part of the tool's public wire contract: the JSON field tags are
// observed by MCP clients and must not be renamed without a coordinated
// migration. See docs/specs/structured-output.md.
type TargetEntry struct {
	Name    string `json:"name"`
	Server  string `json:"server,omitempty"`
	Default bool   `json:"default"`
}

// TargetsListResult is the structured-content payload of the <target>_list
// tool: the default target by name, and every target in lexicographic
// order. Part of the tool's public wire contract (docs/specs/structured-output.md).
type TargetsListResult struct {
	Default string        `json:"default"`
	Targets []TargetEntry `json:"targets"`
}

// WithTargetListTool shapes the generic "targets_list" tool for the
// provider: named <target>_list (context_list for a kubeconfig, cluster_list
// for ACM), described in the provider's terms, and answered from the
// provider's targets.
func WithTargetListTool(defaultTarget, targetParameterName string, p targetLister) ToolMutator {
	return func(tool api.ServerTool) api.ServerTool {
		if tool.Tool.Name != TargetsListToolName {
			return tool
		}
		tool.Tool.Name = targetParameterName + "_list"
		tool.Tool.Description = fmt.Sprintf("List the %ss this server can run tools against, and which one is the default. "+
			"Pass a %s name as the %s argument of any tool to run it there.", targetParameterName, targetParameterName, targetParameterName)
		tool.Tool.Annotations.Title = capitalizeFirst(targetParameterName) + ": List"
		tool.Tool.Examples = []api.Example{{Summary: fmt.Sprintf("Which %ss are available", targetParameterName)}}
		tool.Handler = createTargetListHandler(p, targetParameterName, defaultTarget)
		return tool
	}
}

func createTargetListHandler(p targetLister, targetParameterName, defaultTarget string) api.ToolHandlerFunc {
	return func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		var targets []kubernetes.TargetInfo
		var err error
		if describer, ok := p.(kubernetes.TargetDescriber); ok {
			targets, err = describer.DescribeTargets(params.Context)
		} else {
			var names []string
			names, err = p.GetTargets(params.Context)
			for _, name := range names {
				targets = append(targets, kubernetes.TargetInfo{Name: name})
			}
		}
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to list %ss: %w", targetParameterName, err)), nil
		}
		if len(targets) == 0 {
			// Structured content is intentionally omitted (nil) on this branch:
			// docs/specs/structured-output.md "Empty / nil results".
			return api.NewToolCallResult(fmt.Sprintf("No %ss available", targetParameterName), nil), nil
		}
		// Lexicographic order is the documented contract (docs/specs/structured-output.md).
		sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
		result := TargetsListResult{Default: defaultTarget, Targets: make([]TargetEntry, 0, len(targets))}
		for _, target := range targets {
			result.Targets = append(result.Targets, TargetEntry{Name: target.Name, Server: target.Server, Default: target.Name == defaultTarget})
		}
		text, err := output.MarshalYaml(result)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to list %ss: %w", targetParameterName, err)), nil
		}
		return api.NewToolCallResultFull(text, result, nil), nil
	}
}

func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// WithToolOverrides returns a mutator that applies per-tool configuration overrides
// (such as custom descriptions) from the user's config file.
func WithToolOverrides(overrides map[string]config.ToolOverride) ToolMutator {
	return func(tool api.ServerTool) api.ServerTool {
		if overrides == nil {
			return tool
		}
		if o, ok := overrides[tool.Tool.Name]; ok {
			if o.Description != "" {
				tool.Tool.Description = o.Description
			}
		}
		return tool
	}
}
