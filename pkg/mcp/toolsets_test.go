package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	configuration "github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kcp"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/logs"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/metrics"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/otelcol"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/tekton"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/traces"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const updateJsonEnvVar = "UPDATE_TOOLSETS_JSON"

const persesListWithInstance = `{
	"apiVersion": "perses.dev/v1alpha2",
	"kind": "PersesList",
	"items": [{
		"apiVersion": "perses.dev/v1alpha2",
		"kind": "Perses",
		"metadata": {"name": "monitoring", "namespace": "observability"}
	}]
}`

type ToolsetsSuite struct {
	suite.Suite
	originalToolsets []api.Toolset
	*test.MockServer
	*test.McpClient
	Cfg        *configuration.Config
	mcpServer  *Server
	updateJson bool
}

func (s *ToolsetsSuite) SetupTest() {
	s.originalToolsets = toolsets.Toolsets()
	s.MockServer = test.NewMockServer()
	// Set up default discovery handler for non-OpenShift cluster
	s.Handle(test.NewDiscoveryClientHandler())
	s.Cfg = configuration.BaseDefault()
	s.Cfg.KubeConfig.SetForTest(s.KubeconfigFile(s.T()))
	test.ApplyEnvtestClientLimits(s.Cfg)
	s.updateJson = os.Getenv(updateJsonEnvVar) != ""
}

func (s *ToolsetsSuite) TearDownTest() {
	toolsets.Clear()
	for _, toolset := range s.originalToolsets {
		toolsets.Register(toolset)
	}
	s.MockServer.Close()
}

func (s *ToolsetsSuite) TearDownSubTest() {
	if s.McpClient != nil {
		s.McpClient.Close()
	}
	if s.mcpServer != nil {
		s.mcpServer.Close()
	}
}

func (s *ToolsetsSuite) TestNoToolsets() {
	s.Run("No toolsets registered", func() {
		toolsets.Clear()
		s.Cfg.Toolsets.SetForTest([]string{})
		s.InitMcpClient()
		tools, err := s.ListTools()
		s.Run("ListTools returns no tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
			s.Empty(tools.Tools, "Expected no tools from ListTools")
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsTools() {
	s.Run("Default configuration toolsets", func() {
		s.InitMcpClient()
		tools, err := s.ListTools()
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("ListTools returns correct Tool metadata", func() {
			s.assertJsonSnapshot("toolsets-full-tools.json", tools.Tools)
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsToolsInOpenShift() {
	s.Run("Default configuration toolsets in OpenShift", func() {
		// Replace default handler with OpenShift handler
		s.ResetHandlers()
		s.Handle(test.NewInOpenShiftHandler())
		s.InitMcpClient()
		tools, err := s.ListTools()
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("ListTools returns correct Tool metadata", func() {
			s.assertJsonSnapshot("toolsets-full-tools-openshift.json", tools.Tools)
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsToolsWithFilteringEnabled() {
	s.Run("Default configuration toolsets with filtering enabled on non-OpenShift", func() {
		s.Cfg.EnableTargetCompatibilityToolFilters.SetForTest(true)
		s.InitMcpClient()
		tools, err := s.ListTools()
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("projects_list tool is not present", func() {
			for _, tool := range tools.Tools {
				s.Require().NotEqual("projects_list", tool.Name, "Expected projects_list to not be present when filtering enabled on non-OpenShift cluster")
			}
		})
		s.Run("metrics tools are not present without metrics API", func() {
			for _, tool := range tools.Tools {
				s.Require().NotEqual("pods_top", tool.Name, "Expected pods_top to not be present when filtering enabled without metrics API")
				s.Require().NotEqual("nodes_top", tool.Name, "Expected nodes_top to not be present when filtering enabled without metrics API")
			}
		})
	})
}

func (s *ToolsetsSuite) TestMetricsToolsPresentWithFilteringEnabled() {
	s.Run("pods_top and nodes_top are present when metrics API is available", func() {
		s.ResetHandlers()
		s.Handle(test.NewDiscoveryClientHandler(metav1.APIResourceList{
			GroupVersion: "metrics.k8s.io/v1beta1",
			APIResources: []metav1.APIResource{
				{Name: "nodes", Kind: "NodeMetrics", Namespaced: false, Verbs: metav1.Verbs{"get", "list"}},
				{Name: "pods", Kind: "PodMetrics", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}},
			},
		}))
		s.Cfg.EnableTargetCompatibilityToolFilters.SetForTest(true)
		s.InitMcpClient()
		tools, err := s.ListTools()
		s.Require().NoError(err, "Expected no error from ListTools")
		s.Require().NotNil(tools, "Expected tools from ListTools")

		found := map[string]bool{}
		for _, tool := range tools.Tools {
			if tool.Name == "pods_top" || tool.Name == "nodes_top" {
				found[tool.Name] = true
			}
		}
		s.True(found["pods_top"], "Expected pods_top to be present when metrics API is available")
		s.True(found["nodes_top"], "Expected nodes_top to be present when metrics API is available")
	})
}

func (s *ToolsetsSuite) TestKubevirtToolsFilteredWithoutCRDs() {
	s.Run("Kubevirt tools are filtered out when VirtualMachine GVK is not present", func() {
		s.Cfg.Toolsets.SetForTest([]string{"kubevirt"})
		s.Cfg.EnableTargetCompatibilityToolFilters.SetForTest(true)
		s.InitMcpClient()
		tools, err := s.ListTools()
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("kubevirt tools are not present", func() {
			kubevirtTools := []string{"vm_create", "vm_create_from_template", "vm_lifecycle", "vm_clone", "vm_guest_info"}
			for _, tool := range tools.Tools {
				for _, kvTool := range kubevirtTools {
					s.Require().NotEqual(kvTool, tool.Name, "Expected %s to not be present when filtering enabled on cluster without KubeVirt", kvTool)
				}
			}
		})
	})
}

func (s *ToolsetsSuite) TestKialiToolsFilteredWhenURLUnreachable() {
	s.Run("Kiali tools are filtered out when configured URL fails /api/status probe", func() {
		toolsetName := (&kiali.Toolset{}).GetName()
		kubeConfig := s.Cfg.KubeConfig.Get()
		cfg, err := configuration.ReadToml(s.T().Context(), fmt.Appendf(nil, `
			toolsets = ["%s"]
			experimental_enable_target_compatibility_tool_filters = true
			[toolset_configs.kiali]
			url = "http://127.0.0.1:1"
			insecure = true
		`, toolsetName))
		s.Require().NoError(err)
		s.Cfg = cfg
		s.Cfg.KubeConfig.SetForTest(kubeConfig)
		s.InitMcpClient()
		tools, err := s.ListTools()
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("kiali tools are not present", func() {
			for _, tool := range tools.Tools {
				for _, kialiTool := range s.kialiToolNames(toolsetName) {
					s.Require().NotEqual(kialiTool, tool.Name, "Expected %s to not be present when Kiali URL is unreachable", kialiTool)
				}
			}
		})
	})
}

func (s *ToolsetsSuite) kialiToolNames(toolsetName string) []string {
	return []string{
		toolsetName + "_get_logs",
		toolsetName + "_get_mesh_status",
		toolsetName + "_get_mesh_traffic_graph",
		toolsetName + "_get_metrics",
		toolsetName + "_get_pod_performance",
		toolsetName + "_get_resource_details",
		toolsetName + "_get_trace_details",
		toolsetName + "_list_mesh_clusters",
		toolsetName + "_list_traces",
		toolsetName + "_manage_istio_config",
		toolsetName + "_manage_istio_config_read",
	}
}

func (s *ToolsetsSuite) TestDefaultToolsetsToolsInMultiCluster() {
	s.Run("Default configuration toolsets in multi-cluster (with 11 clusters)", func() {
		kubeconfig := s.Kubeconfig()
		for i := 0; i < 10; i++ {
			// Add multiple fake contexts to force multi-cluster behavior
			kubeconfig.Contexts[strconv.Itoa(i)] = clientcmdapi.NewContext()
		}
		s.Cfg.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), kubeconfig))
		s.InitMcpClient()
		tools, err := s.ListTools()
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("ListTools returns correct Tool metadata", func() {
			s.assertJsonSnapshot("toolsets-full-tools-multicluster.json", tools.Tools)
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsPrompts() {
	s.Run("Default configuration toolsets", func() {
		s.InitMcpClient()
		prompts, err := s.ListPrompts()
		s.Run("ListPrompts returns prompts", func() {
			s.NotNil(prompts, "Expected prompts from ListPrompts")
			s.NoError(err, "Expected no error from ListPrompts")
			s.NotEmpty(prompts.Prompts, "Expected at least one prompt")
		})
		s.Run("ListPrompts returns correct Prompt metadata", func() {
			s.assertJsonSnapshot("toolsets-full-prompts.json", prompts.Prompts)
		})
		s.Run("cluster-aware prompts do not have context argument in single cluster", func() {
			s.Require().NotNil(prompts)
			for _, prompt := range prompts.Prompts {
				for _, arg := range prompt.Arguments {
					s.NotEqualf("context", arg.Name,
						"Prompt %s should not have context argument in single-cluster mode", prompt.Name)
				}
			}
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsPromptsInMultiCluster() {
	s.Run("Default configuration toolsets in multi-cluster (with 11 clusters)", func() {
		kubeconfig := s.Kubeconfig()
		for i := 0; i < 10; i++ {
			// Add multiple fake contexts to force multi-cluster behavior
			kubeconfig.Contexts[strconv.Itoa(i)] = clientcmdapi.NewContext()
		}
		s.Cfg.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), kubeconfig))
		s.InitMcpClient()
		prompts, err := s.ListPrompts()
		s.Run("ListPrompts returns prompts", func() {
			s.NotNil(prompts, "Expected prompts from ListPrompts")
			s.NoError(err, "Expected no error from ListPrompts")
			s.NotEmpty(prompts.Prompts, "Expected at least one prompt")
		})
		s.Run("ListPrompts returns correct Prompt metadata", func() {
			s.assertJsonSnapshot("toolsets-full-prompts-multicluster.json", prompts.Prompts)
		})
	})
}

func (s *ToolsetsSuite) TestShowTimeseriesRequiresPersesInstance() {
	testCases := []struct {
		name    string
		status  int
		body    string
		present bool
	}{
		{
			name:    "Perses instance exists",
			status:  http.StatusOK,
			body:    persesListWithInstance,
			present: true,
		},
		{
			name:   "Perses CRD exists without instances",
			status: http.StatusOK,
			body:   `{"apiVersion":"perses.dev/v1alpha2","kind":"PersesList","items":[]}`,
		},
		{
			name:   "Perses API is missing",
			status: http.StatusNotFound,
			body:   `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"NotFound","code":404}`,
		},
		{
			name:   "Perses list permission denied",
			status: http.StatusForbidden,
			body:   `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403}`,
		},
	}
	for _, filtering := range []bool{false, true} {
		for _, tc := range testCases {
			s.Run(tc.name+"/filtering="+strconv.FormatBool(filtering), func() {
				s.ResetHandlers()
				var resources []metav1.APIResourceList
				if tc.status != http.StatusNotFound {
					resources = append(resources, metav1.APIResourceList{
						GroupVersion: "perses.dev/v1alpha2",
						APIResources: []metav1.APIResource{
							{Name: "perses", Kind: "Perses", Namespaced: true, Verbs: metav1.Verbs{"list"}},
						},
					})
				}
				s.Handle(test.NewDiscoveryClientHandler(resources...))
				s.handlePersesList(tc.status, tc.body)
				s.Cfg.Toolsets.SetForTest([]string{(&metrics.Toolset{}).GetName()})
				s.Cfg.EnableTargetCompatibilityToolFilters.SetForTest(filtering)
				s.InitMcpClient()

				tools, err := s.ListTools()
				s.Require().NoError(err)
				s.Require().NotNil(tools)
				names := make([]string, 0, len(tools.Tools))
				found := false
				for _, tool := range tools.Tools {
					names = append(names, tool.Name)
					if tool.Name == "show_timeseries" {
						found = true
					}
				}
				s.Equal(tc.present, found, "show_timeseries must require a Perses instance")
				s.Contains(names, "execute_range_query", "Other metrics tools must remain available")
			})
		}
	}
}

func (s *ToolsetsSuite) handlePersesList(status int, body string) {
	s.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/apis/perses.dev/v1alpha2/perses" {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func (s *ToolsetsSuite) TestGranularToolsetsTools() {
	s.ResetHandlers()
	s.Handle(test.NewDiscoveryClientHandler(metav1.APIResourceList{
		GroupVersion: "perses.dev/v1alpha2",
		APIResources: []metav1.APIResource{
			{Name: "perses", Kind: "Perses", Namespaced: true, Verbs: metav1.Verbs{"list"}},
		},
	}))
	s.handlePersesList(http.StatusOK, persesListWithInstance)
	testCases := []api.Toolset{
		&core.Toolset{},
		&config.Toolset{},
		&helm.Toolset{},
		&kiali.Toolset{},
		&kubevirt.Toolset{},
		&logs.Toolset{},
		&metrics.Toolset{},
		&otelcol.Toolset{},
		&tekton.Toolset{},
		&traces.Toolset{},
	}
	for _, testCase := range testCases {
		s.Run("Toolset "+testCase.GetName(), func() {
			toolsets.Clear()
			toolsets.Register(testCase)
			s.Cfg.Toolsets.SetForTest([]string{testCase.GetName()})
			s.InitMcpClient()
			tools, err := s.ListTools()
			s.Run("ListTools returns tools", func() {
				s.NotNil(tools, "Expected tools from ListTools")
				s.NoError(err, "Expected no error from ListTools")
			})
			s.Run("ListTools returns correct Tool metadata", func() {
				s.assertJsonSnapshot("toolsets-"+strings.ReplaceAll(testCase.GetName(), "/", "-")+"-tools.json", tools.Tools)
			})
		})
	}
}

// TestAllCapabilitiesDeclareRBACMetadata fails when a registered toolset ships a tool, prompt,
// resource or resource template without RBAC metadata, or with RBAC metadata that doesn't validate.
// https://github.com/containers/kubernetes-mcp-server/issues/1435
func (s *ToolsetsSuite) TestAllCapabilitiesDeclareRBACMetadata() {
	// Keep target compatibility filters off so toolsets that filter on cluster state
	// (e.g. kiali) return every capability they can provide.
	s.Cfg.EnableTargetCompatibilityToolFilters.SetForTest(false)
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err, "Expected no error creating kubernetes target provider")
	s.T().Cleanup(provider.Close)
	toolsetContext := api.ToolsetContext{
		Inspector: kubernetes.NewClusterInspector(provider),
		Config:    s.Cfg,
	}
	for _, toolset := range s.originalToolsets {
		s.Run("Toolset "+toolset.GetName(), func() {
			// Validate also rejects nil, so a capability that forgot RBAC fails here too.
			for _, tool := range toolset.GetTools(s.T().Context(), toolsetContext) {
				s.NoErrorf(tool.RBAC.Validate(), "tool %s: missing or invalid RBAC metadata", tool.Tool.Name)
			}
			for _, prompt := range toolset.GetPrompts(s.T().Context(), toolsetContext) {
				s.NoErrorf(prompt.RBAC.Validate(), "prompt %s: missing or invalid RBAC metadata", prompt.Prompt.Name)
			}
			for _, resource := range toolset.GetResources(s.T().Context(), toolsetContext) {
				s.NoErrorf(resource.RBAC.Validate(), "resource %s: missing or invalid RBAC metadata", resource.Resource.URI)
			}
			for _, template := range toolset.GetResourceTemplates(s.T().Context(), toolsetContext) {
				s.NoErrorf(template.RBAC.Validate(), "resource template %s: missing or invalid RBAC metadata", template.ResourceTemplate.URITemplate)
			}
		})
	}
}

func (s *ToolsetsSuite) TestInputSchemaEdgeCases() {
	// https://github.com/containers/kubernetes-mcp-server/issues/340
	// A no-argument tool must still emit properties so OpenAI accepts the schema.
	// configuration_contexts_list stays argument-free; a single cluster hides it.
	s.Run("InputSchema for no-arg tool is object with empty properties", func() {
		kubeconfig := s.Kubeconfig()
		kubeconfig.Contexts["extra-cluster"] = clientcmdapi.NewContext()
		s.Cfg.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), kubeconfig))
		s.InitMcpClient()
		tools, err := s.ListTools()
		s.Require().NoError(err, "Expected no error from ListTools")
		s.Require().NotNil(tools, "Expected tools from ListTools")
		var contextsList *mcp.Tool
		for _, t := range tools.Tools {
			if t.Name == "configuration_contexts_list" {
				contextsList = t
				break
			}
		}
		s.Require().NotNil(contextsList, "Expected configuration_contexts_list from ListTools")
		schema, ok := contextsList.InputSchema.(map[string]any)
		s.Require().True(ok, "Expected InputSchema to be map[string]any")
		s.NotNil(schema["properties"], "Expected configuration_contexts_list.InputSchema.properties not to be nil")
		properties, ok := schema["properties"].(map[string]any)
		s.Require().True(ok, "Expected properties to be map[string]any")
		s.Empty(properties, "Expected configuration_contexts_list.InputSchema.properties to be empty")
	})
	// https://github.com/containers/kubernetes-mcp-server/issues/717
	// Verifies ALL tools have Properties initialized (not just cluster-aware ones)
	// OpenAI API requires properties field even if empty
	s.Run("InputSchema has properties for all tools regardless of cluster-awareness", func() {
		// Register all toolsets including kcp to catch kcp_workspaces_list
		toolsets.Clear()
		toolsets.Register(&core.Toolset{})
		toolsets.Register(&config.Toolset{})
		toolsets.Register(&helm.Toolset{})
		toolsets.Register(&kcp.Toolset{})
		s.Cfg.Toolsets.SetForTest([]string{"core", "config", "helm", "kcp"})
		// Enable multi-cluster mode to include configuration_contexts_list tool
		kubeconfig := s.Kubeconfig()
		kubeconfig.Contexts["extra-cluster"] = clientcmdapi.NewContext()
		s.Cfg.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), kubeconfig))
		s.InitMcpClient()
		tools, err := s.ListTools()
		s.Require().NoError(err, "Expected no error from ListTools")
		s.Require().NotNil(tools, "Expected tools from ListTools")
		s.Require().NotEmpty(tools.Tools, "Expected at least one tool")
		// Check each tool has InputSchema.properties initialized
		for _, tool := range tools.Tools {
			s.Run(tool.Name, func() {
				schema, ok := tool.InputSchema.(map[string]any)
				s.Require().True(ok, "Expected InputSchema to be map[string]any for tool %s", tool.Name)
				s.Require().NotNil(schema["properties"],
					"Expected InputSchema.properties not to be nil for tool %s (required by OpenAI API)",
					tool.Name)
			})
		}
	})
}

type fakeProvider struct{}

func (f *fakeProvider) IsMultiTarget() bool { return false }

func (f *fakeProvider) GetTargets(context.Context) ([]string, error) { return []string{""}, nil }

func (f *fakeProvider) GetDefaultTarget() string { return "" }

func (f *fakeProvider) GetTargetParameterName() string { return "" }

func (f *fakeProvider) Discovery() api.AggregateDiscovery       { return f }
func (f *fakeProvider) Unstructured() api.AggregateUnstructured { return f }
func (f *fakeProvider) BearerToken(ctx context.Context) api.Results[string] {
	return api.NewResults(ctx, f, func(context.Context, string) (string, error) { return "", nil })
}
func (f *fakeProvider) ServerResourcesForGroupVersion(ctx context.Context, _ string) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, f, func(context.Context, string) (*metav1.APIResourceList, error) {
		return &metav1.APIResourceList{APIResources: []metav1.APIResource{
			{Kind: "Project"}, {Kind: "Route"}, {Kind: "NodeMetrics"}, {Kind: "PodMetrics"},
			{Kind: "VirtualMachine"}, {Kind: "VirtualMachineTemplate"},
		}}, nil
	})
}

func (f *fakeProvider) Resource(resource schema.GroupVersionResource) api.AggregateNamespaceableResourceInterface {
	return &fakeInspectionResource{fakeProvider: f, resource: resource}
}

type fakeInspectionResource struct {
	*fakeProvider
	resource schema.GroupVersionResource
}

func (r *fakeInspectionResource) Namespace(_ string) api.AggregateResourceInterface {
	return r
}

func (r *fakeInspectionResource) Get(ctx context.Context, name string, _ metav1.GetOptions, _ ...string) api.Results[*unstructured.Unstructured] {
	return api.NewResults(ctx, r, func(context.Context, string) (*unstructured.Unstructured, error) {
		return nil, apierrors.NewNotFound(r.resource.GroupResource(), name)
	})
}

func (r *fakeInspectionResource) List(ctx context.Context, _ metav1.ListOptions) api.Results[*unstructured.UnstructuredList] {
	return api.NewResults(ctx, r, func(context.Context, string) (*unstructured.UnstructuredList, error) {
		return &unstructured.UnstructuredList{
			Items: []unstructured.Unstructured{{}},
		}, nil
	})
}

func (s *ToolsetsSuite) TestUniqueToolNames() {
	toolNames := make(map[string]bool)
	for _, toolset := range s.originalToolsets {
		for _, tool := range toolset.GetTools(s.T().Context(), api.ToolsetContext{Inspector: &fakeProvider{}}) {
			s.Falsef(toolNames[tool.Tool.Name], "duplicate tool name: %s", tool.Tool.Name)
			toolNames[tool.Tool.Name] = true
		}
	}
}

func (s *ToolsetsSuite) TestUniquePromptNames() {
	promptNames := make(map[string]bool)
	for _, toolset := range s.originalToolsets {
		for _, prompt := range toolset.GetPrompts(s.T().Context(), api.ToolsetContext{Inspector: &fakeProvider{}}) {
			s.Falsef(promptNames[prompt.Prompt.Name], "duplicate prompt name: %s", prompt.Prompt.Name)
			promptNames[prompt.Prompt.Name] = true
		}
	}
}

func (s *ToolsetsSuite) TestPromptDefinitionsAreValid() {
	for _, toolset := range s.originalToolsets {
		for _, prompt := range toolset.GetPrompts(s.T().Context(), api.ToolsetContext{Inspector: &fakeProvider{}}) {
			s.Run(toolset.GetName()+"/"+prompt.Prompt.Name, func() {
				s.NotEmpty(prompt.Prompt.Name, "prompt name must not be empty")
				s.NotEmpty(prompt.Prompt.Description, "prompt %s must have a description", prompt.Prompt.Name)
				s.NotNil(prompt.Handler, "prompt %s must have a handler", prompt.Prompt.Name)
				argNames := make(map[string]bool)
				for _, arg := range prompt.Prompt.Arguments {
					s.NotEmpty(arg.Name, "prompt %s has an argument with an empty name", prompt.Prompt.Name)
					s.Falsef(argNames[arg.Name], "prompt %s has duplicate argument: %s", prompt.Prompt.Name, arg.Name)
					argNames[arg.Name] = true
				}
				if prompt.RBAC != nil {
					s.NoError(prompt.RBAC.Validate(), "prompt %s has invalid RBAC metadata", prompt.Prompt.Name)
				}
			})
		}
	}
}

func (s *ToolsetsSuite) TestUniqueResourceURIs() {
	resourceURIs := make(map[string]bool)
	for _, toolset := range s.originalToolsets {
		for _, resource := range toolset.GetResources(s.T().Context(), api.ToolsetContext{Inspector: &fakeProvider{}}) {
			s.Falsef(resourceURIs[resource.Resource.URI], "duplicate resource URI: %s", resource.Resource.URI)
			resourceURIs[resource.Resource.URI] = true
		}
	}
}

func (s *ToolsetsSuite) TestUniqueResourceTemplateURITemplates() {
	uriTemplates := make(map[string]bool)
	for _, toolset := range s.originalToolsets {
		for _, template := range toolset.GetResourceTemplates(s.T().Context(), api.ToolsetContext{Inspector: &fakeProvider{}}) {
			s.Falsef(uriTemplates[template.ResourceTemplate.URITemplate], "duplicate resource template URI template: %s", template.ResourceTemplate.URITemplate)
			uriTemplates[template.ResourceTemplate.URITemplate] = true
		}
	}
}

func (s *ToolsetsSuite) InitMcpClient() {
	provider, err := kubernetes.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err, "Expected no error creating kubernetes target provider")
	s.mcpServer, err = NewServer(s.T().Context(), Configuration{Config: s.Cfg}, provider)
	s.Require().NoError(err, "Expected no error creating MCP server")
	s.McpClient = test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP())
}

// assertJsonSnapshot compares actual data against a JSON snapshot file.
// When the snapshot doesn't match, the test fails with instructions on how to update it.
// Set UPDATE_TOOLSETS_JSON=1 environment variable to regenerate snapshot files.
// Example: UPDATE_TOOLSETS_JSON=1 go test ./pkg/mcp -v
func (s *ToolsetsSuite) assertJsonSnapshot(snapshotFile string, actual any) {
	_, file, _, _ := runtime.Caller(1)
	snapshotPath := filepath.Join(filepath.Dir(file), "testdata", snapshotFile)
	actualJson, err := json.MarshalIndent(actual, "", "  ")
	s.Require().NoErrorf(err, "failed to marshal actual data: %v", err)
	if s.updateJson {
		err := os.WriteFile(snapshotPath, append(actualJson, '\n'), 0644)
		s.Require().NoErrorf(err, "failed to write snapshot file %s: %v", snapshotFile, err)
		s.T().Logf("Updated snapshot: %s", snapshotFile)
		return
	}
	expectedJson := test.ReadFile("testdata", snapshotFile)
	s.JSONEq(
		expectedJson,
		string(actualJson),
		"snapshot %s does not match - to update snapshots re-run the tests with %s=1",
		snapshotFile,
		updateJsonEnvVar,
	)
}

func TestToolsets(t *testing.T) {
	suite.Run(t, new(ToolsetsSuite))
}
