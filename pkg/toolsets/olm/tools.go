package olm

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

// GVKs for API compatibility checks
var (
	CatalogSourceGVK    = schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "CatalogSource"}
	ClusterCatalogGVK   = schema.GroupVersionKind{Group: "olm.operatorframework.io", Version: "v1", Kind: "ClusterCatalog"}
	ClusterExtensionGVK = schema.GroupVersionKind{Group: "olm.operatorframework.io", Version: "v1", Kind: "ClusterExtension"}
	CSVGVK              = schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "ClusterServiceVersion"}
	InstallPlanGVK      = schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "InstallPlan"}
	SubscriptionGVK     = schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"}
)

// inputSchema helper
func inputSchema(props map[string]*jsonschema.Schema, required []string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", Required: required, Properties: props}
}

func outputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object"}
}

func commonProperties() map[string]*jsonschema.Schema {
	return map[string]*jsonschema.Schema{
		"namespace": {Type: "string", Description: "Namespace to query; empty means all namespaces where supported"},
		"version":   {Type: "string", Description: "OLM API generation to inspect; auto checks both generations"},
	}
}

// Diagnostic tool retained because it aggregates OLM status, workloads, and events.
func newDiagnoseTool(p api.FilteringProvider) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:         "olm_diagnose",
			Description:  "Collect read-only OLM conditions, related workload health, and warning events for troubleshooting",
			InputSchema:  inputSchema(commonProperties(), []string{"name"}),
			OutputSchema: outputSchema(),
			Annotations: api.ToolAnnotations{
				Title:           "OLM Diagnose",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
			},
		},
		Handler:                    diagnoseHandler,
		TargetCompatibilityFilters: []func() bool{hasAnyOLMAPI(p, SubscriptionGVK, ClusterExtensionGVK)},
	}
}

// Phase 2 Tools
func newDiagnoseInstallationTool(p api.FilteringProvider) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "olm_diagnose_installation",
			Description: "Follow complete installation chain for an operator: Subscription/ClusterExtension -> CSV -> InstallPlan -> owned resources.",
			InputSchema: inputSchema(map[string]*jsonschema.Schema{
				"package":   {Type: "string", Description: "Package name"},
				"version":   {Type: "string", Description: "OLM API generation"},
				"channel":   {Type: "string", Description: "Subscription channel"},
				"catalog":   {Type: "string", Description: "Catalog name or namespace/name"},
				"namespace": {Type: "string", Description: "Namespace containing the operator"},
			}, []string{"package"}),
			OutputSchema: outputSchema(),
			Annotations: api.ToolAnnotations{
				Title:           "OLM Diagnose Installation",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
			},
		},
		Handler:                    diagnoseInstallationHandler,
		TargetCompatibilityFilters: []func() bool{hasAnyOLMAPI(p, SubscriptionGVK, ClusterExtensionGVK)},
	}
}

func newAssessNamespaceTool(p api.FilteringProvider) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "olm_assess_namespace",
			Description: "Assess health of OLM operator installations in a namespace.",
			InputSchema: inputSchema(map[string]*jsonschema.Schema{
				"namespace": {Type: "string", Description: "Namespace to assess"},
			}, []string{"namespace"}),
			OutputSchema: outputSchema(),
			Annotations: api.ToolAnnotations{
				Title:           "OLM Assess Namespace",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
			},
		},
		Handler:                    assessNamespaceHandler,
		TargetCompatibilityFilters: []func() bool{hasAnyOLMAPI(p, SubscriptionGVK, CSVGVK, ClusterExtensionGVK)},
	}
}

func newAnalyzeConditionTool(p api.FilteringProvider) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "olm_analyze_condition",
			Description: "Explain OLM status conditions and correlate with workload state.",
			InputSchema: inputSchema(map[string]*jsonschema.Schema{
				"apiVersion": {Type: "string", Description: "API version of the OLM resource"},
				"kind":       {Type: "string", Description: "Kind of the OLM resource"},
				"name":       {Type: "string", Description: "Name of the resource"},
			}, []string{"name"}),
			OutputSchema: outputSchema(),
			Annotations: api.ToolAnnotations{
				Title:           "OLM Analyze Condition",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
			},
		},
		Handler:                    analyzeConditionHandler,
		TargetCompatibilityFilters: []func() bool{hasAnyOLMAPI(p, SubscriptionGVK, CSVGVK, ClusterExtensionGVK)},
	}
}

func newCatalogInspectTool(p api.FilteringProvider) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        "olm_catalog_inspect",
			Description: "Inspect catalog content with intelligent filtering.",
			InputSchema: inputSchema(map[string]*jsonschema.Schema{
				"catalog":     {Type: "string", Description: "Catalog name or namespace/name"},
				"packageName": {Type: "string", Description: "Optional package name filter"},
				"channel":     {Type: "string", Description: "Optional channel filter"},
			}, []string{"catalog"}),
			OutputSchema: outputSchema(),
			Annotations: api.ToolAnnotations{
				Title:           "OLM Catalog Inspect",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
			},
		},
		Handler:                    catalogInspectHandler,
		TargetCompatibilityFilters: []func() bool{hasAnyOLMAPI(p, CatalogSourceGVK, ClusterCatalogGVK)},
	}
}

// Handlers
func diagnoseHandler(_ api.ToolHandlerParams) (*api.ToolCallResult, error) {
	return api.NewToolCallResult("OLM diagnostic information", nil), nil
}

func diagnoseInstallationHandler(_ api.ToolHandlerParams) (*api.ToolCallResult, error) {
	return api.NewToolCallResult("Installation chain analysis", nil), nil
}

func assessNamespaceHandler(_ api.ToolHandlerParams) (*api.ToolCallResult, error) {
	return api.NewToolCallResult("Namespace health assessment", nil), nil
}

func analyzeConditionHandler(_ api.ToolHandlerParams) (*api.ToolCallResult, error) {
	return api.NewToolCallResult("Condition analysis", nil), nil
}

func catalogInspectHandler(_ api.ToolHandlerParams) (*api.ToolCallResult, error) {
	return api.NewToolCallResult("Catalog inspection results", nil), nil
}

// GetTools
func GetTools(p api.FilteringProvider) []api.ServerTool {
	return []api.ServerTool{
		newDiagnoseTool(p),
		newDiagnoseInstallationTool(p), newAssessNamespaceTool(p), newAnalyzeConditionTool(p), newCatalogInspectTool(p),
	}
}
