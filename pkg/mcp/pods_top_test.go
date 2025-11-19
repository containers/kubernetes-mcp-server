package mcp

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
)

type PodsTopSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
}

func (s *PodsTopSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.mockServer = test.NewMockServer()
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
}

func (s *PodsTopSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *PodsTopSuite) TestPodsTopMetricsUnavailable() {
	s.mockServer.Handle(&test.DiscoveryClientHandler{})
	s.InitMcpClient()

	s.Run("pods_top with metrics API not available", func() {
		result, err := s.CallTool("pods_top", map[string]interface{}{})
		s.NoError(err, "call tool failed %v", err)
		s.Require().NoError(err)
		s.True(result.IsError, "call tool should have returned an error")
		s.Equalf("failed to get pods top: metrics API is not available", result.Content[0].(mcp.TextContent).Text,
			"call tool returned unexpected content: %s", result.Content[0].(mcp.TextContent).Text)
	})
}

func (s *PodsTopSuite) TestPodsTopMetricsAvailable() {
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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
					`{"metadata":{"name":"pod-ns-5-42","namespace":"ns-5"},"containers":[{"name":"container-1","usage":{"cpu":"42m","memory":"42Mi","swap":"42Mi"}}]}` +
					`]}`))
			} else {
				_, _ = w.Write([]byte(`{"kind":"PodMetricsList","apiVersion":"metrics.k8s.io/v1beta1","items":[` +
					`{"metadata":{"name":"pod-1","namespace":"default"},"containers":[{"name":"container-1","usage":{"cpu":"100m","memory":"200Mi","swap":"13Mi"}},{"name":"container-2","usage":{"cpu":"200m","memory":"300Mi","swap":"37Mi"}}]},` +
					`{"metadata":{"name":"pod-2","namespace":"ns-1"},"containers":[{"name":"container-1-ns-1","usage":{"cpu":"300m","memory":"400Mi","swap":"42Mi"}}]}` +
					`]}`))

			}
			return
		}
		// Pod Metrics from configured namespace
		if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/namespaces/default/pods" {
			_, _ = w.Write([]byte(`{"kind":"PodMetricsList","apiVersion":"metrics.k8s.io/v1beta1","items":[` +
				`{"metadata":{"name":"pod-1","namespace":"default"},"containers":[{"name":"container-1","usage":{"cpu":"10m","memory":"20Mi","swap":"13Mi"}},{"name":"container-2","usage":{"cpu":"30m","memory":"40Mi","swap":"37Mi"}}]}` +
				`]}`))
			return
		}
		// Pod Metrics from ns-5 namespace
		if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/namespaces/ns-5/pods" {
			_, _ = w.Write([]byte(`{"kind":"PodMetricsList","apiVersion":"metrics.k8s.io/v1beta1","items":[` +
				`{"metadata":{"name":"pod-ns-5-1","namespace":"ns-5"},"containers":[{"name":"container-1","usage":{"cpu":"10m","memory":"20Mi","swap":"42Mi"}}]}` +
				`]}`))
			return
		}
		// Pod Metrics from ns-5 namespace with pod-ns-5-5 pod name
		if req.URL.Path == "/apis/metrics.k8s.io/v1beta1/namespaces/ns-5/pods/pod-ns-5-5" {
			_, _ = w.Write([]byte(`{"kind":"PodMetrics","apiVersion":"metrics.k8s.io/v1beta1",` +
				`"metadata":{"name":"pod-ns-5-5","namespace":"ns-5"},` +
				`"containers":[{"name":"container-1","usage":{"cpu":"13m","memory":"37Mi","swap":"42Mi"}}]` +
				`}`))
		}
	}))
	s.InitMcpClient()

	s.Run("pods_top(defaults) returns pod metrics from all namespaces", func() {
		result, err := s.CallTool("pods_top", map[string]interface{}{})
		s.Require().NotNil(result)
		s.NoErrorf(err, "call tool failed %v", err)
		textContent := result.Content[0].(mcp.TextContent).Text
		s.Falsef(result.IsError, "call tool failed %v", textContent)

		expectedHeaders := regexp.MustCompile(`(?m)^\s*NAMESPACE\s+POD\s+NAME\s+CPU\(cores\)\s+MEMORY\(bytes\)\s+SWAP\(bytes\)\s*$`)
		s.Regexpf(expectedHeaders, textContent, "expected headers '%s' not found in output:\n%s", expectedHeaders.String(), textContent)
		expectedRows := []string{
			"default\\s+pod-1\\s+container-1\\s+100m\\s+200Mi\\s+13Mi",
			"default\\s+pod-1\\s+container-2\\s+200m\\s+300Mi\\s+37Mi",
			"ns-1\\s+pod-2\\s+container-1-ns-1\\s+300m\\s+400Mi\\s+42Mi",
		}

		for _, row := range expectedRows {
			s.Regexpf(row, textContent, "expected row '%s' not found in output:\n%s", row, textContent)
		}

		expectedTotal := regexp.MustCompile(`(?m)^\s+600m\s+900Mi\s+92Mi\s*$`)
		s.Regexpf(expectedTotal, textContent, "expected total row '%s' not found in output:\n%s", expectedTotal.String(), textContent)
	})

	s.Run("pods_top(allNamespaces=false) returns pod metrics from configured namespace", func() {
		result, err := s.CallTool("pods_top", map[string]interface{}{
			"all_namespaces": false,
		})
		s.Require().NotNil(result)
		s.NoErrorf(err, "call tool failed %v", err)
		textContent := result.Content[0].(mcp.TextContent).Text
		s.Falsef(result.IsError, "call tool failed %v", textContent)

		expectedRows := []string{
			"default\\s+pod-1\\s+container-1\\s+10m\\s+20Mi\\s+13Mi",
			"default\\s+pod-1\\s+container-2\\s+30m\\s+40Mi\\s+37Mi",
		}
		for _, row := range expectedRows {
			s.Regexpf(row, textContent, "expected row '%s' not found in output:\n%s", row, textContent)
		}

		expectedTotal := regexp.MustCompile(`(?m)^\s+40m\s+60Mi\s+50Mi\s*$`)
		s.Regexpf(expectedTotal, textContent, "expected total row '%s' not found in output:\n%s", expectedTotal.String(), textContent)
	})

	s.Run("pods_top(namespace=ns-5) returns pod metrics from provided namespace", func() {
		result, err := s.CallTool("pods_top", map[string]interface{}{
			"namespace": "ns-5",
		})
		s.Require().NotNil(result)
		s.NoErrorf(err, "call tool failed %v", err)
		textContent := result.Content[0].(mcp.TextContent).Text
		s.Falsef(result.IsError, "call tool failed %v", textContent)

		expectedRow := regexp.MustCompile(`ns-5\s+pod-ns-5-1\s+container-1\s+10m\s+20Mi\s+42Mi`)
		s.Regexpf(expectedRow, textContent, "expected row '%s' not found in output:\n%s", expectedRow.String(), textContent)

		expectedTotal := regexp.MustCompile(`(?m)^\s+10m\s+20Mi\s+42Mi\s*$`)
		s.Regexpf(expectedTotal, textContent, "expected total row '%s' not found in output:\n%s", expectedTotal.String(), textContent)
	})

	s.Run("pods_top(namespace=ns-5,name=pod-ns-5-5) returns pod metrics from provided namespace and name", func() {
		result, err := s.CallTool("pods_top", map[string]interface{}{
			"namespace": "ns-5",
			"name":      "pod-ns-5-5",
		})
		s.Require().NotNil(result)
		s.NoErrorf(err, "call tool failed %v", err)
		textContent := result.Content[0].(mcp.TextContent).Text
		s.Falsef(result.IsError, "call tool failed %v", textContent)

		expectedRow := regexp.MustCompile(`ns-5\s+pod-ns-5-5\s+container-1\s+13m\s+37Mi\s+42Mi`)
		s.Regexpf(expectedRow, textContent, "expected row '%s' not found in output:\n%s", expectedRow.String(), textContent)

		expectedTotal := regexp.MustCompile(`(?m)^\s+13m\s+37Mi\s+42Mi\s*$`)
		s.Regexpf(expectedTotal, textContent, "expected total row '%s' not found in output:\n%s", expectedTotal.String(), textContent)
	})

	s.Run("pods_top[label_selector=app=pod-ns-5-42] returns pod metrics from pods matching selector", func() {
		result, err := s.CallTool("pods_top", map[string]interface{}{
			"label_selector": "app=pod-ns-5-42",
		})
		s.Require().NotNil(result)
		s.NoErrorf(err, "call tool failed %v", err)
		textContent := result.Content[0].(mcp.TextContent).Text
		s.Falsef(result.IsError, "call tool failed %v", textContent)

		expectedRow := regexp.MustCompile(`ns-5\s+pod-ns-5-42\s+container-1\s+42m\s+42Mi`)
		s.Regexpf(expectedRow, textContent, "expected row '%s' not found in output:\n%s", expectedRow.String(), textContent)

		expectedTotal := regexp.MustCompile(`(?m)^\s+42m\s+42Mi\s+42Mi\s*$`)
		s.Regexpf(expectedTotal, textContent, "expected total row '%s' not found in output:\n%s", expectedTotal.String(), textContent)
	})
}

func (s *PodsTopSuite) TestPodsTopDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { group = "metrics.k8s.io", version = "v1beta1" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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
	s.InitMcpClient()

	s.Run("pods_top (denied)", func() {
		result, err := s.CallTool("pods_top", map[string]interface{}{})
		s.Require().NotNil(result, "toolResult should not be nil")
		s.Run("has error", func() {
			s.Truef(result.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := result.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to get pods top:(.+:)? resource not allowed: metrics.k8s.io/v1beta1, Kind=PodMetrics"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, result.Content[0].(mcp.TextContent).Text)
		})
	})
}

func TestPodsTop(t *testing.T) {
	suite.Run(t, new(PodsTopSuite))
}
