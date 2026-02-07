package alltools

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

// InitGDPStatusTool creates the fusion.gdp.status tool
func InitGDPStatusTool() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.gdp.status",
			Description: "Get Global Data Platform (IBM Spectrum Scale) status across clusters",
			Annotations: api.ToolAnnotations{
				Title:        "GDP Status",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type:       jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{"target": targeting.TargetSchema()},
			},
		},
		Handler: handleGDPStatus,
	}
}

func handleGDPStatus(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var input struct{ Target targeting.Target }
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		input.Target = targeting.Target{Type: targeting.TargetSingle}
	}
	registry := clients.GetOrCreateRegistry(params.KubernetesClient)
	result := services.ExecuteOnClusters(params.Context, registry, input.Target, func(ctx context.Context, client *clients.ClusterClient) (interface{}, error) {
		return services.NewGDPService().GetStatus(ctx, client)
	})
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// InitDRStatusTool creates the fusion.dr.status tool
func InitDRStatusTool() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.dr.status",
			Description: "Get Disaster Recovery status across clusters including Metro DR and Regional DR",
			Annotations: api.ToolAnnotations{
				Title:        "DR Status",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type:       jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{"target": targeting.TargetSchema()},
			},
		},
		Handler: handleDRStatus,
	}
}

func handleDRStatus(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var input struct{ Target targeting.Target }
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		input.Target = targeting.Target{Type: targeting.TargetSingle}
	}
	registry := clients.GetOrCreateRegistry(params.KubernetesClient)
	result := services.ExecuteOnClusters(params.Context, registry, input.Target, func(ctx context.Context, client *clients.ClusterClient) (interface{}, error) {
		return services.NewDRService().GetStatus(ctx, client)
	})
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// InitCatalogStatusTool creates the fusion.catalog.status tool
func InitCatalogStatusTool() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.catalog.status",
			Description: "Get Data Cataloging status across clusters",
			Annotations: api.ToolAnnotations{
				Title:        "Data Catalog Status",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type:       jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{"target": targeting.TargetSchema()},
			},
		},
		Handler: handleCatalogStatus,
	}
}

func handleCatalogStatus(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var input struct{ Target targeting.Target }
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		input.Target = targeting.Target{Type: targeting.TargetSingle}
	}
	registry := clients.GetOrCreateRegistry(params.KubernetesClient)
	result := services.ExecuteOnClusters(params.Context, registry, input.Target, func(ctx context.Context, client *clients.ClusterClient) (interface{}, error) {
		return services.NewCatalogService().GetStatus(ctx, client)
	})
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// InitCASStatusTool creates the fusion.cas.status tool
func InitCASStatusTool() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.cas.status",
			Description: "Get Content Aware Storage status across clusters",
			Annotations: api.ToolAnnotations{
				Title:        "CAS Status",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type:       jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{"target": targeting.TargetSchema()},
			},
		},
		Handler: handleCASStatus,
	}
}

func handleCASStatus(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var input struct{ Target targeting.Target }
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		input.Target = targeting.Target{Type: targeting.TargetSingle}
	}
	registry := clients.GetOrCreateRegistry(params.KubernetesClient)
	result := services.ExecuteOnClusters(params.Context, registry, input.Target, func(ctx context.Context, client *clients.ClusterClient) (interface{}, error) {
		return services.NewCASService().GetStatus(ctx, client)
	})
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// InitServiceabilitySummaryTool creates the fusion.serviceability.summary tool
func InitServiceabilitySummaryTool() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.serviceability.summary",
			Description: "Get serviceability summary across clusters including must-gather and logging",
			Annotations: api.ToolAnnotations{
				Title:        "Serviceability Summary",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type:       jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{"target": targeting.TargetSchema()},
			},
		},
		Handler: handleServiceabilitySummary,
	}
}

func handleServiceabilitySummary(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var input struct{ Target targeting.Target }
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		input.Target = targeting.Target{Type: targeting.TargetSingle}
	}
	registry := clients.GetOrCreateRegistry(params.KubernetesClient)
	result := services.ExecuteOnClusters(params.Context, registry, input.Target, func(ctx context.Context, client *clients.ClusterClient) (interface{}, error) {
		return services.NewServiceabilityService().GetSummary(ctx, client)
	})
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// InitObservabilitySummaryTool creates the fusion.observability.summary tool
func InitObservabilitySummaryTool() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.observability.summary",
			Description: "Get observability summary across clusters including Prometheus, Grafana, and OpenTelemetry",
			Annotations: api.ToolAnnotations{
				Title:        "Observability Summary",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type:       jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{"target": targeting.TargetSchema()},
			},
		},
		Handler: handleObservabilitySummary,
	}
}

func handleObservabilitySummary(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var input struct{ Target targeting.Target }
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		input.Target = targeting.Target{Type: targeting.TargetSingle}
	}
	registry := clients.GetOrCreateRegistry(params.KubernetesClient)
	result := services.ExecuteOnClusters(params.Context, registry, input.Target, func(ctx context.Context, client *clients.ClusterClient) (interface{}, error) {
		return services.NewObservabilityService().GetSummary(ctx, client)
	})
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// InitVirtualizationStatusTool creates the fusion.virtualization.status tool
func InitVirtualizationStatusTool() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.virtualization.status",
			Description: "Get virtualization status across clusters including KubeVirt and OpenShift Virtualization",
			Annotations: api.ToolAnnotations{
				Title:        "Virtualization Status",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type:       jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{"target": targeting.TargetSchema()},
			},
		},
		Handler: handleVirtualizationStatus,
	}
}

func handleVirtualizationStatus(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var input struct{ Target targeting.Target }
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		input.Target = targeting.Target{Type: targeting.TargetSingle}
	}
	registry := clients.GetOrCreateRegistry(params.KubernetesClient)
	result := services.ExecuteOnClusters(params.Context, registry, input.Target, func(ctx context.Context, client *clients.ClusterClient) (interface{}, error) {
		return services.NewVirtualizationService().GetStatus(ctx, client)
	})
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// InitHCPStatusTool creates the fusion.hcp.status tool
func InitHCPStatusTool() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "fusion.hcp.status",
			Description: "Get Hosted Control Planes (HyperShift) status across clusters",
			Annotations: api.ToolAnnotations{
				Title:        "HCP Status",
				ReadOnlyHint: ptr.To(true),
			},
			InputSchema: &jsonschema.Schema{
				Type:       jsonschema.Type{jsonschema.TypeObject},
				Properties: map[string]*jsonschema.Schema{"target": targeting.TargetSchema()},
			},
		},
		Handler: handleHCPStatus,
	}
}

func handleHCPStatus(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	var input struct{ Target targeting.Target }
	if err := json.Unmarshal(params.Arguments, &input); err != nil {
		input.Target = targeting.Target{Type: targeting.TargetSingle}
	}
	registry := clients.GetOrCreateRegistry(params.KubernetesClient)
	result := services.ExecuteOnClusters(params.Context, registry, input.Target, func(ctx context.Context, client *clients.ClusterClient) (interface{}, error) {
		return services.NewHCPService().GetStatus(ctx, client)
	})
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return api.NewToolCallResult(string(jsonBytes), nil), nil
}

// Made with Bob