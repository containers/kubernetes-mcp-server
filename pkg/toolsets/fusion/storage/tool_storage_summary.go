package storage

import (
	"encoding/json"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/internal/fusion/clients"
	"github.com/containers/kubernetes-mcp-server/internal/fusion/services"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"
)

// InitStorageSummary creates the fusion.storage.summary tool
func InitStorageSummary() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.storage.summary",
			Description: "Get a comprehensive summary of storage status for IBM Fusion/OpenShift including storage classes, PVC statistics, and ODF/OCS detection",
			Annotations: api.ToolAnnotations{
				Title:        "IBM Fusion Storage Summary",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type:       jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{
					// No input parameters for now
				},
			},
		},
		Handler: handleStorageSummary,
	}
}

// handleStorageSummary implements the storage summary tool handler
func handleStorageSummary(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Create Fusion Kubernetes client wrapper
	fusionClient := clients.NewKubernetesClient(params.KubernetesClient)

	// Create storage service
	storageService := services.NewStorageService(fusionClient)

	// Get storage summary
	summary, err := storageService.GetStorageSummary(params.Context)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get storage summary: %w", err)), nil
	}

	// Create output structure
	output := StorageSummaryOutput{
		Summary: summary,
	}

	// Marshal to JSON
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal output: %w", err)), nil
	}

	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// Made with Bob
