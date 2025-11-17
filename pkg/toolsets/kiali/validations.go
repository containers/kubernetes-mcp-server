package kiali

import (
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initValidations() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "kiali_validations_list",
			Description: "List all the validations in the current cluster from all namespaces",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Optional single namespace to retrieve validations from (alternative to namespaces)",
					},
					"namespaces": {
						Type:        "string",
						Description: "Optional comma-separated list of namespaces to retrieve validations from",
					},
				},
				Required: []string{},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Validations: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: validationsList,
	})
	return ret
}

func validationsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse arguments: allow either `namespace` or `namespaces` (comma-separated string)
	namespaces := make([]string, 0)
	if v, ok := params.GetArguments()["namespace"].(string); ok {
		v = strings.TrimSpace(v)
		if v != "" {
			namespaces = append(namespaces, v)
		}
	}
	if v, ok := params.GetArguments()["namespaces"].(string); ok {
		for _, ns := range strings.Split(v, ",") {
			ns = strings.TrimSpace(ns)
			if ns != "" {
				namespaces = append(namespaces, ns)
			}
		}
	}
	// Deduplicate namespaces if both provided
	if len(namespaces) > 1 {
		seen := map[string]struct{}{}
		unique := make([]string, 0, len(namespaces))
		for _, ns := range namespaces {
			key := strings.TrimSpace(ns)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			unique = append(unique, key)
		}
		namespaces = unique
	}

	k := params.NewKiali()
	content, err := k.ValidationsList(params.Context, namespaces)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list validations: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
