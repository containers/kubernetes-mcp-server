package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	kc := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())
	_ = kc.CoreV1().ConfigMaps("default").DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: dynamicTestLabel})
	_ = kc.AppsV1().Deployments("default").DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: dynamicTestLabel})
	_ = kc.CoreV1().Events("default").Delete(ctx, "dyn-core-event", metav1.DeleteOptions{})
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
		s.Equal("dyn-cm-created", s.text(result), "string results are returned verbatim")
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
		s.Contains(string(meta), `"none":{"reason":"performs HTTP requests to `+server.URL)
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
	// Passes the CRD schema (the shipped CRD does not cross-check body sources)
	// but fails the toolset's own validation.
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: broken-body
spec:
  description: Has a body with two sources
  http:
    url: https://example.invalid
    body: {argument: a, expression: args}
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
	s.Nil(s.findTool("broken_body"))
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

func (s *DynamicToolsSuite) TestSubresourceAndExpressions() {
	s.createDeployment("dyn-deploy", 1)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: deployment-scale
spec:
  description: Scale a Deployment through the scale subresource
  destructive: true
  inputSchema:
    type: object
    properties:
      namespace: {type: string}
      name: {type: string}
      replicas: {type: integer}
    required: [name, replicas]
  kubernetes:
    verb: patch
    apiVersion: apps/v1
    kind: Deployment
    subresource: scale
    namespace: {argument: namespace}
    name: {argument: name}
    body: {expression: "{'spec': {'replicas': int(args.replicas)}}"}
  output: "{'name': response.metadata.name, 'replicas': response.spec.replicas}"
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: deployment-subresource
spec:
  description: Read a Deployment subresource chosen by argument
  readOnly: true
  kubernetes:
    verb: get
    apiVersion: apps/v1
    resource: deployments
    namespace: {value: default}
    name: {argument: name}
    subresource: {expression: "{'scale': 'scale', 'status': 'status'}[args.what]"}
  output: response.kind
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: namespaces-by-expression
spec:
  description: List namespaces with a query built by CEL
  readOnly: true
  kubernetes:
    verb: list
    apiVersion: v1
    resource: namespaces
    namespaced: false
    queryExpression: "{'fieldSelector': 'metadata.name=' + args.name, 'limit': 1}"
  output: response.items.map(n, n.metadata.name)
`)
	s.InitMcpClient()
	s.Run("patch on a subresource with a CEL body", func() {
		result, err := s.CallTool("deployment_scale", map[string]any{"name": "dyn-deploy", "replicas": 3})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Contains(s.text(result), "replicas: 3\n")
	})
	s.Run("an argument-derived subresource is unbounded in the RBAC metadata", func() {
		meta, _ := json.Marshal(s.findTool("deployment_subresource").Meta)
		s.Contains(string(meta), `"unbounded"`)
		result, err := s.CallTool("deployment_subresource", map[string]any{"name": "dyn-deploy", "what": "scale"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Equal("Scale", s.text(result))
	})
	s.Run("a fixed multi-segment subresource advertises its first segment", func() {
		s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: node-stats
spec:
  description: Kubelet stats summary
  readOnly: true
  kubernetes: {verb: get, apiVersion: v1, kind: Node, name: {argument: name}, subresource: proxy/stats/summary}
`)
		s.Eventually(func() bool { return s.findTool("node_stats") != nil }, 10*time.Second, 100*time.Millisecond)
		meta, _ := json.Marshal(s.findTool("node_stats").Meta)
		s.Contains(string(meta), `"subresource":"proxy"`)
	})
	s.Run("explicit resource and a CEL query expression", func() {
		result, err := s.CallTool("namespaces_by_expression", map[string]any{"name": "default"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Equal("- default\n", s.text(result))
	})
}

func (s *DynamicToolsSuite) TestHTTPQueryExpression() {
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.RawQuery+" accept="+r.Header.Get("Accept"))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b\n1,2\n"))
	}))
	defer server.Close()
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: flows-export
spec:
  description: Export flows as CSV with a computed time window
  readOnly: true
  http:
    url: ` + server.URL + `
    path: /api/loki/export
    headers: {Accept: "text/csv,*/*"}
    queryExpression: "merge(args.transformMapEntry(k, v, k == 'timeRange' ? {} : {k: v}), {'startTime': int(now) - (has(args.timeRange) ? int(args.timeRange) : 300), 'endTime': int(now), 'format': has(args.format) ? args.format : 'csv'})"
`)
	s.InitMcpClient()
	result, err := s.CallTool("flows_export", map[string]any{"timeRange": 60, "filters": "x"})
	s.Require().NoError(err)
	s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
	s.Equal("a,b\n1,2\n", s.text(result), "non-JSON responses are returned verbatim")
	mu.Lock()
	defer mu.Unlock()
	s.Require().Len(requests, 1)
	s.Contains(requests[0], "filters=x")
	s.Contains(requests[0], "format=csv")
	s.NotContains(requests[0], "timeRange")
	s.Regexp(`startTime=\d{10}`, requests[0])
	s.Regexp(`endTime=\d{10}`, requests[0])
	s.Contains(requests[0], "accept=text/csv,*/*")
}

// TestConvertedCoreTools loads the Tool manifests the exporter generated from
// the core toolset and exercises them against envtest with only the dynamic
// toolset enabled.
func (s *DynamicToolsSuite) TestConvertedCoreTools() {
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "dynamic-tools", "converted", "core.yaml"))
	s.Require().NoError(err)
	for _, doc := range strings.Split(string(raw), "\n---\n") {
		if strings.TrimSpace(strings.TrimPrefix(doc, "---")) == "" || !strings.Contains(doc, "kind: Tool") {
			continue
		}
		s.createTool(doc)
	}
	s.createConfigMap("dyn-cm-core", nil)
	_, err = kubernetes.NewForConfigOrDie(test.EnvTestRestConfig()).CoreV1().Events("default").Create(s.T().Context(), &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "dyn-core-event"},
		InvolvedObject: corev1.ObjectReference{APIVersion: "v1", Kind: "ConfigMap", Name: "dyn-cm-core", Namespace: "default"},
		Type:           "Warning", Reason: "DynamicTest", Message: "  hello  ",
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	s.InitMcpClient()
	s.Run("only the converted tools are exposed", func() {
		tools, err := s.ListTools()
		s.Require().NoError(err)
		names := make([]string, 0, len(tools.Tools))
		for _, tool := range tools.Tools {
			names = append(names, tool.Name)
		}
		s.Contains(names, "events_list")
		s.Contains(names, "pods_list")
		s.NotContains(names, "resources_list")
		s.NotContains(names, "pods_exec")
	})
	s.Run("namespaces_list", func() {
		result, err := s.CallTool("namespaces_list", map[string]any{})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Contains(s.text(result), "name: default\n")
		s.Contains(s.text(result), "status: Active\n")
	})
	s.Run("events_list projects events like the built-in tool", func() {
		result, err := s.CallTool("events_list", map[string]any{"namespace": "default", "fieldSelector": "reason=DynamicTest"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Contains(s.text(result), "Reason: DynamicTest\n")
		s.Contains(s.text(result), "Message: hello\n")
		s.Contains(s.text(result), "Kind: ConfigMap\n")
		s.Contains(s.text(result), "Type: Warning\n")
	})
	s.Run("pods_list and pods_get", func() {
		result, err := s.CallTool("pods_list", map[string]any{})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		result, err = s.CallTool("pods_get", map[string]any{"namespace": "default", "name": "nope"})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), `pods "nope" not found`)
	})
}

func (s *DynamicToolsSuite) createDeployment(name string, replicas int32) {
	_, err := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig()).AppsV1().Deployments("default").Create(s.T().Context(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"test": "dynamic"}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
			},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create Deployment")
}

// loadToolManifests creates every Tool document of a generated file. Chart
// templates without Helm values (tekton, kubevirt) are plain YAML.
func (s *DynamicToolsSuite) loadToolManifests(path string) {
	raw, err := os.ReadFile(path)
	s.Require().NoError(err)
	for _, doc := range strings.Split(string(raw), "\n---") {
		if strings.Contains(doc, "kind: Tool") {
			s.createTool(doc)
		}
	}
}

func (s *DynamicToolsSuite) createUnstructured(gvr schema.GroupVersionResource, manifest string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	s.Require().NoError(yaml.Unmarshal([]byte(manifest), &obj.Object))
	created, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(gvr).Namespace("default").Create(s.T().Context(), obj, metav1.CreateOptions{})
	s.Require().NoError(err)
	return created
}

func (s *DynamicToolsSuite) TestStepsAgainstTheAPI() {
	s.createConfigMap("dyn-step-a", map[string]string{"k": "a"})
	s.createConfigMap("dyn-step-b", map[string]string{"k": "b"})
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: configmap-values
spec:
  description: List ConfigMaps then fetch each one (fan-out) and read a key
  readOnly: true
  steps:
    - name: list
      kubernetes: {verb: list, apiVersion: v1, kind: ConfigMap, namespace: {value: default}, query: {labelSelector: {value: "test=dynamic"}}}
    - name: names
      expression: steps.list.items.map(i, i.metadata.name) + ['does-not-exist']
    - name: each
      forEach: steps.names
      ignoreError: true
      kubernetes: {verb: get, apiVersion: v1, kind: ConfigMap, namespace: {value: default}, name: {expression: item}}
    - name: skipped
      when: size(steps.names) > 100
      kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
  output: "{'values': steps.each.map(c, has(c.error) ? 'ERR' : c.data.k), 'skipped': steps.skipped}"
`)
	s.InitMcpClient()
	s.Run("advertises the union of every step's RBAC", func() {
		meta, _ := json.Marshal(s.findTool("configmap_values").Meta)
		s.Contains(string(meta), `"bounded"`)
		s.Contains(string(meta), `"verbs":["list"]`)
		s.Contains(string(meta), `{"namespace":{"name":"default"},"target":{"resource":{"resource":"configmaps"}},"verbs":["get"]}`, "a CEL-derived name advertises the verb and resource for any name")
		s.Contains(string(meta), `"resource":"namespaces"`)
	})
	s.Run("runs the steps in order with fan-out, ignoreError and when", func() {
		result, err := s.CallTool("configmap_values", map[string]any{})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Equal(map[string]any{"values": []any{"a", "b", "ERR"}, "skipped": nil}, result.StructuredContent)
	})
}

func (s *DynamicToolsSuite) TestConvertedTektonTools() {
	ctx := s.T().Context()
	for _, resource := range []string{"pipelineruns", "taskruns"} {
		s.Require().NoError(EnvTestEnableCRD(ctx, "tekton.dev", "v1", resource))
	}
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	s.loadToolManifests(filepath.Join("..", "..", "examples", "dynamic-tools", "charts", "tekton-mcp-tools", "templates", "tools.yaml"))
	pipelineRuns := schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}
	taskRuns := schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "taskruns"}
	client := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig())
	defer func() {
		_ = client.Resource(pipelineRuns).Namespace("default").DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
		_ = client.Resource(taskRuns).Namespace("default").DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
	}()
	s.createUnstructured(pipelineRuns, `
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: build-1
  generateName: build-
spec:
  pipelineRef: {name: build}
  params: [{name: tag, value: v1}]
  status: Cancelled
`)
	s.createUnstructured(taskRuns, `
apiVersion: tekton.dev/v1
kind: TaskRun
metadata:
  name: build-1-compile
  labels: {tekton.dev/pipelineRun: build-1, tekton.dev/pipelineTask: compile}
spec:
  taskRef: {name: compile}
status: {}
`)
	s.InitMcpClient()
	s.Run("pipelinerun restart copies the spec minus status into a generated run", func() {
		result, err := s.CallTool("tekton_pipelinerun_lifecycle", map[string]any{"name": "build-1", "namespace": "default", "action": "restart"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Regexp(`^PipelineRun 'build-1' restarted as 'build-[a-z0-9]+' in namespace 'default'$`, s.text(result))
		list, err := client.Resource(pipelineRuns).Namespace("default").List(ctx, metav1.ListOptions{})
		s.Require().NoError(err)
		s.Len(list.Items, 2)
		for _, item := range list.Items {
			if item.GetName() == "build-1" {
				continue
			}
			spec := item.Object["spec"].(map[string]any)
			s.Equal("build", spec["pipelineRef"].(map[string]any)["name"])
			s.NotContains(spec, "status", "spec.status must be cleared on restart")
			s.Equal([]any{map[string]any{"name": "tag", "value": "v1"}}, spec["params"])
		}
	})
	s.Run("pipelinerun cancel patches spec.status", func() {
		s.createUnstructured(pipelineRuns, "apiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata: {name: build-2}\nspec: {pipelineRef: {name: build}}\n")
		result, err := s.CallTool("tekton_pipelinerun_lifecycle", map[string]any{"name": "build-2", "namespace": "default", "action": "cancel"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Equal("PipelineRun 'build-2' cancelled in namespace 'default'", s.text(result))
		pr, err := client.Resource(pipelineRuns).Namespace("default").Get(ctx, "build-2", metav1.GetOptions{})
		s.Require().NoError(err)
		s.Equal("Cancelled", pr.Object["spec"].(map[string]any)["status"])
	})
	s.Run("taskrun restart", func() {
		result, err := s.CallTool("tekton_taskrun_restart", map[string]any{"name": "build-1-compile", "namespace": "default"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Regexp(`^TaskRun 'build-1-compile' restarted as 'build-1-compile-[a-z0-9]+' in namespace 'default'$`, s.text(result))
	})
	s.Run("pipelinerun logs reports runs without a pod and missing runs", func() {
		result, err := s.CallTool("tekton_pipelinerun_logs", map[string]any{"name": "build-1", "namespace": "default"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Equal("TaskRun 'build-1-compile' in namespace 'default' has not started a pod yet", s.text(result))
		result, err = s.CallTool("tekton_pipelinerun_logs", map[string]any{"name": "build-1", "namespace": "default", "task": "nope"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Equal("No TaskRuns found for PipelineRun 'build-1' in namespace 'default' matching pipeline task 'nope'", s.text(result))
	})
	s.Run("taskrun logs fans out over step containers", func() {
		s.createUnstructured(taskRuns, `
apiVersion: tekton.dev/v1
kind: TaskRun
metadata: {name: with-pod}
spec: {taskRef: {name: compile}}
status:
  podName: no-such-pod
  steps: [{name: compile, container: step-compile}, {name: test, container: step-test}]
  sidecars: [{name: db, container: sidecar-db}]
`)
		result, err := s.CallTool("tekton_taskrun_logs", map[string]any{"name": "with-pod", "namespace": "default", "step": "test"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Contains(s.text(result), "# TaskRun: with-pod [step: test]\nerror retrieving logs: ")
		s.NotContains(s.text(result), "step: compile")
		s.NotContains(s.text(result), "sidecar", "a step filter excludes sidecars")
	})
}

func (s *DynamicToolsSuite) TestConvertedKubevirtTools() {
	ctx := s.T().Context()
	for _, resource := range []string{"virtualmachines", "virtualmachineinstances"} {
		s.Require().NoError(EnvTestEnableCRD(ctx, "kubevirt.io", "v1", resource))
	}
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	s.loadToolManifests(filepath.Join("..", "..", "examples", "dynamic-tools", "charts", "kubevirt-mcp-tools", "templates", "tools.yaml"))
	vms := schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"}
	client := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig())
	defer func() {
		_ = client.Resource(vms).Namespace("default").DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
	}()
	s.createUnstructured(vms, `
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata: {name: dyn-vm}
spec:
  runStrategy: Halted
  template:
    spec:
      volumes:
        - {name: root, dataVolume: {name: dyn-vm-root}}
        - {name: cloudinit, cloudInitNoCloud: {userData: "#cloud-config\npassword: secret"}}
`)
	s.InitMcpClient()
	runStrategy := func() string {
		vm, err := client.Resource(vms).Namespace("default").Get(ctx, "dyn-vm", metav1.GetOptions{})
		s.Require().NoError(err)
		return vm.Object["spec"].(map[string]any)["runStrategy"].(string)
	}
	lifecycle := func(action string) *mcp.CallToolResult {
		result, err := s.CallTool("vm_lifecycle", map[string]any{"namespace": "default", "name": "dyn-vm", "action": action})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		return result
	}
	s.Run("vm_lifecycle keeps the built-in contract", func() {
		s.Equal("VirtualMachine started successfully", lifecycle("start").StructuredContent.(map[string]any)["message"])
		s.Equal("Always", runStrategy())
		s.Equal("VirtualMachine 'dyn-vm' in namespace 'default' is already running", lifecycle("start").StructuredContent.(map[string]any)["message"])
		s.Equal("VirtualMachine stopped successfully", lifecycle("stop").StructuredContent.(map[string]any)["message"])
		s.Equal("Halted", runStrategy())
		s.Equal("VirtualMachine restarted successfully", lifecycle("restart").StructuredContent.(map[string]any)["message"])
		s.Equal("Always", runStrategy())
		s.Contains(s.text(lifecycle("stop")), "runStrategy: Halted", "the final VM is returned")
	})
	s.Run("vm_troubleshoot gathers what exists and tolerates what does not", func() {
		result, err := s.CallTool("vm_troubleshoot", map[string]any{"namespace": "default", "name": "dyn-vm"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		out := result.StructuredContent.(map[string]any)
		s.Equal("Halted", out["virtualMachine"].(map[string]any)["runStrategy"])
		s.Contains(out["virtualMachineInstance"].(map[string]any)["error"], "not found")
		s.Empty(out["virtLauncherPods"])
		s.Contains(s.text(result), "cloudInitNoCloud: <redacted>")
		s.NotContains(s.text(result), "password: secret")
		dvs := out["dataVolumes"].([]any)
		s.Require().Len(dvs, 1, "one DataVolume lookup per dataVolume volume")
		s.Contains(dvs[0].(map[string]any)["error"], "DataVolume", "envtest serves no CDI API; the failure is recorded, not fatal")
	})
}

func (s *DynamicToolsSuite) TestConvertedPodsRun() {
	ctx := s.T().Context()
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	s.loadToolManifests(filepath.Join("..", "..", "examples", "dynamic-tools", "converted", "core.yaml"))
	kc := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())
	defer func() {
		_ = kc.CoreV1().Pods("default").Delete(ctx, "dyn-run", metav1.DeleteOptions{})
		_ = kc.CoreV1().Services("default").Delete(ctx, "dyn-run", metav1.DeleteOptions{})
	}()
	s.InitMcpClient()
	result, err := s.CallTool("pods_run", map[string]any{"namespace": "default", "name": "dyn-run", "image": "busybox", "port": 8080})
	s.Require().NoError(err)
	s.Falsef(result.IsError, "%s", s.text(result))
	s.Contains(s.text(result), "kind: Pod\n")
	s.Contains(s.text(result), "kind: Service\n")
	pod, err := kc.CoreV1().Pods("default").Get(ctx, "dyn-run", metav1.GetOptions{})
	s.Require().NoError(err)
	s.Equal("busybox", pod.Spec.Containers[0].Image)
	s.Equal(int32(8080), pod.Spec.Containers[0].Ports[0].ContainerPort)
	s.Equal("kubernetes-mcp-server-run-sandbox", pod.Labels["app.kubernetes.io/part-of"])
	svc, err := kc.CoreV1().Services("default").Get(ctx, "dyn-run", metav1.GetOptions{})
	s.Require().NoError(err)
	s.Equal("dyn-run", svc.Spec.Selector["app.kubernetes.io/name"])
	s.Equal(int32(8080), svc.Spec.Ports[0].Port)
}

// TestExampleCRDAcceptsGeneratedTools installs the schema of the shipped CRD
// (examples/dynamic-tools/crd.yaml) on the envtest Tool CRD and checks that
// every generated manifest passes structural validation and the CEL rules,
// and that an invalid Tool is rejected.
func (s *DynamicToolsSuite) TestExampleCRDAcceptsGeneratedTools() {
	ctx := s.T().Context()
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "dynamic-tools", "crd.yaml"))
	s.Require().NoError(err)
	var example apiextensionsv1.CustomResourceDefinition
	s.Require().NoError(yaml.Unmarshal(raw, &example))
	crds := apiextensionsclient.NewForConfigOrDie(test.EnvTestRestConfig()).CustomResourceDefinitions()
	current, err := crds.Get(ctx, example.Name, metav1.GetOptions{})
	s.Require().NoError(err, "the envtest Tool CRD must have the same name as the shipped one")
	current.Spec.Versions[0].Schema = example.Spec.Versions[0].Schema
	current.Spec.Versions[0].AdditionalPrinterColumns = example.Spec.Versions[0].AdditionalPrinterColumns
	_, err = crds.Update(ctx, current, metav1.UpdateOptions{})
	s.Require().NoError(err, "the shipped CRD schema must be a valid structural schema with valid CEL rules")
	s.Require().NoError(EnvTestWaitForAPIResourceCondition(ctx, dynamic.Group, dynamic.Version, dynamic.Resource, true))

	for _, file := range []string{
		filepath.Join("..", "..", "examples", "dynamic-tools", "converted", "core.yaml"),
		filepath.Join("..", "..", "examples", "dynamic-tools", "converted", "kcp.yaml"),
		filepath.Join("..", "..", "examples", "dynamic-tools", "charts", "tekton-mcp-tools", "templates", "tools.yaml"),
		filepath.Join("..", "..", "examples", "dynamic-tools", "charts", "kubevirt-mcp-tools", "templates", "tools.yaml"),
	} {
		s.Run(filepath.Base(filepath.Dir(filepath.Dir(file)))+"/"+filepath.Base(file), func() {
			// Fails inside createTool if the API server rejects a manifest.
			s.loadToolManifests(file)
		})
	}
	s.Run("the CEL rules reject a Tool with two kinds of action", func() {
		obj := &unstructured.Unstructured{}
		s.Require().NoError(yaml.Unmarshal([]byte(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata: {name: invalid-two-actions}
spec:
  description: invalid
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
  steps: [{name: a, expression: "1"}]
`), &obj.Object))
		_, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).Create(ctx, obj, metav1.CreateOptions{})
		s.Require().Error(err)
		s.Contains(err.Error(), "exactly one of spec.kubernetes, spec.http or spec.steps")
	})
}
