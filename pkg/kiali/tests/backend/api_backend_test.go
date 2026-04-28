//go:build kiali_contract
// +build kiali_contract

package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
	"github.com/stretchr/testify/suite"
)

// ContractTestSuite tests the contract of Kiali MCP endpoints
// (POST /api/chat/mcp/<tool>) that the kubernetes-mcp-server delegates to.
// Each test sends realistic arguments matching the tool's input schema and
// asserts a successful (2xx) response with a non-empty body.
type ContractTestSuite struct {
	suite.Suite
	kialiURL   string
	kialiToken string
	httpClient *http.Client
	testNS     string
	tracingOn  bool
}

func (s *ContractTestSuite) SetupSuite() {
	s.kialiURL = strings.TrimSuffix(os.Getenv("KIALI_URL"), "/")
	if s.kialiURL == "" {
		s.kialiURL = "http://localhost:20001/kiali"
	}
	s.kialiToken = os.Getenv("KIALI_TOKEN")

	s.testNS = os.Getenv("TEST_NAMESPACE")
	if s.testNS == "" {
		s.testNS = "bookinfo"
	}

	s.httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	s.tracingOn = s.detectTracing()
}

// detectTracing queries Kiali's /api/tracing endpoint to determine whether
// distributed tracing (Jaeger/Tempo) is enabled.
func (s *ContractTestSuite) detectTracing() bool {
	req, err := http.NewRequest(http.MethodGet, s.kialiURL+"/api/tracing", nil)
	if err != nil {
		return false
	}
	if s.kialiToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.kialiToken)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	var info struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return false
	}
	return info.Enabled
}

// mcpCall POSTs a JSON body to a Kiali MCP tool endpoint and returns the response.
func (s *ContractTestSuite) mcpCall(endpoint string, args map[string]interface{}) (*http.Response, []byte, error) {
	if args == nil {
		args = map[string]interface{}{}
	}
	body, err := json.Marshal(args)
	if err != nil {
		return nil, nil, err
	}

	fullURL := s.kialiURL + endpoint
	req, err := http.NewRequest(http.MethodPost, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}

	if s.kialiToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.kialiToken)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return resp, nil, err
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return resp, nil, err
	}
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		basePath := strings.Split(endpoint, "?")[0]
		s.T().Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		s.T().Logf("❌ FAILED REQUEST: POST %s", basePath)
		s.T().Logf("   Full URL: %s", fullURL)
		s.T().Logf("   Status Code: %d", resp.StatusCode)
		if len(respBody) > 0 {
			bodyStr := string(respBody)
			if len(bodyStr) > 1000 {
				bodyStr = bodyStr[:1000] + "..."
			}
			s.T().Logf("   Response Body: %s", bodyStr)
		}
		s.T().Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	}

	return resp, respBody, nil
}

// requireNotToolNotFound asserts the response is NOT the handler-level
// "Tool 'xxx' not found" 404, which would mean the endpoint isn't registered.
// Any other status (including tool-level 404s like "Trace not found" or
// "not available when tracing is disabled") is acceptable.
func (s *ContractTestSuite) requireNotToolNotFound(endpoint string, resp *http.Response, body []byte) {
	if resp.StatusCode == http.StatusNotFound {
		s.False(strings.Contains(string(body), "' not found"),
			"Endpoint %s returned handler-level 'Tool not found' 404 — endpoint is not registered", endpoint)
	}
}

// requireSuccess asserts a 2xx status and non-empty response body.
func (s *ContractTestSuite) requireSuccess(endpoint string, resp *http.Response, body []byte) {
	s.Require().GreaterOrEqual(resp.StatusCode, 200,
		"Endpoint %s returned status %d, expected 2xx", endpoint, resp.StatusCode)
	s.Require().Less(resp.StatusCode, 300,
		"Endpoint %s returned status %d, expected 2xx", endpoint, resp.StatusCode)
	s.Require().NotEmpty(body,
		"Endpoint %s returned empty response body", endpoint)
}

// requireValidJSON asserts the response body is valid JSON and returns the raw decoded value.
func (s *ContractTestSuite) requireValidJSON(endpoint string, body []byte) interface{} {
	var parsed interface{}
	err := json.Unmarshal(body, &parsed)
	s.Require().NoError(err, "Endpoint %s returned invalid JSON: %s", endpoint, string(body))
	return parsed
}

// requireJSONObject asserts the response is a JSON object and returns it.
func (s *ContractTestSuite) requireJSONObject(endpoint string, body []byte) map[string]interface{} {
	parsed := s.requireValidJSON(endpoint, body)
	obj, ok := parsed.(map[string]interface{})
	s.Require().True(ok, "Endpoint %s expected JSON object, got %T", endpoint, parsed)
	return obj
}

// requireJSONKeys asserts the JSON object response contains all expected top-level keys.
func (s *ContractTestSuite) requireJSONKeys(endpoint string, body []byte, keys ...string) map[string]interface{} {
	obj := s.requireJSONObject(endpoint, body)
	for _, key := range keys {
		s.Contains(obj, key, "Endpoint %s response missing expected key %q", endpoint, key)
	}
	return obj
}

// requireJSONString asserts the response is a JSON-encoded string (e.g. markdown text)
// and returns the decoded string.
func (s *ContractTestSuite) requireJSONString(endpoint string, body []byte) string {
	parsed := s.requireValidJSON(endpoint, body)
	str, ok := parsed.(string)
	s.Require().True(ok, "Endpoint %s expected JSON string, got %T", endpoint, parsed)
	s.Require().NotEmpty(str, "Endpoint %s returned empty string", endpoint)
	return str
}

func (s *ContractTestSuite) TestGetMeshStatus() {
	s.Run("returns mesh status with non-empty response", func() {
		resp, body, err := s.mcpCall(tools.KialiGetMeshStatusEndpoint, nil)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiGetMeshStatusEndpoint, resp, body)
		s.requireJSONKeys(tools.KialiGetMeshStatusEndpoint, body,
			"components", "environment")
	})
}

func (s *ContractTestSuite) TestGetMeshTrafficGraph() {
	s.Run("returns graph for test namespace", func() {
		args := map[string]interface{}{
			"namespaces": s.testNS,
			"graphType":  "versionedApp",
		}
		resp, body, err := s.mcpCall(tools.KialiGetMeshTrafficGraphEndpoint, args)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiGetMeshTrafficGraphEndpoint, resp, body)
		s.requireJSONKeys(tools.KialiGetMeshTrafficGraphEndpoint, body,
			"nodes", "graphType")
	})
}

func (s *ContractTestSuite) TestListOrGetResources() {
	s.Run("lists services in test namespace", func() {
		args := map[string]interface{}{
			"resourceType": "service",
			"namespaces":   s.testNS,
		}
		resp, body, err := s.mcpCall(tools.KialiListOrGetResourcesEndpoint, args)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiListOrGetResourcesEndpoint, resp, body)
		obj := s.requireJSONObject(tools.KialiListOrGetResourcesEndpoint, body)
		s.NotEmpty(obj, "list_or_get_resources response should have at least one cluster key")
	})

	s.Run("lists workloads in test namespace", func() {
		args := map[string]interface{}{
			"resourceType": "workload",
			"namespaces":   s.testNS,
		}
		resp, body, err := s.mcpCall(tools.KialiListOrGetResourcesEndpoint, args)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiListOrGetResourcesEndpoint, resp, body)
		obj := s.requireJSONObject(tools.KialiListOrGetResourcesEndpoint, body)
		s.NotEmpty(obj, "list_or_get_resources response should have at least one cluster key")
	})
}

func (s *ContractTestSuite) TestGetMetrics() {
	s.Run("returns metrics for a service", func() {
		args := map[string]interface{}{
			"resourceType": "service",
			"namespace":    s.testNS,
			"resourceName": "productpage",
		}
		resp, body, err := s.mcpCall(tools.KialiGetMetricsEndpoint, args)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiGetMetricsEndpoint, resp, body)
		s.requireJSONKeys(tools.KialiGetMetricsEndpoint, body,
			"overview", "traffic", "throughput", "latency")
	})
}

func (s *ContractTestSuite) TestGetLogs() {
	s.Run("returns logs for a workload", func() {
		args := map[string]interface{}{
			"namespace": s.testNS,
			"name":      "productpage-v1",
		}
		resp, body, err := s.mcpCall(tools.KialiGetLogsEndpoint, args)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiGetLogsEndpoint, resp, body)
		s.requireJSONString(tools.KialiGetLogsEndpoint, body)
	})
}

func (s *ContractTestSuite) TestGetPodPerformance() {
	s.Run("returns pod performance for a workload", func() {
		args := map[string]interface{}{
			"namespace":    s.testNS,
			"workloadName": "productpage-v1",
		}
		resp, body, err := s.mcpCall(tools.KialiGetPodPerformanceEndpoint, args)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiGetPodPerformanceEndpoint, resp, body)
		s.requireJSONString(tools.KialiGetPodPerformanceEndpoint, body)
	})
}

func (s *ContractTestSuite) TestManageIstioConfigRead() {
	s.Run("lists istio config", func() {
		args := map[string]interface{}{
			"action": "list",
		}
		resp, body, err := s.mcpCall(tools.KialiManageIstioConfigReadEndpoint, args)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiManageIstioConfigReadEndpoint, resp, body)
		s.requireValidJSON(tools.KialiManageIstioConfigReadEndpoint, body)
	})
}

func (s *ContractTestSuite) TestManageIstioConfigCRUD() {
	var createdName string

	s.Run("creates a ServiceEntry", func() {
		createdName = fmt.Sprintf("contract-test-%d", time.Now().UnixMilli())
		seData := map[string]interface{}{
			"apiVersion": "networking.istio.io/v1",
			"kind":       "ServiceEntry",
			"metadata": map[string]interface{}{
				"name":      createdName,
				"namespace": s.testNS,
			},
			"spec": map[string]interface{}{
				"location":   "MESH_EXTERNAL",
				"resolution": "NONE",
				"ports": []map[string]interface{}{
					{"name": "http", "protocol": "HTTP", "number": 80},
				},
				"hosts": []string{"contract-test.example.com"},
			},
		}
		dataBytes, err := json.Marshal(seData)
		s.Require().NoError(err)

		args := map[string]interface{}{
			"action":    "create",
			"namespace": s.testNS,
			"group":     "networking.istio.io",
			"version":   "v1",
			"kind":      "ServiceEntry",
			"object":    createdName,
			"data":      string(dataBytes),
		}
		resp, body, err := s.mcpCall(tools.KialiManageIstioConfigEndpoint, args)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiManageIstioConfigEndpoint, resp, body)
		s.requireValidJSON(tools.KialiManageIstioConfigEndpoint, body)
	})

	s.Run("deletes the ServiceEntry", func() {
		if createdName == "" {
			s.T().Skip("create step did not produce a resource name")
			return
		}
		args := map[string]interface{}{
			"action":    "delete",
			"namespace": s.testNS,
			"group":     "networking.istio.io",
			"version":   "v1",
			"kind":      "ServiceEntry",
			"object":    createdName,
		}
		resp, body, err := s.mcpCall(tools.KialiManageIstioConfigEndpoint, args)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiManageIstioConfigEndpoint, resp, body)
	})
}

func (s *ContractTestSuite) TestListTraces() {
	s.Run("returns traces or error when tracing disabled", func() {
		args := map[string]interface{}{
			"namespace":   s.testNS,
			"serviceName": "productpage",
		}
		resp, body, err := s.mcpCall(tools.KialiListTracesEndpoint, args)
		s.Require().NoError(err)
		if s.tracingOn {
			s.requireSuccess(tools.KialiListTracesEndpoint, resp, body)
			s.requireJSONKeys(tools.KialiListTracesEndpoint, body,
				"summary", "traces")
		} else {
			s.requireNotToolNotFound(tools.KialiListTracesEndpoint, resp, body)
		}
	})
}

func (s *ContractTestSuite) TestGetTraceDetails() {
	s.Run("returns trace details or expected error", func() {
		args := map[string]interface{}{
			"traceId":   "0000000000000001",
			"namespace": s.testNS,
		}
		resp, body, err := s.mcpCall(tools.KialiGetTraceDetailsEndpoint, args)
		s.Require().NoError(err)

		// A fake trace ID legitimately returns 404 "Trace not found" from the tool.
		// Ensure it's not the handler-level "Tool 'xxx' not found" 404.
		if resp.StatusCode == http.StatusNotFound {
			s.requireNotToolNotFound(tools.KialiGetTraceDetailsEndpoint, resp, body)
		} else {
			s.True(resp.StatusCode >= 200 && resp.StatusCode < 300,
				"get_trace_details should return 200 or 404, got %d", resp.StatusCode)
		}
	})
}

func TestContract(t *testing.T) {
	suite.Run(t, new(ContractTestSuite))
}
