package core

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func initWorkloadResources() []api.ServerTool {
	return []api.ServerTool{
		// --- HorizontalPodAutoscaler ---
		{
			Tool: api.Tool{
				Name:        "horizontalpodautoscalers_list",
				Description: "List Kubernetes HorizontalPodAutoscalers (HPAs) in the current cluster from all namespaces or the provided namespace. Shows autoscaling rules and current/target metrics for workloads",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Optional Namespace to retrieve HPAs from. If not provided, lists HPAs from all namespaces",
						},
					},
				},
				Annotations: api.ToolAnnotations{
					Title:           "HorizontalPodAutoscalers: List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: hpasList,
		},
		{
			Tool: api.Tool{
				Name:        "horizontalpodautoscalers_get",
				Description: "Get a Kubernetes HorizontalPodAutoscaler (HPA) by name in the current or provided namespace, showing its scaling rules, targets, and current status",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to get the HPA from (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the HPA",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "HorizontalPodAutoscaler: Get",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: hpaGet,
		},
		{
			Tool: api.Tool{
				Name:        "horizontalpodautoscalers_create",
				Description: "Create a Kubernetes HorizontalPodAutoscaler (HPA) from a YAML or JSON manifest. The manifest must be a complete HPA definition with apiVersion autoscaling/v2",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"manifest": {
							Type:        "string",
							Description: "Complete YAML or JSON manifest of the HorizontalPodAutoscaler (apiVersion: autoscaling/v2, kind: HorizontalPodAutoscaler)",
						},
					},
					Required: []string{"manifest"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "HorizontalPodAutoscaler: Create",
					DestructiveHint: ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: hpaCreate,
		},
		{
			Tool: api.Tool{
				Name:        "horizontalpodautoscalers_delete",
				Description: "Delete a Kubernetes HorizontalPodAutoscaler (HPA) by name in the current or provided namespace, removing its autoscaling rules",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to delete the HPA from (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the HPA to delete",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "HorizontalPodAutoscaler: Delete",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: hpaDelete,
		},
		// --- PodDisruptionBudget ---
		{
			Tool: api.Tool{
				Name:        "poddisruptionbudgets_list",
				Description: "List Kubernetes PodDisruptionBudgets (PDBs) in the current cluster from all namespaces or the provided namespace. Shows the allowed disruptions and current/min available pod counts",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Optional Namespace to retrieve PDBs from. If not provided, lists PDBs from all namespaces",
						},
					},
				},
				Annotations: api.ToolAnnotations{
					Title:           "PodDisruptionBudgets: List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: pdbsList,
		},
		{
			Tool: api.Tool{
				Name:        "poddisruptionbudgets_get",
				Description: "Get a Kubernetes PodDisruptionBudget (PDB) by name in the current or provided namespace, showing its selector, allowed disruptions, and status",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to get the PDB from (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the PDB",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "PodDisruptionBudget: Get",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: pdbGet,
		},
		{
			Tool: api.Tool{
				Name:        "poddisruptionbudgets_create",
				Description: "Create a Kubernetes PodDisruptionBudget (PDB) from a YAML or JSON manifest. The manifest must be a complete PDB definition with apiVersion policy/v1",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"manifest": {
							Type:        "string",
							Description: "Complete YAML or JSON manifest of the PodDisruptionBudget (apiVersion: policy/v1, kind: PodDisruptionBudget)",
						},
					},
					Required: []string{"manifest"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "PodDisruptionBudget: Create",
					DestructiveHint: ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: pdbCreate,
		},
		{
			Tool: api.Tool{
				Name:        "poddisruptionbudgets_delete",
				Description: "Delete a Kubernetes PodDisruptionBudget (PDB) by name in the current or provided namespace",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to delete the PDB from (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the PDB to delete",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "PodDisruptionBudget: Delete",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: pdbDelete,
		},
		// --- ResourceQuota ---
		{
			Tool: api.Tool{
				Name:        "resourcequotas_list",
				Description: "List Kubernetes ResourceQuotas in the current cluster from all namespaces or the provided namespace. Shows namespace-level resource limits and current usage",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Optional Namespace to retrieve ResourceQuotas from. If not provided, lists ResourceQuotas from all namespaces",
						},
					},
				},
				Annotations: api.ToolAnnotations{
					Title:           "ResourceQuotas: List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: resourceQuotasList,
		},
		{
			Tool: api.Tool{
				Name:        "resourcequotas_get",
				Description: "Get a Kubernetes ResourceQuota by name in the current or provided namespace, showing its hard limits and current usage",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to get the ResourceQuota from (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the ResourceQuota",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "ResourceQuota: Get",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: resourceQuotaGet,
		},
		{
			Tool: api.Tool{
				Name:        "resourcequotas_create",
				Description: "Create a Kubernetes ResourceQuota from a YAML or JSON manifest. The manifest must be a complete ResourceQuota definition with apiVersion v1",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"manifest": {
							Type:        "string",
							Description: "Complete YAML or JSON manifest of the ResourceQuota (apiVersion: v1, kind: ResourceQuota)",
						},
					},
					Required: []string{"manifest"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "ResourceQuota: Create",
					DestructiveHint: ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: resourceQuotaCreate,
		},
		{
			Tool: api.Tool{
				Name:        "resourcequotas_delete",
				Description: "Delete a Kubernetes ResourceQuota by name in the current or provided namespace",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to delete the ResourceQuota from (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the ResourceQuota to delete",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "ResourceQuota: Delete",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: resourceQuotaDelete,
		},
		// --- LimitRange ---
		{
			Tool: api.Tool{
				Name:        "limitranges_list",
				Description: "List Kubernetes LimitRanges in the current cluster from all namespaces or the provided namespace. Shows per-container default resource limits enforced in each namespace",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Optional Namespace to retrieve LimitRanges from. If not provided, lists LimitRanges from all namespaces",
						},
					},
				},
				Annotations: api.ToolAnnotations{
					Title:           "LimitRanges: List",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: limitRangesList,
		},
		{
			Tool: api.Tool{
				Name:        "limitranges_get",
				Description: "Get a Kubernetes LimitRange by name in the current or provided namespace, showing its per-container resource default/min/max limits",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to get the LimitRange from (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the LimitRange",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "LimitRange: Get",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: limitRangeGet,
		},
		{
			Tool: api.Tool{
				Name:        "limitranges_create",
				Description: "Create a Kubernetes LimitRange from a YAML or JSON manifest. The manifest must be a complete LimitRange definition with apiVersion v1",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"manifest": {
							Type:        "string",
							Description: "Complete YAML or JSON manifest of the LimitRange (apiVersion: v1, kind: LimitRange)",
						},
					},
					Required: []string{"manifest"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "LimitRange: Create",
					DestructiveHint: ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: limitRangeCreate,
		},
		{
			Tool: api.Tool{
				Name:        "limitranges_delete",
				Description: "Delete a Kubernetes LimitRange by name in the current or provided namespace",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "Namespace to delete the LimitRange from (Optional, current namespace if not provided)",
						},
						"name": {
							Type:        "string",
							Description: "Name of the LimitRange to delete",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "LimitRange: Delete",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: limitRangeDelete,
		},
	}
}

// --- HPA handlers ---

func hpasList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list HPAs: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).HPAsList(params.Context, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list horizontalpodautoscalers: %w", err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func hpaGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get HPA: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).HPAGet(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get horizontalpodautoscaler %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func hpaCreate(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	manifest := p.RequiredString("manifest")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create HPA: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).HPACreate(params.Context, manifest)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create horizontalpodautoscaler: %w", err)), nil
	}
	return api.NewToolCallResult("# The following HorizontalPodAutoscaler (YAML) has been created successfully\n"+ret, nil), nil
}

func hpaDelete(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete HPA: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).HPADelete(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete horizontalpodautoscaler %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

// --- PDB handlers ---

func pdbsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list PDBs: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).PDBsList(params.Context, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list poddisruptionbudgets: %w", err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func pdbGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get PDB: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).PDBGet(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get poddisruptionbudget %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func pdbCreate(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	manifest := p.RequiredString("manifest")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create PDB: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).PDBCreate(params.Context, manifest)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create poddisruptionbudget: %w", err)), nil
	}
	return api.NewToolCallResult("# The following PodDisruptionBudget (YAML) has been created successfully\n"+ret, nil), nil
}

func pdbDelete(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete PDB: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).PDBDelete(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete poddisruptionbudget %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

// --- ResourceQuota handlers ---

func resourceQuotasList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list ResourceQuotas: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).ResourceQuotasList(params.Context, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list resourcequotas: %w", err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func resourceQuotaGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get ResourceQuota: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).ResourceQuotaGet(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get resourcequota %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func resourceQuotaCreate(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	manifest := p.RequiredString("manifest")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create ResourceQuota: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).ResourceQuotaCreate(params.Context, manifest)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create resourcequota: %w", err)), nil
	}
	return api.NewToolCallResult("# The following ResourceQuota (YAML) has been created successfully\n"+ret, nil), nil
}

func resourceQuotaDelete(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete ResourceQuota: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).ResourceQuotaDelete(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete resourcequota %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

// --- LimitRange handlers ---

func limitRangesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list LimitRanges: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).LimitRangesList(params.Context, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list limitranges: %w", err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func limitRangeGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get LimitRange: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).LimitRangeGet(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get limitrange %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func limitRangeCreate(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	manifest := p.RequiredString("manifest")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create LimitRange: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).LimitRangeCreate(params.Context, manifest)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create limitrange: %w", err)), nil
	}
	return api.NewToolCallResult("# The following LimitRange (YAML) has been created successfully\n"+ret, nil), nil
}

func limitRangeDelete(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete LimitRange: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).LimitRangeDelete(params.Context, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to delete limitrange %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}
