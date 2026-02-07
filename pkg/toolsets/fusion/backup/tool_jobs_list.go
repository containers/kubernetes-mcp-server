package backup

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

// InitJobsListTool creates the fusion.backup.jobs.list tool
func InitJobsListTool() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.backup.jobs.list",
			Description: "List backup jobs across clusters including OADP/Velero backups with status and age",
			Annotations: api.ToolAnnotations{
				Title:        "Backup Jobs List",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type: jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{
					"target": targeting.TargetSchema(),
				},
			},
		},
		Handler: handleBackupJobsList,
	}
}

// handleBackupJobsList implements the backup jobs list tool handler
func handleBackupJobsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse target
	var input struct {
		Target targeting.Target `json:"target"`
	}
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		input.Target = targeting.Target{Type: targeting.TargetSingle}
	}

	// Get or create registry
	registry := clients.GetOrCreateRegistry(params.KubernetesClient)

	// Execute on clusters
	result := services.ExecuteOnClusters(params.Context, registry, input.Target, func(ctx context.Context, client *clients.ClusterClient) (interface{}, error) {
		service := services.NewBackupService(nil)
		return service.ListJobs(ctx, client)
	})

	// Marshal result to JSON
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// Made with Bob