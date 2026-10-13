package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	dynamicclient "k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/dynamic"
)

const dynamicTestLabel = "test=dynamic"

type DynamicToolsSuite struct {
	BaseMcpSuite
}

func (s *DynamicToolsSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	dynamic.PollInterval = 100 * time.Millisecond
	s.Cfg.Toolsets.SetForTest(append(s.Cfg.Toolsets.Get(), "dynamic"))
}

func (s *DynamicToolsSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	ctx := s.T().Context()
	_ = dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).
		DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
	_ = kubernetes.NewForConfigOrDie(test.EnvTestRestConfig()).CoreV1().ConfigMaps("default").
		DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: dynamicTestLabel})
}

func (s *DynamicToolsSuite) createTool(manifest string) {
	obj := &unstructured.Unstructured{}
	s.Require().NoError(yaml.Unmarshal([]byte(manifest), &obj.Object))
	_, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).
		Create(s.T().Context(), obj, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create Tool resource")
}

func (s *DynamicToolsSuite) deleteTool(name string) {
	err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).
		Delete(s.T().Context(), name, metav1.DeleteOptions{})
	s.Require().NoError(err, "failed to delete Tool resource")
}

func (s *DynamicToolsSuite) createConfigMap(name string, data map[string]string) {
	_, err := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig()).CoreV1().ConfigMaps("default").
		Create(s.T().Context(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"test": "dynamic"}},
			Data:       data,
		}, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create ConfigMap")
}

func (s *DynamicToolsSuite) findTool(name string) *mcp.Tool {
	tools, err := s.ListTools()
	s.Require().NoError(err, "ListTools failed")
	for _, tool := range tools.Tools {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

func (s *DynamicToolsSuite) text(result *mcp.CallToolResult) string {
	s.Require().NotEmpty(result.Content, "tool result has no content")
	return result.Content[0].(*mcp.TextContent).Text
}

func (s *DynamicToolsSuite) TestKubernetesListTool() {
	s.createConfigMap("dyn-cm-a", map[string]string{"k": "a"})
	s.createConfigMap("dyn-cm-b", map[string]string{"k": "b"})
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: configmaps-names
spec:
  title: "ConfigMaps: Names"
  description: List the names of the ConfigMaps in a namespace, optionally filtered by a label selector
  readOnly: true
  idempotent: true
  inputSchema:
    type: object
    properties:
      namespace:
        type: string
        description: Namespace to list ConfigMaps from
      label_selector:
        type: string
        description: Optional Kubernetes label selector
    required: [namespace]
  kubernetes:
    verb: list
    apiVersion: v1
    kind: ConfigMap
    namespace: {argument: namespace}
    query:
      labelSelector: {argument: label_selector}
  output: response.items.map(i, i.metadata.name)
`)
	s.InitMcpClient()
	tool := s.findTool("configmaps_names")
	s.Run("tools/list exposes the Tool resource as an MCP tool", func() {
		s.Require().NotNil(tool, "expected configmaps_names in tools/list")
		s.Equal("ConfigMaps: Names", tool.Annotations.Title)
		s.True(tool.Annotations.ReadOnlyHint)
		s.False(*tool.Annotations.DestructiveHint)
		schema, _ := json.Marshal(tool.InputSchema)
		s.Contains(string(schema), `"required":["namespace"]`)
		s.Contains(string(schema), `"label_selector"`)
	})
	s.Run("advertises bounded RBAC metadata derived from the declaration", func() {
		s.Require().NotNil(tool)
		meta, _ := json.Marshal(tool.Meta)
		s.Contains(string(meta), `"verbs":["list"]`)
		s.Contains(string(meta), `"resource":"configmaps"`)
		s.Contains(string(meta), `"namespace":{"argument":"namespace"}`)
	})
	s.Run("call projects the response through the CEL output expression", func() {
		result, err := s.CallTool("configmaps_names", map[string]any{"namespace": "default", "label_selector": dynamicTestLabel})
		s.Require().NoError(err, "call tool failed")
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "- dyn-cm-a\n")
		s.Contains(s.text(result), "- dyn-cm-b\n")
		s.NotContains(s.text(result), "resourceVersion", "raw objects should have been projected away")
	})
}

func (s *DynamicToolsSuite) TestKubernetesGetCreateDelete() {
	s.createConfigMap("dyn-cm-get", map[string]string{"color": "blue"})
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: configmap-data
spec:
  description: Get the data of a ConfigMap
  readOnly: true
  inputSchema:
    type: object
    properties:
      namespace: {type: string}
      name: {type: string}
    required: [name]
  kubernetes:
    verb: get
    apiVersion: v1
    kind: ConfigMap
    namespace: {argument: namespace}
    name: {argument: name}
  output: response.data
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: configmap-create
spec:
  description: Create a ConfigMap from a manifest
  inputSchema:
    type: object
    properties:
      manifest: {type: string, description: YAML manifest of the ConfigMap}
    required: [manifest]
  kubernetes:
    verb: create
    apiVersion: v1
    kind: ConfigMap
    namespace: {value: default}
    body: {argument: manifest}
  output: response.metadata.name
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: configmap-delete
spec:
  description: Delete a ConfigMap
  destructive: true
  inputSchema:
    type: object
    properties:
      name: {type: string}
    required: [name]
  kubernetes:
    verb: delete
    apiVersion: v1
    kind: ConfigMap
    namespace: {value: default}
    name: {argument: name}
  output: "'deleted'"
`)
	s.InitMcpClient()
	s.Run("get with an argument-derived name, default namespace from the kubeconfig", func() {
		result, err := s.CallTool("configmap_data", map[string]any{"name": "dyn-cm-get"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Equal("color: blue\n", s.text(result))
		s.Equal(map[string]any{"color": "blue"}, result.StructuredContent, "object results are also returned as structuredContent")
	})
	s.Run("get of a missing resource surfaces the API error", func() {
		result, err := s.CallTool("configmap_data", map[string]any{"name": "does-not-exist"})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), "not found")
	})
	s.Run("create from a YAML manifest argument", func() {
		result, err := s.CallTool("configmap_create", map[string]any{"manifest": "" +
			"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: dyn-cm-created\n  labels:\n    test: dynamic\ndata:\n  a: b\n"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Equal("dyn-cm-created\n", s.text(result))
		cm, err := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig()).CoreV1().ConfigMaps("default").Get(s.T().Context(), "dyn-cm-created", metav1.GetOptions{})
		s.Require().NoError(err)
		s.Equal("b", cm.Data["a"])
	})
	s.Run("delete is advertised as destructive and removes the resource", func() {
		s.True(*s.findTool("configmap_delete").Annotations.DestructiveHint)
		result, err := s.CallTool("configmap_delete", map[string]any{"name": "dyn-cm-created"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		_, err = kubernetes.NewForConfigOrDie(test.EnvTestRestConfig()).CoreV1().ConfigMaps("default").Get(s.T().Context(), "dyn-cm-created", metav1.GetOptions{})
		s.True(apierrors.IsNotFound(err), "expected ConfigMap to be deleted")
	})
}

func (s *DynamicToolsSuite) TestHTTPTool() {
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/fail") {
			http.Error(w, "kaboom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "healthy", "echo": string(body)})
	}))
	defer server.Close()
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: mesh-namespace-health
spec:
  description: Health of a service mesh namespace as reported by the console backend
  readOnly: true
  inputSchema:
    type: object
    properties:
      namespace: {type: string}
    required: [namespace]
  http:
    method: POST
    url: ` + server.URL + `
    path: /api/mcp/namespace_health/{namespace}
    query:
      rateInterval: {value: 10m}
    body: {expression: args}
  output: "{'status': response.status, 'echo': response.echo}"
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: mesh-fail
spec:
  description: Always fails
  readOnly: true
  http:
    url: ` + server.URL + `
    path: /fail
`)
	s.InitMcpClient()
	s.Run("declares no Kubernetes RBAC", func() {
		meta, _ := json.Marshal(s.findTool("mesh_namespace_health").Meta)
		s.Contains(string(meta), `"none":{"reason":"performs an HTTP request to `+server.URL)
	})
	s.Run("expands the path, query and body from the arguments", func() {
		result, err := s.CallTool("mesh_namespace_health", map[string]any{"namespace": "istio-system", "extra": "value"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "status: healthy\n")
		s.Contains(s.text(result), `{"extra":"value","namespace":"istio-system"}`)
		mu.Lock()
		defer mu.Unlock()
		s.Equal([]string{"POST /api/mcp/namespace_health/istio-system?rateInterval=10m"}, requests)
	})
	s.Run("non-2xx responses are reported as tool errors", func() {
		result, err := s.CallTool("mesh_fail", map[string]any{})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), "HTTP 500")
		s.Contains(s.text(result), "kaboom")
	})
}

func (s *DynamicToolsSuite) TestToolsFollowClusterChanges() {
	s.InitMcpClient()
	s.Nil(s.findTool("namespaces_names"), "tool must not exist before the resource is created")
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: namespaces-names
spec:
  description: List the names of all namespaces
  readOnly: true
  kubernetes:
    verb: list
    apiVersion: v1
    kind: Namespace
  output: response.items.map(i, i.metadata.name)
`)
	s.Run("a new Tool resource appears without restarting the server", func() {
		s.Eventually(func() bool { return s.findTool("namespaces_names") != nil }, 10*time.Second, 100*time.Millisecond)
		result, err := s.CallTool("namespaces_names", map[string]any{})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "- default\n", "cluster-scoped resources are listed without a namespace")
	})
	s.Run("a deleted Tool resource disappears", func() {
		s.deleteTool("namespaces-names")
		s.Eventually(func() bool { return s.findTool("namespaces_names") == nil }, 10*time.Second, 100*time.Millisecond)
	})
}

func (s *DynamicToolsSuite) TestInvalidToolIsSkipped() {
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: broken-no-action
spec:
  description: Has no action
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: broken-cel
spec:
  description: Has a CEL syntax error
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
  output: "response.items.map("
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: still-fine
spec:
  description: Valid tool next to broken ones
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
`)
	s.InitMcpClient()
	s.Nil(s.findTool("broken_no_action"))
	s.Nil(s.findTool("broken_cel"))
	s.NotNil(s.findTool("still_fine"))
}

func (s *DynamicToolsSuite) TestReadOnlyModeFiltersMutatingTools() {
	s.Cfg.ReadOnly.SetForTest(true)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: ro-tool
spec:
  description: Read-only
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: rw-tool
spec:
  description: Mutating
  kubernetes: {verb: delete, apiVersion: v1, kind: Namespace, name: {argument: name}}
`)
	s.InitMcpClient()
	s.NotNil(s.findTool("ro_tool"))
	s.Nil(s.findTool("rw_tool"), "read_only must hide dynamic tools that are not read-only")
}

func TestDynamicTools(t *testing.T) {
	suite.Run(t, new(DynamicToolsSuite))
}
