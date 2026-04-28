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
	testService  string
	testWorkload string
	testTraceID  string
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
	s.testService = os.Getenv("TEST_SERVICE")
	if s.testService == "" {
		s.testService = "productpage"
	}
	s.testWorkload = os.Getenv("TEST_WORKLOAD")
	if s.testWorkload == "" {
		s.testWorkload = "productpage-v1"
	}
	s.testTraceID = os.Getenv("TEST_TRACE_ID")

	s.httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}
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

// requireToolError asserts a JSON error response with the expected status and
// at least one distinguishing substring in the error message.
func (s *ContractTestSuite) requireToolError(endpoint string, resp *http.Response, body []byte, expectedStatus int, expectedSubstrings ...string) {
	s.Require().Equal(expectedStatus, resp.StatusCode,
		"Endpoint %s returned status %d, expected %d", endpoint, resp.StatusCode, expectedStatus)
	s.requireNotToolNotFound(endpoint, resp, body)

	var payload struct {
		Error string `json:"error"`
	}
	s.Require().NoError(json.Unmarshal(body, &payload),
		"Endpoint %s returned a non-JSON error payload: %s", endpoint, string(body))
	s.Require().NotEmpty(payload.Error,
		"Endpoint %s returned an empty error message", endpoint)

	errorText := strings.ToLower(payload.Error)
	for _, expected := range expectedSubstrings {
		if strings.Contains(errorText, strings.ToLower(expected)) {
			return
		}
	}

	s.FailNow(fmt.Sprintf("Endpoint %s returned unexpected error: %s", endpoint, payload.Error))
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
func (s *ContractTestSuite) requireJSONMessage(endpoint string, body []byte) string {
	var message string
	s.Require().NoError(json.Unmarshal(body, &message),
		"Endpoint %s returned an unexpected non-string payload: %s", endpoint, string(body))
	s.Require().NotEmpty(message,
		"Endpoint %s returned an empty message payload", endpoint)
	return message
}

type traceListPayload struct {
	Summary struct {
		Namespace    string  `json:"namespace"`
		Service      string  `json:"service"`
		TotalFound   int     `json:"total_found"`
		AvgDurationM float64 `json:"avg_duration_ms"`
	} `json:"summary"`
	Traces []struct {
		ID string `json:"id"`
	} `json:"traces"`
}

func (s *ContractTestSuite) fetchTraceList(serviceName string) (*traceListPayload, bool) {
	args := map[string]interface{}{
		"namespace":   s.testNS,
		"serviceName": serviceName,
	}
	resp, body, err := s.mcpCall(tools.KialiListTracesEndpoint, args)
	s.Require().NoError(err)

	switch resp.StatusCode {
	case http.StatusOK:
		s.requireSuccess(tools.KialiListTracesEndpoint, resp, body)

		var payload traceListPayload
		s.Require().NoError(json.Unmarshal(body, &payload))
		s.Equal(s.testNS, payload.Summary.Namespace)
		s.Equal(serviceName, payload.Summary.Service)
		if payload.Summary.TotalFound > 0 {
			s.Require().NotEmpty(payload.Traces,
				"list_traces reported %d traces but returned an empty trace list", payload.Summary.TotalFound)
			s.NotEmpty(payload.Traces[0].ID,
				"list_traces should return a trace id when traces are present")
		}

		return &payload, true

	case http.StatusNotFound:
		s.requireToolError(tools.KialiListTracesEndpoint, resp, body, http.StatusNotFound, "tracing")
		return nil, false

	default:
		s.FailNow(fmt.Sprintf("list_traces returned unexpected status %d", resp.StatusCode))
		return nil, false
	}
}

func (s *ContractTestSuite) requireServiceEntryHost(name, expectedHost string) {
	args := map[string]interface{}{
		"action":    "get",
		"namespace": s.testNS,
		"group":     "networking.istio.io",
		"version":   "v1",
		"kind":      "ServiceEntry",
		"object":    name,
	}
	resp, body, err := s.mcpCall(tools.KialiManageIstioConfigReadEndpoint, args)
	s.Require().NoError(err)
	s.requireSuccess(tools.KialiManageIstioConfigReadEndpoint, resp, body)

	var payload struct {
		Resource struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				Hosts []string `json:"hosts"`
			} `json:"spec"`
		} `json:"resource"`
	}
	s.Require().NoError(json.Unmarshal(body, &payload))
	s.Equal(name, payload.Resource.Metadata.Name)
	s.Equal(s.testNS, payload.Resource.Metadata.Namespace)
	s.Contains(payload.Resource.Spec.Hosts, expectedHost)
}

func (s *ContractTestSuite) requireServiceEntryMissing(name string) {
	args := map[string]interface{}{
		"action":    "get",
		"namespace": s.testNS,
		"group":     "networking.istio.io",
		"version":   "v1",
		"kind":      "ServiceEntry",
		"object":    name,
	}
	resp, body, err := s.mcpCall(tools.KialiManageIstioConfigReadEndpoint, args)
	s.Require().NoError(err)
	s.requireSuccess(tools.KialiManageIstioConfigReadEndpoint, resp, body)

	message := s.requireJSONMessage(tools.KialiManageIstioConfigReadEndpoint, body)
	s.Contains(message, "does not exist")
	s.Contains(message, name)
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

		// Token-efficiency check: service listing should return a compact summary structure
		// (cluster -> []{name, namespace, health, configuration, details, labels}) rather than
		// raw Kiali /api/services payload (which includes large validations/istioReferences blocks).
		type serviceSummary struct {
			Name          string `json:"name"`
			Namespace     string `json:"namespace"`
			Health        string `json:"health"`
			Configuration string `json:"configuration"`
			Details       string `json:"details"`
			Labels        string `json:"labels"`
		}
		var payload map[string][]serviceSummary
		s.Require().NoError(json.Unmarshal(body, &payload),
			"list_or_get_resources(service) returned unexpected JSON: %s", string(body))
		s.Require().NotEmpty(payload, "expected at least one cluster key in response")

		seenAtLeastOne := false
		for cluster, services := range payload {
			s.NotEmpty(cluster, "cluster key must be non-empty")
			if len(services) == 0 {
				continue
			}
			seenAtLeastOne = true
			for _, svc := range services {
				s.NotEmpty(svc.Name, "service name should not be empty")
				s.NotEmpty(svc.Namespace, "service namespace should not be empty")
				s.NotEmpty(svc.Configuration, "configuration summary should not be empty")
				s.NotEmpty(svc.Labels, "labels summary should not be empty (use 'None' if no labels)")
				// Health/details may be empty depending on telemetry and Istio references.
				_ = svc.Health
				_ = svc.Details
			}
		}
		s.Require().True(seenAtLeastOne, "expected at least one service in response")

		// Heuristic: compact output should not contain raw payload top-level blocks.
		s.NotContains(string(body), `"validations"`, "response should not include raw validations block")
		s.NotContains(string(body), `"istioReferences"`, "response should not include raw istioReferences block")
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
			"resourceName": s.testService,
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
			"name":      s.testWorkload,
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
			"workloadName": s.testWorkload,
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
	originalHost := "contract-test.example.com"
	updatedHost := "contract-test-updated.example.com"

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
				"hosts": []string{originalHost},
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

		message := s.requireJSONMessage(tools.KialiManageIstioConfigEndpoint, body)
		s.Contains(message, "Successfully created ServiceEntry")
		s.Contains(message, createdName)
	})

	s.Run("reads back the created ServiceEntry", func() {
		if createdName == "" {
			s.T().Skip("create step did not produce a resource name")
			return
		}
		s.requireServiceEntryHost(createdName, originalHost)
	})

	s.Run("patches the ServiceEntry", func() {
		if createdName == "" {
			s.T().Skip("create step did not produce a resource name")
			return
		}

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
				"hosts": []string{updatedHost},
			},
		}
		dataBytes, err := json.Marshal(seData)
		s.Require().NoError(err)

		args := map[string]interface{}{
			"action":    "patch",
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

		message := s.requireJSONMessage(tools.KialiManageIstioConfigEndpoint, body)
		s.Contains(message, "Successfully patched ServiceEntry")
		s.Contains(message, createdName)
	})

	s.Run("reads back the patched ServiceEntry", func() {
		if createdName == "" {
			s.T().Skip("create step did not produce a resource name")
			return
		}
		s.requireServiceEntryHost(createdName, updatedHost)
	})

	s.Run("previews delete for the ServiceEntry", func() {
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

		var payload struct {
			Result  string `json:"result"`
			Actions []struct {
				Operation string `json:"operation"`
				Object    string `json:"object"`
			} `json:"actions"`
		}
		s.Require().NoError(json.Unmarshal(body, &payload))
		s.Contains(payload.Result, "PREVIEW READY")
		s.Require().NotEmpty(payload.Actions)
		s.Equal("delete", payload.Actions[0].Operation)
		s.Equal(createdName, payload.Actions[0].Object)
	})

	s.Run("deletes the ServiceEntry after confirmation", func() {
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
			"confirmed": true,
		}
		resp, body, err := s.mcpCall(tools.KialiManageIstioConfigEndpoint, args)
		s.Require().NoError(err)
		s.requireSuccess(tools.KialiManageIstioConfigEndpoint, resp, body)

		message := s.requireJSONMessage(tools.KialiManageIstioConfigEndpoint, body)
		s.Contains(message, "Successfully deleted ServiceEntry")
		s.Contains(message, createdName)
	})

	s.Run("confirms the ServiceEntry is gone", func() {
		if createdName == "" {
			s.T().Skip("create step did not produce a resource name")
			return
		}
		s.requireServiceEntryMissing(createdName)
	})
}

func (s *ContractTestSuite) TestListTraces() {
	s.Run("returns a valid trace list or a tracing-disabled error", func() {
		s.fetchTraceList(s.testService)
	})
}

func (s *ContractTestSuite) TestGetTraceDetails() {
	s.Run("returns a real trace when available, otherwise a precise error", func() {
		traceList, tracingAvailable := s.fetchTraceList(s.testService)

		traceID := "0000000000000001"
		expectSuccess := false
		if s.testTraceID != "" {
			traceID = s.testTraceID
			expectSuccess = true
		} else if tracingAvailable && traceList != nil && len(traceList.Traces) > 0 {
			traceID = traceList.Traces[0].ID
			expectSuccess = true
		}

		args := map[string]interface{}{
			"traceId": traceID,
		}
		resp, body, err := s.mcpCall(tools.KialiGetTraceDetailsEndpoint, args)
		s.Require().NoError(err)

		if expectSuccess {
			s.requireSuccess(tools.KialiGetTraceDetailsEndpoint, resp, body)

			var payload struct {
				TraceID string  `json:"trace_id"`
				TotalMS float64 `json:"total_ms"`
			}
			s.Require().NoError(json.Unmarshal(body, &payload))
			s.Equal(traceID, payload.TraceID)
			s.Greater(payload.TotalMS, float64(0))
			return
		}

		s.requireToolError(tools.KialiGetTraceDetailsEndpoint, resp, body, http.StatusNotFound, "trace not found", "tracing")
	})
}

func TestContract(t *testing.T) {
	suite.Run(t, new(ContractTestSuite))
}
