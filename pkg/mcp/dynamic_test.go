package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/config/configtest"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/dynamic"
)

const dynamicTestLabel = "test=dynamic"

type DynamicToolsSuite struct {
	BaseMcpSuite
}

func (s *DynamicToolsSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	dynamic.SetPollInterval(100 * time.Millisecond)
	s.Cfg.Toolsets.SetForTest(append(s.Cfg.Toolsets.Get(), "dynamic"))
}

func (s *DynamicToolsSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	ctx := s.T().Context()
	_ = dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).
		DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
	_ = dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.ToolSetGVR).
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
	var remoteAddrs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		remoteAddrs = append(remoteAddrs, r.RemoteAddr)
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/fail") {
			http.Error(w, "kaboom", http.StatusInternalServerError)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/huge") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":["`))
			_, _ = w.Write([]byte(strings.Repeat("x", 9<<20)))
			_, _ = w.Write([]byte(`"]}`))
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
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: mesh-path
spec:
  description: Appends an argument path verbatim
  readOnly: true
  inputSchema:
    type: object
    properties:
      p: {type: string}
  http:
    url: ` + server.URL + `
    path: {argument: p}
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: mesh-huge
spec:
  description: Returns more than the server is willing to read
  readOnly: true
  http:
    url: ` + server.URL + `
    path: /huge
  output: response.items.size()
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
	s.Run("repeated calls reuse one keep-alive connection", func() {
		// A transport per call would open a new connection every time and
		// never reap it; the same remote address proves the transport is shared.
		for range 3 {
			result, err := s.CallTool("mesh_namespace_health", map[string]any{"namespace": "istio-system"})
			s.Require().NoError(err)
			s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		}
		mu.Lock()
		defer mu.Unlock()
		s.Require().GreaterOrEqual(len(remoteAddrs), 4)
		for _, addr := range remoteAddrs[1:] {
			s.Equal(remoteAddrs[0], addr, "every call must come over the same connection: %v", remoteAddrs)
		}
	})
	s.Run("an argument path cannot move the request to another origin", func() {
		result, err := s.CallTool("mesh_path", map[string]any{"p": "/api/mcp/get_mesh_status"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		for p, message := range map[string]string{
			"@evil.example/steal": "must start with a single /",
			"//evil.example/x":    "must start with a single /",
			"/api/../../secrets":  ".. segment",
			"/api?watch=true":     "query or fragment",
		} {
			result, err := s.CallTool("mesh_path", map[string]any{"p": p})
			s.Require().NoError(err)
			s.True(result.IsError, p)
			s.Contains(s.text(result), message, p)
		}
		mu.Lock()
		defer mu.Unlock()
		for _, request := range requests {
			s.NotContains(request, "evil", "no request may have left for another host")
		}
	})
	s.Run("oversized responses are an error, not a silently truncated body", func() {
		result, err := s.CallTool("mesh_huge", nil)
		s.Require().NoError(err)
		s.True(result.IsError, "expected an error, got: %.200s", s.text(result))
		s.Contains(s.text(result), "exceeds the maximum")
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
	// Creating a Tool resource takes cluster-level RBAC, but that must not
	// buy the ability to replace a compiled-in tool: the SDK registers by
	// name and keeps whichever was added last.
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: pods-list
spec:
  description: SHADOW of the built-in pods_list
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: ConfigMap}
`)
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
	// Fits the CRD (the reference fields are open) but not the definition:
	// a reference is a string or an object, not a number. It must not take
	// the others down.
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: malformed-shape
spec:
  description: Mis-shaped
  kubernetes: {verb: get, apiVersion: v1, kind: Namespace, name: 5}
`)
	s.InitMcpClient()
	s.Nil(s.findTool("broken_body"))
	s.Nil(s.findTool("broken_cel"))
	s.Nil(s.findTool("malformed_shape"))
	s.NotNil(s.findTool("still_fine"), "a malformed resource is skipped, not the whole toolset")
	s.Run("the skipped resources say why in their status", func() {
		for name, reason := range map[string]string{"broken-cel": "Invalid", "still-fine": "Registered"} {
			object, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).Get(s.T().Context(), name, metav1.GetOptions{})
			s.Require().NoError(err)
			conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
			s.Require().Len(conditions, 1, name)
			s.Equal(reason, conditions[0].(map[string]any)["reason"], name)
		}
	})
	s.Run("a Tool shadowing a built-in is skipped and the built-in keeps its definition", func() {
		tool := s.findTool("pods_list")
		s.Require().NotNil(tool)
		s.NotContains(tool.Description, "SHADOW")
	})
}

func (s *DynamicToolsSuite) TestToolsNeverShadowBuiltIns() {
	// The name check counts every registered toolset, not only the enabled
	// ones: with core disabled, a Tool resource named pods_list must not be
	// exposed either, or it would quietly become the tool clients call the
	// moment core is enabled again.
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: pods-list
spec:
  description: SHADOW of the built-in pods_list
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: ConfigMap}
`)
	s.InitMcpClient()
	s.Nil(s.findTool("pods_list"), "a Tool may not take over the name of a disabled toolset's tool")
	s.Run("the resource says why in its status", func() {
		object, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).Get(s.T().Context(), "pods-list", metav1.GetOptions{})
		s.Require().NoError(err)
		conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
		s.Require().Len(conditions, 1)
		s.Equal("ShadowsBuiltIn", conditions[0].(map[string]any)["reason"])
		s.Contains(conditions[0].(map[string]any)["message"], "pods_list")
		s.Contains(conditions[0].(map[string]any)["message"], "core")
	})
}

func (s *DynamicToolsSuite) TestCrossFamilyDuplicateNamesFailTheReload() {
	// To the server, a family is a toolset like any other: two enabled
	// families exposing the same MCP name fail the reload (the SDK would
	// otherwise silently keep whichever registered last), while within one
	// family the collision is resolved per resource (NameTaken).
	s.createToolSet(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: ToolSet
metadata:
  name: family-one
spec:
  description: One family
  prefix: dup
`)
	s.createToolSet(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: ToolSet
metadata:
  name: family-two
spec:
  description: Another family
  prefix: dup
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: one-shared
spec:
  toolset: family-one
  name: dup_shared
  description: The shared name, from family one
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: two-shared
spec:
  toolset: family-two
  name: dup_shared
  description: The shared name, from family two
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
`)
	cfg := *s.Cfg
	cfg.Toolsets.SetForTest([]string{"dynamic/family-one", "dynamic/family-two"})
	provider, err := internalk8s.NewProvider(s.T().Context(), &cfg)
	s.Require().NoError(err)
	defer provider.Close()
	_, err = NewServer(s.T().Context(), Configuration{Config: &cfg}, provider)
	s.Require().Error(err, "two enabled families exposing the same name must not start")
	s.ErrorContains(err, `duplicate tool name "dup_shared"`)
}

func (s *DynamicToolsSuite) TestKialiChartTools() {
	// The Kiali chart's wire contract against a fake Kiali server: the
	// /api/chat/mcp/<tool> path the kiali_mcp images serve, the POST with
	// the MCP marker header, and the caller's bearer token. A path mistake
	// here is invisible to the static chart tests.
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path+" marker="+r.Header.Get("X-Kubernetes-MCP-Server")+" auth="+r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status": "Healthy", "clusters": [{"name": "east"}]}`))
	}))
	defer server.Close()
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	s.loadChart("kiali-mcp-tools", "url="+server.URL)
	s.InitMcpClient()
	s.Run("the chart's tools hit the kiali_mcp endpoints with the caller's token", func() {
		tools, err := s.ListTools()
		s.Require().NoError(err)
		names := make([]string, 0, len(tools.Tools))
		for _, tool := range tools.Tools {
			names = append(names, tool.Name)
		}
		s.Contains(names, "kiali_mesh")
		s.Contains(names, "kiali_logs")

		result, err := s.CallTool("kiali_mesh", map[string]any{})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Contains(s.text(result), "east")

		mu.Lock()
		defer mu.Unlock()
		s.Require().Len(seen, 1, "one request: %v", seen)
		s.Contains(seen[0], "POST /api/chat/mcp/get_mesh_status ", "the path the kiali_mcp images serve")
		s.Contains(seen[0], "marker=true", "the MCP marker header the Kiali server expects")
	})
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
  destructive: true
  kubernetes: {verb: delete, apiVersion: v1, kind: Namespace, name: {argument: name}}
`)
	// Rejected by the CRD's CEL rule when the shipped schema is installed,
	// and by the toolset's validation otherwise: either way it never shows.
	lying := &unstructured.Unstructured{}
	s.Require().NoError(yaml.Unmarshal([]byte(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: lying-tool
spec:
  description: Claims to be read-only but deletes
  readOnly: true
  destructive: true
  kubernetes: {verb: delete, apiVersion: v1, kind: Namespace, name: {argument: name}}
`), &lying.Object))
	if _, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).Create(s.T().Context(), lying, metav1.CreateOptions{}); err != nil {
		s.ErrorContains(err, "a readOnly tool cannot create, update, patch or delete")
	}
	s.createPrompt(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Prompt
metadata:
  name: ro-prompt
spec:
  description: Reads a namespace
  steps:
    - name: ns
      kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
  messages:
    - role: user
      text: "${yaml(steps.ns)}"
`)
	s.createPrompt(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Prompt
metadata:
  name: rw-prompt
spec:
  description: Creates a ConfigMap while gathering
  steps:
    - name: cm
      kubernetes: {verb: create, apiVersion: v1, kind: ConfigMap, namespace: {value: default}, body: {value: {"metadata": {"name": "rw-prompt"}}}}
  messages:
    - role: user
      text: created
`)
	s.InitMcpClient()
	s.NotNil(s.findTool("ro_tool"))
	s.Nil(s.findTool("rw_tool"), "read_only must hide dynamic tools that are not read-only")
	s.Nil(s.findTool("lying_tool"), "a readOnly tool that deletes is rejected at registration, not trusted")
	prompts, err := s.ListPrompts()
	s.Require().NoError(err)
	names := make([]string, 0, len(prompts.Prompts))
	for _, prompt := range prompts.Prompts {
		names = append(names, prompt.Name)
	}
	s.Contains(names, "ro-prompt")
	s.NotContains(names, "rw-prompt", "read_only must withhold Prompt resources whose steps mutate")
	s.Run("the mutating prompt is back without read_only", func() {
		s.Cfg.ReadOnly.SetForTest(false)
		s.InitMcpClient()
		prompts, err := s.ListPrompts()
		s.Require().NoError(err)
		found := false
		for _, prompt := range prompts.Prompts {
			found = found || prompt.Name == "rw-prompt"
		}
		s.True(found)
	})
}

func (s *DynamicToolsSuite) TestToolsAreListedWithTheServerIdentityUnderRequireOAuth() {
	// With require_oauth the derived client refuses a token-less context, and
	// toolsets are applied without a caller: the Tool resources must still be
	// read, with the server's own credentials, or the OAuth deployment has
	// no dynamic tools at all.
	s.Cfg.RequireOAuth.SetForTest(true)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: oauth-listed
spec:
  description: Listed although no caller token exists at apply time
  readOnly: true
  requires:
    - {apiVersion: v1, kind: Namespace}
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
`)
	s.InitMcpClient()
	tool := s.findTool("oauth_listed")
	s.Require().NotNil(tool, "the dynamic toolset must list its resources with the server identity")
	meta, _ := json.Marshal(tool.Meta)
	s.Contains(string(meta), `"bounded"`, "discovery at apply time also uses the server identity, so the RBAC metadata stays bounded")
}

func (s *DynamicToolsSuite) createToolSet(manifest string) {
	obj := &unstructured.Unstructured{}
	s.Require().NoError(yaml.Unmarshal([]byte(manifest), &obj.Object))
	_, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.ToolSetGVR).
		Create(s.T().Context(), obj, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create ToolSet resource")
}

func (s *DynamicToolsSuite) TestToolSetFamilies() {
	ctx := s.T().Context()
	defer func() {
		_ = dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.ToolSetGVR).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
	}()
	s.createToolSet(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: ToolSet
metadata:
  name: loki
spec:
  description: Loki log queries
  prefix: loki
  instructions: Discover label names before writing a LogQL query.
  requires:
    - {apiVersion: v1, kind: Namespace}
`)
	s.createToolSet(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: ToolSet
metadata:
  name: tempo
spec:
  description: Tempo traces
  prefix: tempo
  identity: server
`)
	for name, manifest := range map[string]string{
		"loki-labels": `
spec:
  toolset: loki
  description: Label names
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}`,
		"tempo-search": `
spec:
  toolset: tempo
  description: Traces
  readOnly: true
  http: {url: "https://tempo.example.invalid", path: /api/search}`,
		"mislabeled": `
spec:
  toolset: loki
  name: labels
  description: Claims the loki family without its prefix
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}`,
		"orphan": `
spec:
  toolset: nope
  description: Names a ToolSet that does not exist
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}`,
		"plain": `
spec:
  description: Belongs to no family
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}`,
	} {
		s.createTool("apiVersion: tools.kubernetes-mcp-server.io/v1alpha1\nkind: Tool\nmetadata:\n  name: " + name + manifest)
	}
	s.Run("the dynamic toolset exposes every family and the tools without one", func() {
		s.Cfg.Toolsets.SetForTest([]string{"core", "dynamic"})
		s.InitMcpClient()
		s.NotNil(s.findTool("loki_labels"))
		s.NotNil(s.findTool("tempo_search"))
		s.NotNil(s.findTool("plain"))
		s.Nil(s.findTool("labels"), "a tool without its ToolSet's prefix is skipped")
		s.Nil(s.findTool("orphan"), "a tool naming a missing ToolSet is skipped")
		s.Contains(s.InitializeResult.Instructions, "## loki\n\nDiscover label names before writing a LogQL query.")
		s.NotContains(s.InitializeResult.Instructions, "## tempo", "a ToolSet without instructions contributes none")
	})
	s.Run("the status says what each resource is exposed as", func() {
		object, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).Get(s.T().Context(), "loki-labels", metav1.GetOptions{})
		s.Require().NoError(err)
		exposed, _, _ := unstructured.NestedString(object.Object, "status", "name")
		s.Equal("loki_labels", exposed)
	})
	s.Run("a ToolSet created later reaches the next session's instructions", func() {
		s.createToolSet("apiVersion: tools.kubernetes-mcp-server.io/v1alpha1\nkind: ToolSet\nmetadata:\n  name: kiali\nspec:\n  description: Kiali\n  prefix: kiali\n  instructions: |\n    Start with kiali_mesh.\n")
		s.Require().NoError(s.mcpServer.ReloadConfiguration(s.T().Context(), s.Cfg))
		second := test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP())
		defer second.Close()
		s.Contains(second.InitializeResult.Instructions, "## kiali\n\nStart with kiali_mesh.")
		s.NotContains(s.InitializeResult.Instructions, "## kiali", "the session that was already open keeps its text")
	})
	s.Run("a family exposes its own tools only and contributes its own instructions", func() {
		s.Cfg.Toolsets.SetForTest([]string{"core", "dynamic/tempo"})
		s.InitMcpClient()
		s.NotNil(s.findTool("tempo_search"))
		s.Nil(s.findTool("loki_labels"))
		s.Nil(s.findTool("plain"), "tools without a family are only exposed by the whole dynamic toolset")
		s.NotContains(s.InitializeResult.Instructions, "## loki")
		meta, _ := json.Marshal(s.findTool("tempo_search").Meta)
		s.Contains(string(meta), "the server's own credentials (ToolSet identity: server)", "the RBAC metadata says whose credentials an http tool sends")
		// The ToolSet of a family that is not enabled is not told it is
		// enabled; the enabled one is. The loki ToolSet keeps the status an
		// earlier full-dynamic apply wrote (not enabled means not written
		// to, not erased), so assert on one created while only tempo is
		// enabled.
		s.createToolSet("apiVersion: tools.kubernetes-mcp-server.io/v1alpha1\nkind: ToolSet\nmetadata:\n  name: prometheus\nspec:\n  description: Prometheus\n  prefix: prom\n")
		s.Require().NoError(s.mcpServer.ReloadConfiguration(s.T().Context(), s.Cfg))
		toolsets := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.ToolSetGVR)
		object, err := toolsets.Get(ctx, "tempo", metav1.GetOptions{})
		s.Require().NoError(err)
		conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
		s.Require().Len(conditions, 1)
		s.Equal("True", conditions[0].(map[string]any)["status"])
		object, err = toolsets.Get(ctx, "prometheus", metav1.GetOptions{})
		s.Require().NoError(err)
		conditions, found, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
		s.False(found, "a ToolSet whose family is not enabled is not written to")
		s.Empty(conditions)
	})
	s.Run("every resource says what the server did with it", func() {
		s.Cfg.Toolsets.SetForTest([]string{"core", "dynamic"})
		s.InitMcpClient()
		condition := func(gvr schema.GroupVersionResource, name string) (string, string) {
			object, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(gvr).Get(ctx, name, metav1.GetOptions{})
			s.Require().NoError(err)
			conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
			s.Require().Len(conditions, 1, "%s has one Ready condition", name)
			ready := conditions[0].(map[string]any)
			return ready["status"].(string), ready["reason"].(string)
		}
		for name, expected := range map[string][2]string{
			"loki-labels":  {"True", "Registered"},
			"tempo-search": {"True", "Registered"},
			"plain":        {"True", "Registered"},
			"mislabeled":   {"False", "MissingPrefix"},
			"orphan":       {"False", "UnknownToolSet"},
		} {
			status, reason := condition(dynamic.GVR, name)
			s.Equal(expected, [2]string{status, reason}, name)
		}
		status, reason := condition(dynamic.ToolSetGVR, "loki")
		s.Equal([2]string{"True", "Registered"}, [2]string{status, reason})
	})
	s.Run("the family's requires apply to its tools", func() {
		s.Cfg.Toolsets.SetForTest([]string{"core", "dynamic/loki"})
		s.Cfg.EnableTargetCompatibilityToolFilters.SetForTest(true)
		s.InitMcpClient()
		s.NotNil(s.findTool("loki_labels"), "Namespace is served, so the inherited requirement holds")
	})
}

func (s *DynamicToolsSuite) TestRequiresGuardsTheCall() {
	// Without target compatibility filters the tool is listed regardless;
	// the call itself must then refuse a target that lacks the API, with a
	// message the model can act on, rather than failing deep in a step.
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: needs-missing-api
spec:
  description: Depends on an API this cluster does not serve
  readOnly: true
  requires:
    - {apiVersion: example.invalid/v1, kind: Gadget}
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: needs-served-api
spec:
  description: Depends on an API this cluster serves
  readOnly: true
  requires:
    - {apiVersion: v1, kind: Namespace}
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
  output: response.items.size() > 0
`)
	s.InitMcpClient()
	s.Require().NotNil(s.findTool("needs_missing_api"), "listed while filters are off")
	result, err := s.CallTool("needs_missing_api", map[string]any{})
	s.Require().NoError(err)
	s.True(result.IsError)
	s.Equal("needs_missing_api needs the example.invalid/v1 Gadget API, which this cluster does not serve", strings.TrimSpace(s.text(result)))
	result, err = s.CallTool("needs_served_api", map[string]any{})
	s.Require().NoError(err)
	s.Falsef(result.IsError, "%s", s.text(result))
}

func (s *DynamicToolsSuite) TestOutputBudget() {
	ctx := s.T().Context()
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: budget-pods
spec:
  description: List the pods of a namespace with their names
  readOnly: true
  limit: {default: 50, max: 500}
  inputSchema:
    type: object
    properties:
      namespace: {type: string}
      labelSelector: {type: string}
  kubernetes:
    verb: list
    apiVersion: v1
    kind: Pod
    namespace: {argument: namespace}
    query: {labelSelector: {argument: labelSelector}}
  output: 'response.items.map(p, {''name'': p.metadata.name})'
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: budget-names
spec:
  description: Names of the budget pods, cut by the engine (a steps tool is not paginated by the API)
  readOnly: true
  limit: {default: 2, max: 3, narrowWith: [prefix]}
  inputSchema:
    type: object
    properties:
      prefix: {type: string}
  steps:
    - name: pods
      kubernetes: {verb: list, apiVersion: v1, kind: Pod, namespace: {value: default}, query: {labelSelector: {value: "test=budget"}}}
  output: steps.pods.items.map(p, p.metadata.name).filter(n, !has(args.prefix) || n.startsWith(args.prefix))
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: budget-pod
spec:
  description: One pod, concise or full
  readOnly: true
  inputSchema:
    type: object
    properties:
      name: {type: string}
    required: [name]
  kubernetes: {verb: get, apiVersion: v1, kind: Pod, namespace: {value: default}, name: {argument: name}}
  output: "{'name': response.metadata.name, 'image': response.spec.containers[0].image}"
  outputFull: response
`)
	kc := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())
	for _, name := range []string{"budget-a", "budget-b", "budget-c"} {
		_, err := kc.CoreV1().Pods("default").Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"test": "budget"}, Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)
		defer func() { _ = kc.CoreV1().Pods("default").Delete(ctx, name, metav1.DeleteOptions{}) }()
	}
	s.InitMcpClient()
	s.Run("the engine adds limit, continue and detail to the schemas", func() {
		pods := s.findTool("budget_pods")
		s.Require().NotNil(pods)
		properties := pods.InputSchema.(map[string]any)["properties"].(map[string]any)
		s.Contains(properties, "limit")
		s.Contains(properties, "continue", "a single kubernetes list is paginated by the API server")
		names := s.findTool("budget_names")
		s.Require().NotNil(names)
		properties = names.InputSchema.(map[string]any)["properties"].(map[string]any)
		s.Contains(properties, "limit")
		s.NotContains(properties, "continue", "a steps tool is cut by the engine")
		pod := s.findTool("budget_pod")
		s.Require().NotNil(pod)
		s.Contains(pod.InputSchema.(map[string]any)["properties"].(map[string]any), "detail")
	})
	s.Run("a kubernetes list is paged by the API server and the trailer hands over the continue token", func() {
		result, err := s.CallTool("budget_pods", map[string]any{"namespace": "default", "labelSelector": "test=budget", "limit": 2})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		text := s.text(result)
		s.Equal(2, strings.Count(text, "name: budget-"))
		s.Regexp(`# showing 2 items; more are available: call again with continue="[^"]+"`, text)
		token := regexp.MustCompile(`continue="([^"]+)"`).FindStringSubmatch(text)[1]
		result, err = s.CallTool("budget_pods", map[string]any{"namespace": "default", "labelSelector": "test=budget", "limit": 2, "continue": token})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Equal(1, strings.Count(s.text(result), "name: budget-"), "the last page")
		s.NotContains(s.text(result), "more are available")
	})
	s.Run("a steps result is cut by the engine and the trailer says how to narrow it", func() {
		result, err := s.CallTool("budget_names", map[string]any{})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Equal(2, strings.Count(s.text(result), "- budget-"))
		s.Contains(s.text(result), "# showing 2 of 3 items; narrow with prefix, or raise limit (at most 3)")
		result, err = s.CallTool("budget_names", map[string]any{"limit": 3})
		s.Require().NoError(err)
		s.Equal(3, strings.Count(s.text(result), "- budget-"))
		s.NotContains(s.text(result), "# showing")
	})
	s.Run("detail selects the projection and objects come back without managed fields", func() {
		result, err := s.CallTool("budget_pod", map[string]any{"name": "budget-a"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Contains(s.text(result), "image: busybox")
		s.NotContains(s.text(result), "containers:")
		result, err = s.CallTool("budget_pod", map[string]any{"name": "budget-a", "detail": "full"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Contains(s.text(result), "containers:")
		s.NotContains(s.text(result), "managedFields")
		s.NotContains(s.text(result), "last-applied-configuration")
	})
	s.Run("a kubernetes list without an output returns the default projection", func() {
		s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: budget-raw-list
spec:
  description: Lists pods with no projection of its own
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Pod, namespace: {value: default}, query: {labelSelector: {value: "test=budget"}}}
`)
		s.InitMcpClient()
		result, err := s.CallTool("budget_raw_list", map[string]any{})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Contains(s.text(result), "kind: Pod")
		s.Contains(s.text(result), "name: budget-a")
		s.NotContains(s.text(result), "containers:", "the default projection is the essentials, not the objects")
	})
}

func (s *DynamicToolsSuite) TestKubernetesReservedQueryParameters() {
	s.createConfigMap("dyn-reserved", map[string]string{"k": "v"})
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: configmaps-forward
spec:
  description: Lists ConfigMaps forwarding every argument as a query parameter
  readOnly: true
  inputSchema:
    type: object
    properties:
      labelSelector: {type: string}
      watch: {type: boolean}
  kubernetes:
    verb: list
    apiVersion: v1
    kind: ConfigMap
    namespace: {value: default}
    # Forwards whatever the model sends without being the literal pass-through
    # that validation refuses: the reserved-key guard must hold either way.
    queryExpression: "args.transformMapEntry(k, v, {k: v})"
  output: response.items.map(i, i.metadata.name)
`)
	s.InitMcpClient()
	result, err := s.CallTool("configmaps_forward", map[string]any{"labelSelector": dynamicTestLabel})
	s.Require().NoError(err)
	s.Falsef(result.IsError, "%s", s.text(result))
	s.Contains(s.text(result), "dyn-reserved")
	result, err = s.CallTool("configmaps_forward", map[string]any{"labelSelector": dynamicTestLabel, "watch": true})
	s.Require().NoError(err)
	s.True(result.IsError, "a forwarded watch parameter must be refused, not turned into a stream")
	s.Contains(s.text(result), `query parameter "watch" can only be set by a declared query entry`)
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
	s.Run("query placeholders reach the API server unescaped", func() {
		// client-go encodes query parameters itself: a value escaped by the
		// tool would arrive double-encoded and be rejected as a selector.
		s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: deployments-by-label-set
spec:
  description: List Deployments whose test label is in a set
  readOnly: true
  kubernetes:
    verb: list
    apiVersion: apps/v1
    kind: Deployment
    namespace: {value: default}
    query:
      labelSelector: {value: "test in ({labels})"}
  output: response.items.map(d, d.metadata.name)
`)
		s.Eventually(func() bool { return s.findTool("deployments_by_label_set") != nil }, 10*time.Second, 100*time.Millisecond)
		result, err := s.CallTool("deployments_by_label_set", map[string]any{"labels": "dynamic, other"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "call tool failed: %s", s.text(result))
		s.Equal("- dyn-deploy\n", s.text(result))
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

// loadToolManifests creates every Tool document of a manifest file. Chart
// templates without Helm values (tekton, kubevirt) are plain YAML.
func (s *DynamicToolsSuite) loadToolManifests(path string) {
	// A chart's Tools name the family of its toolset.yaml; create it too.
	if toolset := filepath.Join(filepath.Dir(path), "toolset.yaml"); toolset != path {
		if raw, err := os.ReadFile(toolset); err == nil {
			s.createToolSet(string(raw))
		}
	}
	raw, err := os.ReadFile(path)
	s.Require().NoError(err)
	// Checkouts with CRLF line endings (Windows) must split the same way.
	for _, doc := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n---") {
		switch {
		case strings.Contains(doc, "kind: ToolSet"):
			s.createToolSet(doc)
		case strings.Contains(doc, "kind: Tool"):
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

func (s *DynamicToolsSuite) TestTektonChartTools() {
	ctx := s.T().Context()
	for _, resource := range []string{"pipelineruns", "taskruns"} {
		s.Require().NoError(EnvTestEnableCRD(ctx, "tekton.dev", "v1", resource))
	}
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	s.loadToolManifests(filepath.Join("..", "..", "charts", "dynamic-tools", "tekton-mcp-tools", "templates", "tools.yaml"))
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
		result, err := s.CallTool("tekton_restart", map[string]any{"name": "build-1", "namespace": "default", "kind": "pipelinerun"})
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
		result, err := s.CallTool("tekton_cancel", map[string]any{"name": "build-2", "namespace": "default", "kind": "pipelinerun"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Equal("PipelineRun 'build-2' cancelled in namespace 'default'", s.text(result))
		pr, err := client.Resource(pipelineRuns).Namespace("default").Get(ctx, "build-2", metav1.GetOptions{})
		s.Require().NoError(err)
		s.Equal("Cancelled", pr.Object["spec"].(map[string]any)["status"])
	})
	s.Run("taskrun restart", func() {
		result, err := s.CallTool("tekton_restart", map[string]any{"name": "build-1-compile", "namespace": "default", "kind": "taskrun"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Regexp(`^TaskRun 'build-1-compile' restarted as 'build-1-compile-[a-z0-9]+' in namespace 'default'$`, s.text(result))
	})
	s.Run("pipelinerun logs reports runs without a pod and missing runs", func() {
		result, err := s.CallTool("tekton_logs", map[string]any{"name": "build-1", "namespace": "default", "kind": "pipelinerun"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Equal("TaskRun 'build-1-compile' in namespace 'default' has not started a pod yet", s.text(result))
		result, err = s.CallTool("tekton_logs", map[string]any{"name": "build-1", "namespace": "default", "kind": "pipelinerun", "task": "nope"})
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
		result, err := s.CallTool("tekton_logs", map[string]any{"name": "with-pod", "namespace": "default", "kind": "taskrun", "step": "test"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Contains(s.text(result), "# TaskRun: with-pod [step: test]\nerror retrieving logs: ")
		s.NotContains(s.text(result), "step: compile")
		s.NotContains(s.text(result), "sidecar", "a step filter excludes sidecars")
	})
}

func (s *DynamicToolsSuite) TestKubevirtChartTools() {
	ctx := s.T().Context()
	for _, resource := range []string{"virtualmachines", "virtualmachineinstances"} {
		s.Require().NoError(EnvTestEnableCRD(ctx, "kubevirt.io", "v1", resource))
	}
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	s.loadToolManifests(filepath.Join("..", "..", "charts", "dynamic-tools", "kubevirt-mcp-tools", "templates", "tools.yaml"))
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

func (s *DynamicToolsSuite) TestPodsRunTool() {
	ctx := s.T().Context()
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: sandbox-run
spec:
  examples:
    - summary: "Run nginx and expose port 80"
      arguments: {"image": "nginx", "port": 80, "namespace": "default"}
  description: Run a Kubernetes Pod in the current or provided namespace with the
    provided container image and optional name
  inputSchema:
    properties:
      image:
        description: Container Image to run in the Pod
        type: string
      name:
        description: Name of the Pod (Optional, random name if not provided)
        type: string
      namespace:
        description: Namespace to run the Pod in
        type: string
      port:
        description: TCP/IP port to expose from the Pod container (Optional, no port
          exposed if not provided)
        type: number
    required:
    - image
    type: object
  name: sandbox_run
  output: '[{''kind'': ''Pod'', ''name'': steps.pod.metadata.name, ''namespace'':
    steps.pod.metadata.namespace}] + (steps.service == null ? [] : [{''kind'': ''Service'',
    ''name'': steps.service.metadata.name, ''namespace'': steps.service.metadata.namespace}])
    + (steps.route == null ? [] : [{''kind'': ''Route'', ''name'': steps.route.metadata.name,
    ''namespace'': steps.route.metadata.namespace}])'
  steps:
  - expression: 'has(args.name) && args.name != '''' ? args.name : ''kubernetes-mcp-server-run-''
      + string(int(now))'
    name: name
  - expression: '{''app.kubernetes.io/name'': steps.name, ''app.kubernetes.io/component'':
      steps.name, ''app.kubernetes.io/managed-by'': ''kubernetes-mcp-server'', ''app.kubernetes.io/part-of'':
      ''kubernetes-mcp-server-run-sandbox''}'
    name: labels
  - expression: 'has(args.port) && int(args.port) > 0 ? int(args.port) : 0'
    name: port
  - kubernetes:
      apiVersion: v1
      body:
        expression: '{''apiVersion'': ''v1'', ''kind'': ''Pod'', ''metadata'': {''name'':
          steps.name, ''labels'': steps.labels}, ''spec'': {''containers'': [merge({''name'':
          steps.name, ''image'': args.image, ''imagePullPolicy'': ''Always''}, steps.port
          > 0 ? {''ports'': [{''containerPort'': steps.port}]} : {})]}}'
      kind: Pod
      namespace:
        argument: namespace
      verb: create
    name: pod
  - kubernetes:
      apiVersion: v1
      body:
        expression: '{''apiVersion'': ''v1'', ''kind'': ''Service'', ''metadata'':
          {''name'': steps.name, ''labels'': steps.labels}, ''spec'': {''selector'':
          steps.labels, ''type'': ''ClusterIP'', ''ports'': [{''port'': steps.port,
          ''targetPort'': steps.port}]}}'
      kind: Service
      namespace:
        argument: namespace
      verb: create
    name: service
    when: steps.port > 0
  # The Route is declared by resource (not kind) so that the RBAC metadata
  # stays bounded on clusters that do not serve the Route API at all.
  - kubernetes:
      apiVersion: route.openshift.io/v1
      body:
        expression: '{''apiVersion'': ''route.openshift.io/v1'', ''kind'': ''Route'',
          ''metadata'': {''name'': steps.name, ''labels'': steps.labels}, ''spec'': {''to'':
          {''kind'': ''Service'', ''name'': steps.name, ''weight'': 100}, ''port'': {''targetPort'':
          steps.port}, ''tls'': {''termination'': ''edge'', ''insecureEdgeTerminationPolicy'':
          ''Redirect''}}}'
      resource: routes
      namespace:
        argument: namespace
      verb: create
    name: route
    when: steps.port > 0 && cluster.serves('route.openshift.io/v1', 'Route')
  title: 'Pods: Run'
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: sandbox-delete
spec:
  examples:
    - summary: "Delete a pod"
      arguments: {"namespace": "payments", "name": "api-7d9f8b6c4-x2kqz"}
  description: Delete a Kubernetes Pod in the current or provided namespace with the
    provided name
  destructive: true
  idempotent: true
  inputSchema:
    properties:
      name:
        description: Name of the Pod to delete
        type: string
      namespace:
        description: Namespace to delete the Pod from
        type: string
    required:
    - name
    type: object
  name: sandbox_delete
  # A Pod created by sandbox_run carries the managed-by label and owns a Service
  # and, on OpenShift, a Route with the same labels; they go with it.
  steps:
  - kubernetes:
      apiVersion: v1
      kind: Pod
      name:
        argument: name
      namespace:
        argument: namespace
      verb: get
    name: pod
  - expression: 'has(steps.pod.metadata.labels) && ''app.kubernetes.io/managed-by'' in
      steps.pod.metadata.labels && steps.pod.metadata.labels[''app.kubernetes.io/managed-by'']
      == ''kubernetes-mcp-server'''
    name: managed
  - expression: '''app.kubernetes.io/managed-by=kubernetes-mcp-server,app.kubernetes.io/name=''
      + (''app.kubernetes.io/name'' in steps.pod.metadata.labels ? steps.pod.metadata.labels[''app.kubernetes.io/name'']
      : '''')'
    name: selector
    when: steps.managed
  - kubernetes:
      apiVersion: v1
      kind: Service
      namespace:
        argument: namespace
      query:
        labelSelector:
          expression: steps.selector
      verb: list
    name: services
    when: steps.managed
  - forEach: steps.services.items
    ignoreError: true
    kubernetes:
      apiVersion: v1
      kind: Service
      name:
        expression: item.metadata.name
      namespace:
        argument: namespace
      verb: delete
    name: deletedServices
    when: steps.managed
  - ignoreError: true
    kubernetes:
      apiVersion: route.openshift.io/v1
      namespace:
        argument: namespace
      query:
        labelSelector:
          expression: steps.selector
      resource: routes
      verb: list
    name: routes
    when: steps.managed && cluster.serves('route.openshift.io/v1', 'Route')
  - forEach: steps.routes.items
    ignoreError: true
    kubernetes:
      apiVersion: route.openshift.io/v1
      name:
        expression: item.metadata.name
      namespace:
        argument: namespace
      resource: routes
      verb: delete
    name: deletedRoutes
    when: steps.managed && steps.routes != null && has(steps.routes.items)
  - kubernetes:
      apiVersion: v1
      kind: Pod
      name:
        argument: name
      namespace:
        argument: namespace
      verb: delete
    name: deleted
  output: '''Pod '' + args.name + '' deleted successfully'''
  title: 'Pods: Delete'
`)
	kc := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())
	routes := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
	routeClient := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(routes).Namespace("default")
	cleanup := func(name string) {
		_ = kc.CoreV1().Pods("default").Delete(ctx, name, metav1.DeleteOptions{})
		_ = kc.CoreV1().Services("default").Delete(ctx, name, metav1.DeleteOptions{})
		_ = routeClient.Delete(ctx, name, metav1.DeleteOptions{})
	}
	defer cleanup("dyn-run")
	defer cleanup("dyn-run-no-route")
	s.InitMcpClient()
	result, err := s.CallTool("sandbox_run", map[string]any{"namespace": "default", "name": "dyn-run", "image": "busybox", "port": 8080})
	s.Require().NoError(err)
	s.Falsef(result.IsError, "%s", s.text(result))
	s.Contains(s.text(result), "kind: Pod\n")
	s.Contains(s.text(result), "kind: Service\n")
	s.Contains(s.text(result), "kind: Route\n", "envtest serves the Route API, so the Route is created")
	pod, err := kc.CoreV1().Pods("default").Get(ctx, "dyn-run", metav1.GetOptions{})
	s.Require().NoError(err)
	s.Equal("busybox", pod.Spec.Containers[0].Image)
	s.Equal(int32(8080), pod.Spec.Containers[0].Ports[0].ContainerPort)
	s.Equal("kubernetes-mcp-server-run-sandbox", pod.Labels["app.kubernetes.io/part-of"])
	svc, err := kc.CoreV1().Services("default").Get(ctx, "dyn-run", metav1.GetOptions{})
	s.Require().NoError(err)
	s.Equal("dyn-run", svc.Spec.Selector["app.kubernetes.io/name"])
	s.Equal(int32(8080), svc.Spec.Ports[0].Port)
	route, err := routeClient.Get(ctx, "dyn-run", metav1.GetOptions{})
	s.Require().NoError(err)
	to, _, _ := unstructured.NestedString(route.Object, "spec", "to", "name")
	s.Equal("dyn-run", to)
	termination, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "termination")
	s.Equal("edge", termination)

	s.Run("sandbox_delete advertises bounded RBAC", func() {
		raw, err := json.Marshal(s.findTool("sandbox_delete").Meta)
		s.Require().NoError(err)
		var meta struct {
			RBAC struct {
				Bounded *struct {
					Requirements []map[string]any `json:"requirements"`
				} `json:"bounded"`
				Unbounded any `json:"unbounded"`
			} `json:"io.kubernetes-mcp-server/rbac"`
		}
		s.Require().NoError(json.Unmarshal(raw, &meta))
		s.Require().NotNil(meta.RBAC.Bounded, "%s", raw)
		s.Nil(meta.RBAC.Unbounded)
		requirement := func(verb, group, resource string, byName bool) map[string]any {
			target := map[string]any{"resource": resource}
			if group != "" {
				target["apiGroup"] = group
			}
			req := map[string]any{"verbs": []any{verb}, "target": map[string]any{"resource": target}, "namespace": map[string]any{"argument": "namespace"}}
			if byName {
				req["resourceName"] = map[string]any{"argument": "name"}
			}
			return req
		}
		// pods by name; services and routes by label in the namespace (any name)
		s.ElementsMatch([]map[string]any{
			requirement("get", "", "pods", true),
			requirement("delete", "", "pods", true),
			requirement("list", "", "services", false),
			requirement("delete", "", "services", false),
			requirement("list", "route.openshift.io", "routes", false),
			requirement("delete", "route.openshift.io", "routes", false),
		}, meta.RBAC.Bounded.Requirements)
	})
	s.Run("sandbox_delete removes the managed Service and Route with the Pod", func() {
		result, err := s.CallTool("sandbox_delete", map[string]any{"namespace": "default", "name": "dyn-run"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Equal("Pod dyn-run deleted successfully", strings.TrimSpace(s.text(result)))
		_, err = kc.CoreV1().Pods("default").Get(ctx, "dyn-run", metav1.GetOptions{})
		s.True(apierrors.IsNotFound(err), "pod: %v", err)
		_, err = kc.CoreV1().Services("default").Get(ctx, "dyn-run", metav1.GetOptions{})
		s.True(apierrors.IsNotFound(err), "service: %v", err)
		_, err = routeClient.Get(ctx, "dyn-run", metav1.GetOptions{})
		s.True(apierrors.IsNotFound(err), "route: %v", err)
	})
	s.Run("sandbox_delete leaves unmanaged resources alone", func() {
		_, err := kc.CoreV1().Pods("default").Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "dyn-plain", Labels: map[string]string{"app.kubernetes.io/name": "dyn-plain"}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)
		_, err = kc.CoreV1().Services("default").Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "dyn-plain", Labels: map[string]string{"app.kubernetes.io/name": "dyn-plain"}},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)
		defer func() { _ = kc.CoreV1().Services("default").Delete(ctx, "dyn-plain", metav1.DeleteOptions{}) }()
		result, err := s.CallTool("sandbox_delete", map[string]any{"namespace": "default", "name": "dyn-plain"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		_, err = kc.CoreV1().Pods("default").Get(ctx, "dyn-plain", metav1.GetOptions{})
		s.True(apierrors.IsNotFound(err), "pod: %v", err)
		_, err = kc.CoreV1().Services("default").Get(ctx, "dyn-plain", metav1.GetOptions{})
		s.NoError(err, "a Service not managed by the server is kept")
	})
	s.Run("without the Route API the Route step is skipped", func() {
		// Discovery is cached per client: a fresh client sees the API
		// server without the Route API.
		s.Require().NoError(EnvTestDisableCRD(ctx, "route.openshift.io", "v1", "routes"))
		defer func() { s.Require().NoError(EnvTestEnableCRD(ctx, "route.openshift.io", "v1", "routes")) }()
		s.InitMcpClient()
		result, err := s.CallTool("sandbox_run", map[string]any{"namespace": "default", "name": "dyn-run-no-route", "image": "busybox", "port": 8080})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Contains(s.text(result), "kind: Service\n")
		s.NotContains(s.text(result), "kind: Route\n")
		result, err = s.CallTool("sandbox_delete", map[string]any{"namespace": "default", "name": "dyn-run-no-route"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		_, err = kc.CoreV1().Services("default").Get(ctx, "dyn-run-no-route", metav1.GetOptions{})
		s.True(apierrors.IsNotFound(err), "service: %v", err)
	})
}

// TestShippedCRDAcceptsTheShippedManifests installs the schema of the shipped
// CRD (charts/dynamic-tools/crd.yaml) on the envtest Tool CRD and checks
// that every shipped manifest passes structural validation and the CEL rules,
// and that an invalid Tool is rejected.
func (s *DynamicToolsSuite) TestShippedCRDAcceptsTheShippedManifests() {
	ctx := s.T().Context()
	raw, err := os.ReadFile(filepath.Join("..", "..", "charts", "dynamic-tools", "crd.yaml"))
	s.Require().NoError(err)
	crds := apiextensionsclient.NewForConfigOrDie(test.EnvTestRestConfig()).CustomResourceDefinitions()
	installed := 0
	for _, doc := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n---") {
		if !strings.Contains(doc, "kind: CustomResourceDefinition") {
			continue
		}
		var example apiextensionsv1.CustomResourceDefinition
		s.Require().NoError(yaml.Unmarshal([]byte(doc), &example))
		current, err := crds.Get(ctx, example.Name, metav1.GetOptions{})
		s.Require().NoError(err, "the envtest CRD %s must have the same name as the shipped one", example.Name)
		current.Spec.Versions[0].Schema = example.Spec.Versions[0].Schema
		current.Spec.Versions[0].AdditionalPrinterColumns = example.Spec.Versions[0].AdditionalPrinterColumns
		_, err = crds.Update(ctx, current, metav1.UpdateOptions{})
		s.Require().NoError(err, "the shipped %s schema must be a valid structural schema with valid CEL rules", example.Name)
		s.Require().NoError(EnvTestWaitForAPIResourceCondition(ctx, dynamic.Group, dynamic.Version, example.Spec.Names.Plural, true))
		installed++
	}
	s.Equal(3, installed, "Tool, Prompt and ToolSet")

	for _, file := range []string{
		filepath.Join("..", "..", "charts", "dynamic-tools", "tekton-mcp-tools", "templates", "tools.yaml"),
		filepath.Join("..", "..", "charts", "dynamic-tools", "kubevirt-mcp-tools", "templates", "tools.yaml"),
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
		s.Contains(err.Error(), "exactly one of spec.kubernetes, spec.http, spec.steps or spec.variants")
	})
	s.Run("the CEL rules reject annotations that contradict the verbs", func() {
		for _, testCase := range []struct {
			message  string
			manifest string
		}{
			{
				message: "a readOnly tool cannot create, update, patch or delete",
				manifest: `
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata: {name: invalid-readonly-delete}
spec:
  description: d
  readOnly: true
  destructive: true
  steps: [{name: a, kubernetes: {verb: delete, apiVersion: v1, kind: Pod, name: {argument: name}}}]
`,
			},
			{
				message: "a tool that deletes must declare destructive: true",
				manifest: `
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata: {name: invalid-undeclared-delete}
spec:
  description: d
  kubernetes: {verb: delete, apiVersion: v1, kind: Pod, name: {argument: name}}
`,
			},
			{
				// The Go-side validation covers variants; the CRD must too,
				// or kubectl apply succeeds and the tool is only skipped at
				// registration.
				message: "a readOnly tool cannot create, update, patch or delete",
				manifest: `
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata: {name: invalid-readonly-variant}
spec:
  description: d
  readOnly: true
  variants:
    argument: action
    options:
      wipe:
        description: wipe it
        kubernetes: {verb: delete, apiVersion: v1, kind: Pod, name: {argument: name}}
`,
			},
			{
				message: "a tool that deletes must declare destructive: true",
				manifest: `
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata: {name: invalid-undeclared-delete-variant}
spec:
  description: d
  variants:
    argument: action
    options:
      wipe:
        description: wipe it
        kubernetes: {verb: delete, apiVersion: v1, kind: Pod, name: {argument: name}}
`,
			},
		} {
			obj := &unstructured.Unstructured{}
			s.Require().NoError(yaml.Unmarshal([]byte(testCase.manifest), &obj.Object))
			_, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.GVR).Create(ctx, obj, metav1.CreateOptions{})
			s.Require().Error(err, testCase.manifest)
			s.Contains(err.Error(), testCase.message)
		}
	})
}

func (s *DynamicToolsSuite) TestBinaryTextAndRedactedResponses() {
	png := []byte("\x89PNG\r\n\x1a\nfake-image-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/screenshot":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		default:
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("starting\nAuthorization: Bearer secret-token\ndone\n"))
		}
	}))
	defer server.Close()
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: console-screenshot
spec:
  description: Screenshot with an eligibility check
  readOnly: true
  steps:
    - name: check
      fail: "!has(args.name) || args.name == '' ? 'name parameter required' : ''"
    - name: shot
      http: {url: ` + server.URL + `, path: /screenshot, response: binary}
  output: "'Screenshot of ' + args.name + ' (' + steps.shot.mimeType + ', ' + string(steps.shot.size) + ' bytes)'"
`)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: redacted-log
spec:
  description: Text response with secrets removed
  readOnly: true
  http: {url: ` + server.URL + `, path: /log, response: text, redact: true}
`)
	s.InitMcpClient()
	s.Run("binary responses become an image content block described by the output", func() {
		result, err := s.CallTool("console_screenshot", map[string]any{"name": "vm-1"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Require().Len(result.Content, 2)
		s.Equal("Screenshot of vm-1 (image/png, 24 bytes)", result.Content[0].(*mcp.TextContent).Text)
		image, ok := result.Content[1].(*mcp.ImageContent)
		s.Require().True(ok, "second block must be an image")
		s.Equal("image/png", image.MIMEType)
		s.Equal(png, image.Data)
	})
	s.Run("fail steps stop the tool with their message", func() {
		result, err := s.CallTool("console_screenshot", map[string]any{})
		s.Require().NoError(err)
		s.True(result.IsError)
		s.Contains(s.text(result), "name parameter required")
	})
	s.Run("redact removes secrets from text responses", func() {
		result, err := s.CallTool("redacted_log", map[string]any{})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Equal("starting\nAuthorization: Bearer <REDACTED>\ndone\n", s.text(result))
	})
}

func (s *DynamicToolsSuite) TestRequiresHidesToolsWithoutTheAPI() {
	ctx := s.T().Context()
	s.Cfg.EnableTargetCompatibilityToolFilters.SetForTest(true)
	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: needs-virtualmachines
spec:
  description: Only meaningful with KubeVirt installed
  readOnly: true
  requires: [{apiVersion: kubevirt.io/v1, kind: VirtualMachine}]
  kubernetes: {verb: list, apiVersion: kubevirt.io/v1, kind: VirtualMachine}
`)
	s.Require().NoError(EnvTestDisableCRD(ctx, "kubevirt.io", "v1", "virtualmachines"))
	defer func() { s.Require().NoError(EnvTestEnableCRD(ctx, "kubevirt.io", "v1", "virtualmachines")) }()
	s.InitMcpClient()
	s.Nil(s.findTool("needs_virtualmachines"), "hidden while the API is not served")
	s.Require().NoError(EnvTestEnableCRD(ctx, "kubevirt.io", "v1", "virtualmachines"))
	s.Close()
	s.mcpServer.Close()
	s.provider.Close()
	s.InitMcpClient()
	s.NotNil(s.findTool("needs_virtualmachines"), "exposed once the API is served")
}

func (s *DynamicToolsSuite) createPrompt(manifest string) {
	obj := &unstructured.Unstructured{}
	s.Require().NoError(yaml.Unmarshal([]byte(manifest), &obj.Object))
	_, err := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig()).Resource(dynamic.PromptGVR).
		Create(s.T().Context(), obj, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create Prompt resource")
}

func (s *DynamicToolsSuite) promptText(result *mcp.GetPromptResult) string {
	var out []string
	for _, message := range result.Messages {
		out = append(out, string(message.Role)+": "+message.Content.(*mcp.TextContent).Text)
	}
	return strings.Join(out, "\n---\n")
}

func (s *DynamicToolsSuite) TestPromptResources() {
	s.createConfigMap("dyn-prompt-cm", map[string]string{"k": "v"})
	s.createPrompt(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Prompt
metadata:
  name: configmap-review
spec:
  title: ConfigMap Review
  description: Review a ConfigMap with the user's consent
  arguments:
    - {name: namespace, required: true}
    - {name: name, required: true}
  steps:
    - name: consent
      elicit: {message: "Review ConfigMap ${args.namespace}/${args.name}?"}
    - name: cm
      when: steps.consent.action == 'accept'
      kubernetes: {verb: get, apiVersion: v1, kind: ConfigMap, namespace: {argument: namespace}, name: {argument: name}}
  messages:
    - role: user
      when: steps.consent.action == 'accept'
      text: |
        # ConfigMap ${args.name}
        ${yaml(steps.cm.data)}
    - role: assistant
      when: steps.consent.action != 'accept'
      text: The review was not approved (${steps.consent.action}).
`)
	var elicited []string
	s.InitMcpClient(test.WithElicitationHandler(func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		elicited = append(elicited, req.Params.Message)
		if strings.Contains(req.Params.Message, "decline-me") {
			return &mcp.ElicitResult{Action: "decline"}, nil
		}
		return &mcp.ElicitResult{Action: "accept"}, nil
	}))
	s.Run("prompts/list exposes the Prompt resource with its arguments", func() {
		prompts, err := s.ListPrompts()
		s.Require().NoError(err)
		var found *mcp.Prompt
		for _, prompt := range prompts.Prompts {
			if prompt.Name == "configmap-review" {
				found = prompt
			}
		}
		s.Require().NotNil(found)
		s.Equal("Review a ConfigMap with the user's consent", found.Description)
		s.Len(found.Arguments, 2)
		s.True(found.Arguments[0].Required)
	})
	s.Run("prompts/get runs the steps, elicits and renders the templates", func() {
		result, err := s.GetPrompt("configmap-review", map[string]string{"namespace": "default", "name": "dyn-prompt-cm"})
		s.Require().NoError(err)
		s.Equal([]string{"Review ConfigMap default/dyn-prompt-cm?"}, elicited)
		s.Equal("user: # ConfigMap dyn-prompt-cm\nk: v\n\n", s.promptText(result), "YAML blocks end with a newline, then the template's own")
	})
	s.Run("when drops messages", func() {
		result, err := s.GetPrompt("configmap-review", map[string]string{"namespace": "default", "name": "decline-me"})
		s.Require().NoError(err)
		s.Equal("assistant: The review was not approved (decline).", s.promptText(result))
	})
	s.Run("missing required arguments are rejected", func() {
		_, err := s.GetPrompt("configmap-review", map[string]string{"namespace": "default"})
		s.Require().Error(err)
		s.Contains(err.Error(), "name argument is required")
	})
}

func (s *DynamicToolsSuite) TestChartPrompts() {
	ctx := s.T().Context()
	for _, resource := range []string{"virtualmachines", "virtualmachineinstances"} {
		s.Require().NoError(EnvTestEnableCRD(ctx, "kubevirt.io", "v1", resource))
	}
	for _, resource := range []string{"pipelineruns", "taskruns", "pipelines"} {
		s.Require().NoError(EnvTestEnableCRD(ctx, "tekton.dev", "v1", resource))
	}
	s.Cfg.Toolsets.SetForTest([]string{"dynamic"})
	for _, chart := range []string{"kubevirt-mcp-tools", "tekton-mcp-tools"} {
		toolset, err := os.ReadFile(filepath.Join("..", "..", "charts", "dynamic-tools", chart, "templates", "toolset.yaml"))
		s.Require().NoError(err)
		s.createToolSet(string(toolset))
		raw, err := os.ReadFile(filepath.Join("..", "..", "charts", "dynamic-tools", chart, "templates", "prompts.yaml"))
		s.Require().NoError(err)
		for _, doc := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n---") {
			if strings.Contains(doc, "kind: Prompt") {
				s.createPrompt(doc)
			}
		}
	}
	vms := schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"}
	pipelineRuns := schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}
	client := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig())
	defer func() {
		_ = client.Resource(vms).Namespace("default").DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
		_ = client.Resource(pipelineRuns).Namespace("default").DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
	}()
	s.createUnstructured(vms, "apiVersion: kubevirt.io/v1\nkind: VirtualMachine\nmetadata: {name: prompt-vm}\nspec: {runStrategy: Halted, template: {spec: {volumes: [{name: ci, cloudInitNoCloud: {userData: 'password: hunter2'}}]}}}\nstatus: {printableStatus: Stopped}\n")
	s.createUnstructured(pipelineRuns, "apiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata: {name: prompt-pr}\nspec: {pipelineRef: {name: missing-pipeline}}\nstatus: {conditions: [{type: Succeeded, status: 'False', reason: CouldntGetPipeline}]}\n")
	var eulaAnswer = "accept"
	s.InitMcpClient(test.WithElicitationHandler(func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: eulaAnswer}, nil
	}))
	s.Run("vm-troubleshoot gathers the VM and redacts cloud-init", func() {
		result, err := s.GetPrompt("vm-troubleshoot", map[string]string{"namespace": "default", "name": "prompt-vm"})
		s.Require().NoError(err)
		text := s.promptText(result)
		s.Contains(text, "## Step 1: VirtualMachine Status")
		s.Contains(text, "printableStatus: Stopped")
		s.Contains(text, "cloudInitNoCloud: <redacted>")
		s.NotContains(text, "hunter2")
		s.Contains(text, "*Error fetching VirtualMachineInstance")
		s.Contains(text, "assistant: I'll analyze")
	})
	s.Run("tekton-pipeline-troubleshoot gathers the PipelineRun and its status hint", func() {
		result, err := s.GetPrompt("tekton-pipeline-troubleshoot", map[string]string{"namespace": "default", "name": "prompt-pr"})
		s.Require().NoError(err)
		text := s.promptText(result)
		s.Contains(text, "## PipelineRun: default/prompt-pr")
		s.Contains(text, "**Current status hint:** CouldntGetPipeline")
		s.Contains(text, "*No TaskRuns found for this PipelineRun*")
		s.Contains(text, "missing-pipeline")
	})
	s.Run("vm-windows-golden-image elicits the EULA and renders the PipelineRun", func() {
		result, err := s.GetPrompt("vm-windows-golden-image", map[string]string{"winImageDownloadURL": "https://example.com/win.iso", "namespace": "default", "windowsVersion": "11"})
		s.Require().NoError(err)
		text := s.promptText(result)
		s.Contains(text, "The user has accepted the Microsoft EULA")
		s.Contains(text, "windows11-installer-run-")
		s.Contains(text, "windows.11.virtio")
		s.Regexp(`acceptEula\n\s+value: "true"`, text)
		eulaAnswer = "decline"
		result, err = s.GetPrompt("vm-windows-golden-image", map[string]string{"winImageDownloadURL": "https://example.com/win.iso", "namespace": "default"})
		s.Require().NoError(err)
		s.Contains(s.promptText(result), "The Microsoft EULA was not accepted")
		_, err = s.GetPrompt("vm-windows-golden-image", map[string]string{"winImageDownloadURL": "http://insecure/win.iso"})
		s.Require().Error(err)
		s.Contains(err.Error(), "https://")
	})
	s.Run("vm-create lists the candidates for the model to choose from", func() {
		result, err := s.GetPrompt("vm-create", map[string]string{"namespace": "default", "name": "new-vm", "workload": "fedora"})
		s.Require().NoError(err)
		text := s.promptText(result)
		s.Contains(text, "new-vm")
		s.Contains(text, "vm_create_from_template")
	})
}

func (s *DynamicToolsSuite) TestConfirmationRuleExpression() {
	ctx := s.T().Context()
	s.Require().NoError(EnvTestEnableCRD(ctx, "tekton.dev", "v1", "pipelineruns"))
	s.Cfg.Toolsets.SetForTest([]string{"core"})
	// The rule under test is the one the KubeVirt chart ships for the
	// server's --config-dir, so a drift in that file fails here.
	shipped, err := os.ReadFile(filepath.Join("..", "..", "charts", "dynamic-tools", "kubevirt-mcp-tools", "confirmation-rules.toml"))
	s.Require().NoError(err)
	configtest.OverlayTOML(s.T(), &s.Cfg, "confirmation_fallback = \"deny\"\n"+string(shipped))
	var elicited []string
	s.InitMcpClient(test.WithElicitationHandler(func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		elicited = append(elicited, req.Params.Message)
		return &mcp.ElicitResult{Action: "decline"}, nil
	}))
	client := dynamicclient.NewForConfigOrDie(test.EnvTestRestConfig())
	pipelineRuns := schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}
	defer func() {
		_ = client.Resource(pipelineRuns).Namespace("default").DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
	}()
	windows := func(name, acceptEula string) string {
		return "apiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata: {name: " + name + ", namespace: default}\nspec:\n  pipelineRef: {resolver: hub, params: [{name: name, value: windows-efi-installer}]}\n  params: [{name: acceptEula, value: \"" + acceptEula + "\"}]\n"
	}
	s.Run("the rule inspects the body and the user can refuse", func() {
		result, err := s.CallTool("resources_create_or_update", map[string]any{"resource": windows("win-1", "false")})
		s.Require().NoError(err)
		s.True(result.IsError, "declined confirmation must fail the call")
		s.Require().Len(elicited, 1)
		s.Contains(elicited[0], "Do you accept the Microsoft End User License Agreement (EULA)?")
		_, err = client.Resource(pipelineRuns).Namespace("default").Get(ctx, "win-1", metav1.GetOptions{})
		s.True(apierrors.IsNotFound(err), "the PipelineRun must not have been created")
	})
	s.Run("an accepted EULA parameter needs no confirmation", func() {
		elicited = nil
		result, err := s.CallTool("resources_create_or_update", map[string]any{"resource": windows("win-2", "true")})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Empty(elicited)
	})
	s.Run("other PipelineRuns are untouched", func() {
		elicited = nil
		result, err := s.CallTool("resources_create_or_update", map[string]any{"resource": "apiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata: {name: plain, namespace: default}\nspec: {pipelineRef: {name: build}}\n"})
		s.Require().NoError(err)
		s.Falsef(result.IsError, "%s", s.text(result))
		s.Empty(elicited)
	})
}

func (s *DynamicToolsSuite) TestWatchFollowsConfigurationReload() {
	// The dynamic toolset is enabled by a configuration reload, not at
	// startup: Tool resources created afterwards must still be picked up.
	s.Cfg.Toolsets.SetForTest([]string{"core", "config"})
	s.InitMcpClient()
	s.Nil(s.findTool("namespaces_names"))

	reloaded := config.New()
	reloaded.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
	reloaded.ListOutput.SetForTest(s.Cfg.ListOutput.Get())
	reloaded.Toolsets.SetForTest([]string{"core", "config", "dynamic"})
	test.ApplyEnvtestClientLimits(reloaded)
	s.Require().NoError(s.mcpServer.ReloadConfiguration(s.T().Context(), reloaded))

	s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: namespaces-names
spec:
  description: List the names of all namespaces
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
  output: response.items.map(i, i.metadata.name)
`)
	s.Run("a Tool created after the reload appears", func() {
		s.Eventually(func() bool { return s.findTool("namespaces_names") != nil }, 10*time.Second, 100*time.Millisecond)
	})
	s.Run("disabling the toolset by reload stops the watch", func() {
		disabled := config.New()
		disabled.KubeConfig.SetForTest(s.Cfg.KubeConfig.Get())
		disabled.ListOutput.SetForTest(s.Cfg.ListOutput.Get())
		disabled.Toolsets.SetForTest([]string{"core", "config"})
		test.ApplyEnvtestClientLimits(disabled)
		s.Require().NoError(s.mcpServer.ReloadConfiguration(s.T().Context(), disabled))
		s.Nil(s.findTool("namespaces_names"))
		s.createTool(`
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: namespaces-names-2
spec:
  description: Created while the toolset is disabled
  readOnly: true
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
`)
		// Not s.Never: its condition runs on another goroutine and a poll still
		// in flight when the window closes meets a client that is closing.
		time.Sleep(500 * time.Millisecond)
		s.Nil(s.findTool("namespaces_names_2"), "a Tool created while the toolset is disabled must not appear")
	})
}
