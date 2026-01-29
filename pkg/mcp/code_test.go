package mcp

import (
	"encoding/json"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type CodeSuite struct {
	BaseMcpSuite
}

func (s *CodeSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	// Enable the code toolset for these tests
	s.Cfg.Toolsets = []string{"code"}
}

func (s *CodeSuite) TestCodeEvaluateSDKGlobals() {
	s.InitMcpClient()

	s.Run("evaluate_script(script=namespace)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": "namespace",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns default namespace", func() {
			s.Equal("default", toolResult.Content[0].(mcp.TextContent).Text)
		})
	})

	s.Run("evaluate_script(script=typeof ctx)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": "typeof ctx",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("ctx is an object", func() {
			s.Equal("object", toolResult.Content[0].(mcp.TextContent).Text)
		})
	})

	s.Run("evaluate_script(script=typeof k8s)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": "typeof k8s",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("k8s is an object", func() {
			s.Equal("object", toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
	testCases := []string{"CoreV1", "coreV1", "COREV1", "AppsV1", "APPSV1", "DiscoveryClient", "DynamicClient", "MetricsV1beta1Client"}
	for _, methodName := range testCases {
		s.Run("evaluate_script(script=typeof k8s."+methodName+")", func() {
			toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
				"script": "typeof k8s." + methodName,
			})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			s.Run("coreV1 is a function", func() {
				s.Equal("function", toolResult.Content[0].(mcp.TextContent).Text)
			})
		})
	}
}

func (s *CodeSuite) TestCodeEvaluateSDKIntrospection() {
	s.InitMcpClient()

	s.Run("evaluate_script (discover k8s client methods)", func() {
		// Demonstrates how LLMs can discover available API clients on the k8s object
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const methods = [];
				for (const key in k8s) {
					if (typeof k8s[key] === 'function') {
						methods.push(key);
					}
				}
				JSON.stringify(methods.sort());
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns available clients", func() {
			var methods []string
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &methods)
			s.NoError(err, "result should be valid JSON array")
			// Method names are lowercase due to JSON tag field mapping
			s.Contains(methods, "coreV1", "should include coreV1")
			s.Contains(methods, "appsV1", "should include appsV1")
			s.Contains(methods, "discoveryClient", "should include discoveryClient")
			s.Contains(methods, "dynamicClient", "should include dynamicClient")
			s.Contains(methods, "metricsV1beta1Client", "should include metricsV1beta1Client")
		})
	})

	s.Run("evaluate_script (discover CoreV1 resources)", func() {
		// Demonstrates how LLMs can discover available resources on a client
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const coreV1 = k8s.coreV1();
				const resources = [];
				for (const key in coreV1) {
					if (typeof coreV1[key] === 'function') {
						resources.push(key);
					}
				}
				JSON.stringify(resources.sort());
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns available resources", func() {
			var resources []string
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &resources)
			s.NoError(err, "result should be valid JSON array")
			// Method names are lowercase due to JSON tag field mapping
			s.Contains(resources, "pods", "should include pods")
			s.Contains(resources, "services", "should include services")
			s.Contains(resources, "namespaces", "should include namespaces")
			s.Contains(resources, "configMaps", "should include configMaps")
			s.Contains(resources, "secrets", "should include secrets")
		})
	})

	s.Run("evaluate_script (discover Pod operations)", func() {
		// Demonstrates how LLMs can discover available operations on a resource
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const pods = k8s.coreV1().pods(namespace);
				const operations = [];
				for (const key in pods) {
					if (typeof pods[key] === 'function') {
						operations.push(key);
					}
				}
				JSON.stringify(operations.sort());
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns available operations", func() {
			var operations []string
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &operations)
			s.NoError(err, "result should be valid JSON array")
			// Method names are lowercase due to JSON tag field mapping
			s.Contains(operations, "list", "should include list")
			s.Contains(operations, "get", "should include get")
			s.Contains(operations, "create", "should include create")
			s.Contains(operations, "delete", "should include delete")
		})
	})
}

// TestCodeEvaluateAPICompliance verifies that the JavaScript k8s object exposes all methods
// from the api.KubernetesClient interface. This ensures backward compatibility: scripts
// written for a previous version of kubernetes-mcp-server will work in future versions.
func (s *CodeSuite) TestCodeEvaluateAPICompliance() {
	s.InitMcpClient()

	// These are the methods from api.KubernetesClient interface that should be exposed to JavaScript.
	// If this test fails after adding new methods to KubernetesClient, add them here.
	// If this test fails after removing methods, scripts using those methods will break.
	requiredMethods := []string{
		// From kubernetes.Interface (typed clients)
		"coreV1",
		"appsV1",
		"batchV1",
		"networkingV1",
		"rbacV1",
		// From api.KubernetesClient
		"namespaceOrDefault",
		"discoveryClient",
		"dynamicClient",
		"metricsV1beta1Client",
	}

	s.Run("evaluate_script (API compliance check)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const methods = [];
				for (const key in k8s) {
					if (typeof k8s[key] === 'function') {
						methods.push(key);
					}
				}
				JSON.stringify(methods);
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("exposes all required api.KubernetesClient methods", func() {
			var methods []string
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &methods)
			s.NoError(err, "result should be valid JSON array")

			for _, required := range requiredMethods {
				s.Contains(methods, required,
					"JavaScript k8s object must expose '%s' for API compliance", required)
			}
		})
	})
}

func (s *CodeSuite) TestCodeEvaluateErrors() {
	s.InitMcpClient()

	s.Run("evaluate_script (missing script parameter)", func() {
		toolResult, _ := s.CallTool("evaluate_script", map[string]interface{}{})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Contains(toolResult.Content[0].(mcp.TextContent).Text, "script parameter required")
		})
	})

	s.Run("evaluate_script(script=)", func() {
		toolResult, _ := s.CallTool("evaluate_script", map[string]interface{}{
			"script": "",
		})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Contains(toolResult.Content[0].(mcp.TextContent).Text, "script cannot be empty")
		})
	})

	s.Run("evaluate_script (syntax error)", func() {
		toolResult, _ := s.CallTool("evaluate_script", map[string]interface{}{
			"script": "function(",
		})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Contains(toolResult.Content[0].(mcp.TextContent).Text, "SyntaxError")
		})
	})

	s.Run("evaluate_script (runtime error)", func() {
		toolResult, _ := s.CallTool("evaluate_script", map[string]interface{}{
			"script": "throw new Error('test error message')",
		})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Contains(toolResult.Content[0].(mcp.TextContent).Text, "script error")
			s.Contains(toolResult.Content[0].(mcp.TextContent).Text, "test error message")
		})
	})

	s.Run("evaluate_script (undefined variable)", func() {
		toolResult, _ := s.CallTool("evaluate_script", map[string]interface{}{
			"script": "undefinedVariable.property",
		})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
		})
	})
}

func (s *CodeSuite) TestCodeEvaluateNullAndUndefined() {
	s.InitMcpClient()

	s.Run("evaluate_script(script=undefined)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": "undefined",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns empty string", func() {
			s.Equal("", toolResult.Content[0].(mcp.TextContent).Text)
		})
	})

	s.Run("evaluate_script(script=null)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": "null",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns empty string", func() {
			s.Equal("", toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
}

func (s *CodeSuite) TestCodeEvaluateKubernetesIntegration() {
	s.InitMcpClient()

	s.Run("evaluate_script (list namespaces via k8s client)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const namespaces = k8s.coreV1().namespaces().list(ctx, {});
				JSON.stringify(namespaces.items.map(ns => ns.metadata.name));
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns namespace names", func() {
			var namespaces []string
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &namespaces)
			s.NoError(err, "result should be valid JSON array")
			s.Contains(namespaces, "default", "should include default namespace")
		})
	})

	s.Run("evaluate_script (list pods and transform output)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const pods = k8s.coreV1().pods("").list(ctx, {});
				const result = pods.items.map(p => ({
					name: p.metadata.name,
					namespace: p.metadata.namespace
				}));
				JSON.stringify(result);
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns transformed pod data", func() {
			var pods []map[string]interface{}
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &pods)
			s.NoError(err, "result should be valid JSON array")
			s.GreaterOrEqual(len(pods), 1, "should have at least one pod")
			if len(pods) > 0 {
				s.Contains(pods[0], "name", "should have name field")
				s.Contains(pods[0], "namespace", "should have namespace field")
			}
		})
	})

	s.Run("evaluate_script (filter and aggregate pods)", func() {
		// Demonstrates the value of code evaluation: complex filtering and aggregation
		// that would be inefficient with individual tool calls
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const pods = k8s.coreV1().pods("").list(ctx, {});
				const summary = {
					total: pods.items.length,
					byNamespace: {}
				};
				pods.items.forEach(p => {
					const ns = p.metadata.namespace;
					summary.byNamespace[ns] = (summary.byNamespace[ns] || 0) + 1;
				});
				JSON.stringify(summary);
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns aggregated summary", func() {
			var summary map[string]interface{}
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &summary)
			s.NoError(err, "result should be valid JSON object")
			s.Contains(summary, "total", "should have total field")
			s.Contains(summary, "byNamespace", "should have byNamespace field")
		})
	})

	s.Run("evaluate_script (uppercase method names)", func() {
		// Models typically use uppercase method names like CoreV1(), Pods(), List()
		// because that's what they know from Kubernetes client documentation.
		// This test verifies the case-insensitive method resolution works.
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const namespaces = k8s.CoreV1().Namespaces().List(ctx, {});
				JSON.stringify(namespaces.items.map(ns => ns.metadata.name));
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns namespace names", func() {
			var namespaces []string
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &namespaces)
			s.NoError(err, "result should be valid JSON array")
			s.Contains(namespaces, "default", "should include default namespace")
		})
	})

	s.Run("evaluate_script (combine multiple API calls with JavaScript)", func() {
		// This test demonstrates the power of code evaluation: combining data from
		// multiple Kubernetes API calls using JavaScript array methods (flatMap, filter, map).
		// This would be inefficient or impossible with individual tool calls.
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				// Get all namespaces and for each, get its pods and configmaps count
				const namespaces = k8s.coreV1().namespaces().list(ctx, {});
				const result = namespaces.items.flatMap(ns => {
					const nsName = ns.metadata.name;
					const pods = k8s.coreV1().pods(nsName).list(ctx, {});
					const configMaps = k8s.coreV1().configMaps(nsName).list(ctx, {});
					return [{
						namespace: nsName,
						podCount: pods.items.length,
						configMapCount: configMaps.items.length,
						podNames: pods.items.map(p => p.metadata.name)
					}];
				});
				JSON.stringify(result);
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns combined namespace data", func() {
			var result []map[string]interface{}
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &result)
			s.NoError(err, "result should be valid JSON array")
			s.GreaterOrEqual(len(result), 1, "should have at least one namespace")
			// Find the default namespace in results
			var defaultNs map[string]interface{}
			for _, ns := range result {
				if ns["namespace"] == "default" {
					defaultNs = ns
					break
				}
			}
			s.NotNil(defaultNs, "should include default namespace")
			s.Contains(defaultNs, "podCount", "should have podCount")
			s.Contains(defaultNs, "configMapCount", "should have configMapCount")
			s.Contains(defaultNs, "podNames", "should have podNames array")
		})
	})
}

func (s *CodeSuite) TestCodeEvaluateResourceCreation() {
	s.InitMcpClient()
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)

	s.Run("evaluate_script (create pod)", func() {
		podName := "code-evaluate-code-created-pod"
		// Create a Pod using JavaScript with standard Kubernetes YAML/JSON structure.
		// The JavaScript Proxy transparently converts the standard structure (with metadata wrapper)
		// to the format expected by Go's Kubernetes client structs - no helper function needed.
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const pod = {
					apiVersion: "v1",
					kind: "Pod",
					metadata: {
						name: "code-evaluate-code-created-pod",
						namespace: namespace,
						labels: {
							"app": "test",
							"created-by": "code-evaluate"
						}
					},
					spec: {
						containers: [{
							name: "nginx",
							image: "nginx:latest"
						}]
					}
				};
				const created = k8s.coreV1().pods(namespace).create(ctx, pod, {});
				JSON.stringify({
					name: created.metadata.name,
					namespace: created.metadata.namespace,
					labels: created.metadata.labels
				});
			`,
		})
		s.T().Cleanup(func() { _ = kc.CoreV1().Pods("default").Delete(s.T().Context(), podName, metav1.DeleteOptions{}) })
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns created pod info", func() {
			var result map[string]interface{}
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &result)
			s.NoError(err, "result should be valid JSON object")
			s.Equal(podName, result["name"], "should return created pod name")
			s.Equal("default", result["namespace"], "should return pod namespace")
		})
		s.Run("pod exists in cluster", func() {
			// Verify the pod was actually created in the cluster
			pod, err := kc.CoreV1().Pods("default").Get(s.T().Context(), podName, metav1.GetOptions{})
			s.NoError(err, "pod should exist in cluster")
			s.Equal(podName, pod.Name, "pod name should match")
			s.Equal("test", pod.Labels["app"], "pod should have app label")
			s.Equal("code-evaluate", pod.Labels["created-by"], "pod should have created-by label")
			s.Len(pod.Spec.Containers, 1, "pod should have one container")
			s.Equal("nginx", pod.Spec.Containers[0].Name, "container name should be nginx")
			s.Equal("nginx:latest", pod.Spec.Containers[0].Image, "container image should be nginx:latest")
		})
	})
}

func (s *CodeSuite) TestCodeEvaluateDynamicClient() {
	s.InitMcpClient()
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)

	s.Run("evaluate_script (list pods with dynamic client)", func() {
		// Use dynamic client to list pods in the default namespace
		// GVR fields can be lowercase or uppercase - the proxy handles both
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const gvr = {group: "", version: "v1", resource: "pods"};
				const pods = k8s.dynamicClient().resource(gvr).namespace("default").list(ctx, {});
				JSON.stringify(pods.items.map(p => p.metadata.name));
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns pod names array", func() {
			var names []string
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &names)
			s.NoError(err, "result should be valid JSON array")
		})
	})

	s.Run("evaluate_script (create configmap with dynamic client)", func() {
		configMapName := "code-dynamic-client-test-cm"
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const gvr = {group: "", version: "v1", resource: "configmaps"};
				const cm = {
					apiVersion: "v1",
					kind: "ConfigMap",
					metadata: {
						name: "code-dynamic-client-test-cm",
						namespace: "default"
					},
					data: {"key": "value"}
				};
				const created = k8s.dynamicClient().resource(gvr).namespace("default").create(ctx, cm, {});
				created.metadata.name;
			`,
		})
		s.T().Cleanup(func() {
			_ = kc.CoreV1().ConfigMaps("default").Delete(s.T().Context(), configMapName, metav1.DeleteOptions{})
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns created configmap name", func() {
			text := toolResult.Content[0].(mcp.TextContent).Text
			s.Equal(configMapName, text)
		})
		s.Run("configmap exists in cluster", func() {
			cm, err := kc.CoreV1().ConfigMaps("default").Get(s.T().Context(), configMapName, metav1.GetOptions{})
			s.NoError(err, "configmap should exist")
			s.Equal("value", cm.Data["key"])
		})
	})
}

func (s *CodeSuite) TestCodeEvaluatePatch() {
	s.InitMcpClient()
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)

	configMapName := "code-patch-test-cm"

	// Create a configmap first
	_, err := kc.CoreV1().ConfigMaps("default").Create(s.T().Context(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: "default",
		},
		Data: map[string]string{"original": "value"},
	}, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create test configmap")
	s.T().Cleanup(func() {
		_ = kc.CoreV1().ConfigMaps("default").Delete(s.T().Context(), configMapName, metav1.DeleteOptions{})
	})

	s.Run("evaluate_script (patch configmap with merge patch)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const patchData = JSON.stringify({data: {patched: "new-value"}});
				// Convert string to byte array for patch
				const bytes = [];
				for (var i = 0; i < patchData.length; i++) {
					bytes.push(patchData.charCodeAt(i));
				}
				const patched = k8s.coreV1().configMaps("default").patch(
					ctx,
					"code-patch-test-cm",
					"application/merge-patch+json",
					bytes,
					{}
				);
				JSON.stringify({
					name: patched.metadata.name,
					data: patched.data
				});
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			if toolResult.IsError {
				s.T().Logf("Patch error: %v", toolResult.Content)
			}
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns patched configmap", func() {
			text := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(text, "patched")
			s.Contains(text, "new-value")
		})
		s.Run("configmap was updated in cluster", func() {
			cm, err := kc.CoreV1().ConfigMaps("default").Get(s.T().Context(), configMapName, metav1.GetOptions{})
			s.NoError(err)
			s.Equal("new-value", cm.Data["patched"])
		})
	})
}

func (s *CodeSuite) TestCodeEvaluateDiscoveryClient() {
	s.InitMcpClient()

	s.Run("evaluate_script (list server groups)", func() {
		// Use discovery client to list available API groups
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const groups = k8s.discoveryClient().serverGroups();
				JSON.stringify(groups.groups.map(g => g.name));
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns API group names", func() {
			var groups []string
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &groups)
			s.NoError(err, "result should be valid JSON array")
			// Core API group is represented as empty string
			s.Contains(groups, "", "should include core API group (empty string)")
		})
	})

	s.Run("evaluate_script (get server resources for core API)", func() {
		// Use discovery client to list resources in the core API group
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const resources = k8s.discoveryClient().serverResourcesForGroupVersion("v1");
				JSON.stringify(resources.resources.map(r => r.name).sort());
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns core API resources", func() {
			var resources []string
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &resources)
			s.NoError(err, "result should be valid JSON array")
			s.Contains(resources, "pods", "should include pods")
			s.Contains(resources, "services", "should include services")
			s.Contains(resources, "configmaps", "should include configmaps")
			s.Contains(resources, "namespaces", "should include namespaces")
		})
	})

	s.Run("evaluate_script (get server version)", func() {
		// Use discovery client to get the server version
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const version = k8s.discoveryClient().serverVersion();
				JSON.stringify({
					major: version.major,
					minor: version.minor,
					platform: version.platform
				});
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns server version info", func() {
			var version map[string]interface{}
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &version)
			s.NoError(err, "result should be valid JSON object")
			s.Contains(version, "major", "should have major version")
			s.Contains(version, "minor", "should have minor version")
		})
	})

	s.Run("evaluate_script (introspect discovery client)", func() {
		// Demonstrates how LLMs can discover available methods on the discovery client
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const dc = k8s.discoveryClient();
				const methods = [];
				for (const key in dc) {
					if (typeof dc[key] === 'function') {
						methods.push(key);
					}
				}
				JSON.stringify(methods.sort());
			`,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed: %v", toolResult.Content)
		})
		s.Run("returns available methods", func() {
			var methods []string
			text := toolResult.Content[0].(mcp.TextContent).Text
			err := json.Unmarshal([]byte(text), &methods)
			s.NoError(err, "result should be valid JSON array")
			s.Contains(methods, "serverGroups", "should include serverGroups")
			s.Contains(methods, "serverVersion", "should include serverVersion")
		})
	})
}

func (s *CodeSuite) TestCodeEvaluateDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Namespace" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()

	s.Run("evaluate_script (denied namespace access)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const namespaces = k8s.coreV1().namespaces().list(ctx, {});
				JSON.stringify(namespaces.items.map(ns => ns.metadata.name));
			`,
		})
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "(.+:)? resource not allowed: /v1, Kind=Namespace"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func (s *CodeSuite) TestCodeEvaluateForbidden() {
	s.InitMcpClient()
	s.T().Cleanup(func() { restoreAuth(s.T().Context()) })
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)
	// Remove all permissions - user will have forbidden access
	s.Require().NoError(client.RbacV1().ClusterRoles().Delete(s.T().Context(), "allow-all", metav1.DeleteOptions{}))

	s.Run("evaluate_script (forbidden namespace access)", func() {
		toolResult, err := s.CallTool("evaluate_script", map[string]interface{}{
			"script": `
				const namespaces = k8s.coreV1().namespaces().list(ctx, {});
				JSON.stringify(namespaces.items.map(ns => ns.metadata.name));
			`,
		})
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes forbidden", func() {
			s.Contains(toolResult.Content[0].(mcp.TextContent).Text, "namespaces is forbidden",
				"error message should indicate forbidden")
		})
	})
}

func TestCode(t *testing.T) {
	suite.Run(t, new(CodeSuite))
}
