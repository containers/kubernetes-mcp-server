package mcp

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func TestPodsTopMetricsUnavailable(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		mockServer := NewMockServer()
		defer mockServer.Close()
		c.withKubeConfig(mockServer.config)
		mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Request Performed by DiscoveryClient to Kube API (Get API Groups legacy -core-)
			if req.URL.Path == "/api" {
				_, _ = w.Write([]byte(`{"kind":"APIVersions","versions":[],"serverAddressByClientCIDRs":[{"clientCIDR":"0.0.0.0/0"}]}`))
				return
			}
			// Request Performed by DiscoveryClient to Kube API (Get API Groups)
			if req.URL.Path == "/apis" {
				_, _ = w.Write([]byte(`{"kind":"APIGroupList","apiVersion":"v1","groups":[]}`))
				return
			}
		}))
		podsTopMetricsApiUnavailable, err := c.callTool("pods_top", map[string]interface{}{})
		t.Run("pods_top with metrics API not available", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if !podsTopMetricsApiUnavailable.IsError {
				t.Errorf("call tool should have returned an error")
			}
			if podsTopMetricsApiUnavailable.Content[0].(mcp.TextContent).Text != "failed to get pods top: metrics API is not available" {
				t.Errorf("call tool returned unexpected content: %s", podsTopMetricsApiUnavailable.Content[0].(mcp.TextContent).Text)
			}
		})
	})
}

func TestPodsTopMetricsAvailable(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		mockServer := NewMockServer()
		defer mockServer.Close()
		c.withKubeConfig(mockServer.config)
		mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			println("Request received:", req.Method, req.URL.Path) // TODO: REMOVE LINE
			w.Header().Set("Content-Type", "application/json")
			// Request Performed by DiscoveryClient to Kube API (Get API Groups legacy -core-)
			if req.URL.Path == "/api" {
				_, _ = w.Write([]byte(`{"kind":"APIVersions","versions":["metrics.k8s.io/v1beta1"],"serverAddressByClientCIDRs":[{"clientCIDR":"0.0.0.0/0"}]}`))
				return
			}
			// Request Performed by DiscoveryClient to Kube API (Get API Groups)
			if req.URL.Path == "/apis" {
				_, _ = w.Write([]byte(`{"kind":"APIGroupList","apiVersion":"v1","groups":[]}`))
				return
			}
			// Request Performed by DiscoveryClient to Kube API (Get API Resources)
			if req.URL.Path == "/apis/metrics.k8s.io/v1beta1" {
				_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"metrics.k8s.io/v1beta1","resources":[{"name":"pods","singularName":"","namespaced":true,"kind":"PodMetrics","verbs":["get","list"]}]}`))
				return
			}
			// Pod Metrics from all namespaces
			if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/pods" {
				if req.URL.Query().Get("labelSelector") == "app=pod-ns-5-42" {
					_, _ = w.Write([]byte(`{"kind":"PodMetricsList","apiVersion":"metrics.k8s.io/v1beta1","items":[` +
						`{"metadata":{"name":"pod-ns-5-42","namespace":"ns-5"},"containers":[{"name":"container-1","usage":{"cpu":"42m","memory":"42Mi"}}]}` +
						`]}`))
				} else {
					_, _ = w.Write([]byte(`{"kind":"PodMetricsList","apiVersion":"metrics.k8s.io/v1beta1","items":[` +
						`{"metadata":{"name":"pod-1","namespace":"default"},"containers":[{"name":"container-1","usage":{"cpu":"100m","memory":"200Mi"}},{"name":"container-2","usage":{"cpu":"200m","memory":"300Mi"}}]},` +
						`{"metadata":{"name":"pod-2","namespace":"ns-1"},"containers":[{"name":"container-1-ns-1","usage":{"cpu":"300m","memory":"400Mi"}}]}` +
						`]}`))

				}
				return
			}
			// Pod Metrics from configured namespace
			if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/namespaces/default/pods" {
				_, _ = w.Write([]byte(`{"kind":"PodMetricsList","apiVersion":"metrics.k8s.io/v1beta1","items":[` +
					`{"metadata":{"name":"pod-1","namespace":"default"},"containers":[{"name":"container-1","usage":{"cpu":"10m","memory":"20Mi"}},{"name":"container-2","usage":{"cpu":"30m","memory":"40Mi"}}]}` +
					`]}`))
				return
			}
			// Pod Metrics from ns-5 namespace
			if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/namespaces/ns-5/pods" {
				_, _ = w.Write([]byte(`{"kind":"PodMetricsList","apiVersion":"metrics.k8s.io/v1beta1","items":[` +
					`{"metadata":{"name":"pod-ns-5-1","namespace":"ns-5"},"containers":[{"name":"container-1","usage":{"cpu":"10m","memory":"20Mi"}}]}` +
					`]}`))
				return
			}
			// Pod Metrics from ns-5 namespace with pod-ns-5-5 pod name
			if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/namespaces/ns-5/pods/pod-ns-5-5" {
				_, _ = w.Write([]byte(`{"kind":"PodMetrics","apiVersion":"metrics.k8s.io/v1beta1",` +
					`"metadata":{"name":"pod-ns-5-5","namespace":"ns-5"},` +
					`"containers":[{"name":"container-1","usage":{"cpu":"13m","memory":"37Mi"}}]` +
					`}`))
			}
		}))
		podsTopDefaults, err := c.callTool("pods_top", map[string]interface{}{})
		t.Run("pods_top defaults returns pod metrics from all namespaces", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			textContent := podsTopDefaults.Content[0].(mcp.TextContent).Text
			if podsTopDefaults.IsError {
				t.Fatalf("call tool failed %s", textContent)
			}
			expectedHeaders := regexp.MustCompile(`(?m)^\s*NAMESPACE\s+POD\s+NAME\s+CPU\(cores\)\s+MEMORY\(bytes\)\s*$`)
			if !expectedHeaders.MatchString(textContent) {
				t.Errorf("Expected headers '%s' not found in output:\n%s", expectedHeaders.String(), textContent)
			}
			expectedRows := []string{
				"default\\s+pod-1\\s+container-1\\s+100m\\s+200Mi",
				"default\\s+pod-1\\s+container-2\\s+200m\\s+300Mi",
				"ns-1\\s+pod-2\\s+container-1-ns-1\\s+300m\\s+400Mi",
			}
			for _, row := range expectedRows {
				if !regexp.MustCompile(row).MatchString(textContent) {
					t.Errorf("Expected row '%s' not found in output:\n%s", row, textContent)
				}
			}
			expectedTotal := regexp.MustCompile(`(?m)^\s+600m\s+900Mi\s*$`)
			if !expectedTotal.MatchString(textContent) {
				t.Errorf("Expected total row '%s' not found in output:\n%s", expectedTotal.String(), textContent)
			}
		})
		podsTopConfiguredNamespace, err := c.callTool("pods_top", map[string]interface{}{
			"all_namespaces": false,
		})
		t.Run("pods_top[allNamespaces=false] returns pod metrics from configured namespace", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			textContent := podsTopConfiguredNamespace.Content[0].(mcp.TextContent).Text
			expectedRows := []string{
				"default\\s+pod-1\\s+container-1\\s+10m\\s+20Mi",
				"default\\s+pod-1\\s+container-2\\s+30m\\s+40Mi",
			}
			for _, row := range expectedRows {
				if !regexp.MustCompile(row).MatchString(textContent) {
					t.Errorf("Expected row '%s' not found in output:\n%s", row, textContent)
				}
			}
			expectedTotal := regexp.MustCompile(`(?m)^\s+40m\s+60Mi\s*$`)
			if !expectedTotal.MatchString(textContent) {
				t.Errorf("Expected total row '%s' not found in output:\n%s", expectedTotal.String(), textContent)
			}
		})
		podsTopNamespace, err := c.callTool("pods_top", map[string]interface{}{
			"namespace": "ns-5",
		})
		t.Run("pods_top[namespace=ns-5] returns pod metrics from provided namespace", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			textContent := podsTopNamespace.Content[0].(mcp.TextContent).Text
			expectedRow := regexp.MustCompile(`ns-5\s+pod-ns-5-1\s+container-1\s+10m\s+20Mi`)
			if !expectedRow.MatchString(textContent) {
				t.Errorf("Expected row '%s' not found in output:\n%s", expectedRow.String(), textContent)
			}
			expectedTotal := regexp.MustCompile(`(?m)^\s+10m\s+20Mi\s*$`)
			if !expectedTotal.MatchString(textContent) {
				t.Errorf("Expected total row '%s' not found in output:\n%s", expectedTotal.String(), textContent)
			}
		})
		podsTopNamespaceName, err := c.callTool("pods_top", map[string]interface{}{
			"namespace": "ns-5",
			"name":      "pod-ns-5-5",
		})
		t.Run("pods_top[namespace=ns-5,name=pod-ns-5-5] returns pod metrics from provided namespace and name", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			textContent := podsTopNamespaceName.Content[0].(mcp.TextContent).Text
			expectedRow := regexp.MustCompile(`ns-5\s+pod-ns-5-5\s+container-1\s+13m\s+37Mi`)
			if !expectedRow.MatchString(textContent) {
				t.Errorf("Expected row '%s' not found in output:\n%s", expectedRow.String(), textContent)
			}
			expectedTotal := regexp.MustCompile(`(?m)^\s+13m\s+37Mi\s*$`)
			if !expectedTotal.MatchString(textContent) {
				t.Errorf("Expected total row '%s' not found in output:\n%s", expectedTotal.String(), textContent)
			}
		})
		podsTopNamespaceLabelSelector, err := c.callTool("pods_top", map[string]interface{}{
			"label_selector": "app=pod-ns-5-42",
		})
		t.Run("pods_top[label_selector=app=pod-ns-5-42] returns pod metrics from pods matching selector", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			textContent := podsTopNamespaceLabelSelector.Content[0].(mcp.TextContent).Text
			expectedRow := regexp.MustCompile(`ns-5\s+pod-ns-5-42\s+container-1\s+42m\s+42Mi`)
			if !expectedRow.MatchString(textContent) {
				t.Errorf("Expected row '%s' not found in output:\n%s", expectedRow.String(), textContent)
			}
			expectedTotal := regexp.MustCompile(`(?m)^\s+42m\s+42Mi\s*$`)
			if !expectedTotal.MatchString(textContent) {
				t.Errorf("Expected total row '%s' not found in output:\n%s", expectedTotal.String(), textContent)
			}
		})
	})
}

func TestPodsTopDenied(t *testing.T) {
	deniedResourcesServer := &config.StaticConfig{DeniedResources: []config.GroupVersionKind{{Group: "metrics.k8s.io", Version: "v1beta1"}}}
	testCaseWithContext(t, &mcpContext{staticConfig: deniedResourcesServer}, func(c *mcpContext) {
		mockServer := NewMockServer()
		defer mockServer.Close()
		c.withKubeConfig(mockServer.config)
		mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Request Performed by DiscoveryClient to Kube API (Get API Groups legacy -core-)
			if req.URL.Path == "/api" {
				_, _ = w.Write([]byte(`{"kind":"APIVersions","versions":["metrics.k8s.io/v1beta1"],"serverAddressByClientCIDRs":[{"clientCIDR":"0.0.0.0/0"}]}`))
				return
			}
			// Request Performed by DiscoveryClient to Kube API (Get API Groups)
			if req.URL.Path == "/apis" {
				_, _ = w.Write([]byte(`{"kind":"APIGroupList","apiVersion":"v1","groups":[]}`))
				return
			}
			// Request Performed by DiscoveryClient to Kube API (Get API Resources)
			if req.URL.Path == "/apis/metrics.k8s.io/v1beta1" {
				_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"metrics.k8s.io/v1beta1","resources":[{"name":"pods","singularName":"","namespaced":true,"kind":"PodMetrics","verbs":["get","list"]}]}`))
				return
			}
		}))
		podsTop, _ := c.callTool("pods_top", map[string]interface{}{})
		t.Run("pods_run has error", func(t *testing.T) {
			if !podsTop.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("pods_run describes denial", func(t *testing.T) {
			expectedMessage := "failed to get pods top: resource not allowed: metrics.k8s.io/v1beta1, Kind=PodMetrics"
			if podsTop.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, podsTop.Content[0].(mcp.TextContent).Text)
			}
		})
	})
}
