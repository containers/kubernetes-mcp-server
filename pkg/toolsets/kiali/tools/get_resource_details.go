package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

type listDetailsOperations struct {
	singularName string
	listFunc     func(ctx context.Context, k *kialiclient.Kiali, namespaces string) (string, error)
	detailsFunc  func(ctx context.Context, k *kialiclient.Kiali, namespace, name string) (string, error)
}

var listDetailsOpsMap = map[string]listDetailsOperations{
	"service": {
		singularName: "service",
		listFunc: func(ctx context.Context, k *kialiclient.Kiali, nss string) (string, error) {
			return k.ServicesList(ctx, nss)
		},
		detailsFunc: func(ctx context.Context, k *kialiclient.Kiali, ns, name string) (string, error) {
			return k.ServiceDetails(ctx, ns, name)
		},
	},
	"workload": {
		singularName: "workload",
		listFunc: func(ctx context.Context, k *kialiclient.Kiali, nss string) (string, error) {
			return k.WorkloadsList(ctx, nss)
		},
		detailsFunc: func(ctx context.Context, k *kialiclient.Kiali, ns, name string) (string, error) {
			return k.WorkloadDetails(ctx, ns, name)
		},
	},
}

func InitGetResourceDetails() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_resource_details"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Gets lists or detailed info for Kubernetes resources (services, workloads) within the mesh",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"resource_type": {
						Type:        "string",
						Description: "Type of resource to get details for (service, workload)",
						Enum:        []any{"service", "workload"},
					},
					"namespaces": {
						Type:        "string",
						Description: "Comma-separated list of namespaces to get services from (e.g. 'bookinfo' or 'bookinfo,default'). If not provided, will list services from all accessible namespaces",
					},
					"resource_name": {
						Type:        "string",
						Description: "Name of the resource to get details for (optional string - if provided, gets details; if empty, lists all).",
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "List or Resource Details",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: resourceDetailsHandler,
	})

	return ret
}

func resourceDetailsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	resourceType, _ := params.GetArguments()["resource_type"].(string)
	namespaces, _ := params.GetArguments()["namespaces"].(string)
	resourceName, _ := params.GetArguments()["resource_name"].(string)

	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	namespaces = strings.TrimSpace(namespaces)
	resourceName = strings.TrimSpace(resourceName)

	if resourceType == "" {
		return api.NewToolCallResult("", fmt.Errorf("resource_type is required")), nil
	}

	kiali := kialiclient.NewKiali(params, params.RESTConfig())

	ops, ok := listDetailsOpsMap[resourceType]
	if !ok {
		return api.NewToolCallResult("", fmt.Errorf("invalid resource type: %s", resourceType)), nil
	}

	// If a resource name is provided, fetch details. Requires exactly one namespace.
	if resourceName != "" {
		if count := len(strings.Split(namespaces, ",")); count != 1 {
			return api.NewToolCallResult("", fmt.Errorf("exactly one namespace must be provided for %s details", ops.singularName)), nil
		}
		content, err := ops.detailsFunc(params.Context, kiali, namespaces, resourceName)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to get %s details: %w", ops.singularName, err)), nil
		}
		return api.NewToolCallResult(content, nil), nil
	}

	// Otherwise, list resources (supports multiple namespaces)
	content, err := ops.listFunc(params.Context, kiali, namespaces)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list %ss: %w", ops.singularName, err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
