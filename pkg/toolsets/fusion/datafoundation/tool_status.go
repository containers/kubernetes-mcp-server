package datafoundation

import (
	"context"
	"encoding/json"

	"github.com/containers/kubernetes-mcp-server/internal/fusion/clients"
	"github.com/containers/kubernetes-mcp-server/internal/fusion/services"
	"github.com/containers/kubernetes-mcp-server/internal/fusion/targeting"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"
)

// InitStatusTool creates the fusion.datafoundation.status tool
func InitStatusTool() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.datafoundation.status",
			Description: "Get Data Foundation (ODF/OCS) status across clusters including installation status, storage classes, and Ceph health",
			Annotations: api.ToolAnnotations{
				Title:        "Data Foundation Status",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type: jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{
					"target": targeting.TargetSchema(),
				},
			},
		},
		Handler: handleDataFoundationStatus,
	}
}

// handleDataFoundationStatus implements the Data Foundation status tool handler
func handleDataFoundationStatus(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse target
	var input struct {
		Target targeting.Target `json:"target"`
	}
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		// Default to single cluster if no target specified
		input.Target = targeting.Target{Type: targeting.TargetSingle}
	}

	// Get or create registry
	registry := clients.GetOrCreateRegistry(params.KubernetesClient)

	// Execute on clusters
	result := services.ExecuteOnClusters(params.Context, registry, input.Target, func(ctx context.Context, client *clients.ClusterClient) (interface{}, error) {
		service := services.NewDataFoundationService(nil)
		return service.GetStatus(ctx, client)
	})

	// Marshal result to JSON
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// Made with Bob