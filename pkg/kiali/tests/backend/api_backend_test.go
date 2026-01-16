//go:build kiali_contract
// +build kiali_contract

package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/stretchr/testify/suite"
)

// ContractTestSuite tests the contract/interface of Kiali API endpoints
// that are used by the kubernetes-mcp-server.
type ContractTestSuite struct {
	suite.Suite
	kialiURL     string
	kialiToken   string
	httpClient   *http.Client
	testNS       string
	testService  string
	testWorkload string
	testApp      string
	testPod      string
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
	s.testApp = os.Getenv("TEST_APP")
	if s.testApp == "" {
		s.testApp = "productpage"
	}
	s.testPod = os.Getenv("TEST_POD")

	s.httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}
}

// apiCall makes an HTTP request to the Kiali API and returns the response
func (s *ContractTestSuite) apiCall(method, endpoint string, body []byte) (*http.Response, []byte, error) {
	fullURL := s.kialiURL + endpoint
	req, err := http.NewRequest(method, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}

	if s.kialiToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.kialiToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

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

	// Log errors for debugging
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		basePath := strings.Split(endpoint, "?")[0]
		s.T().Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		s.T().Logf("❌ FAILED REQUEST: %s %s", method, basePath)
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

// formatEndpoint formats an endpoint with parameters (similar to Go's fmt.Sprintf)
func formatEndpoint(endpoint string, args ...string) string {
	result := endpoint
	for _, arg := range args {
		result = strings.Replace(result, "%s", url.PathEscape(arg), 1)
	}
	return result
}

func (s *ContractTestSuite) TestAuthInfo() {
	s.Run("returns auth info with expected structure", func() {
		endpoint := kiali.AuthInfoEndpoint
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.Contains(data, "strategy")
	})
}

func (s *ContractTestSuite) TestNamespaces() {
	s.Run("returns list of namespaces", func() {
		endpoint := kiali.NamespacesEndpoint
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data []interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		if len(data) > 0 {
			firstNS, ok := data[0].(map[string]interface{})
			s.True(ok)
			s.Contains(firstNS, "name")
		}
	})
}

func (s *ContractTestSuite) TestMeshGraph() {
	s.Run("returns mesh graph status", func() {
		endpoint := kiali.MeshGraphEndpoint + "?includeGateways=true&includeWaypoints=true"
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestGraph() {
	s.Run("returns namespace graph with expected structure", func() {
		endpoint := fmt.Sprintf("%s?namespaces=%s&duration=10m&graphType=versionedApp", kiali.GraphEndpoint, s.testNS)
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.Contains(data, "timestamp")
	})
}

func (s *ContractTestSuite) TestHealth() {
	s.Run("returns health status for clusters", func() {
		endpoint := fmt.Sprintf("%s?namespaces=%s&type=app&rateInterval=10m", kiali.HealthEndpoint, s.testNS)
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestIstioConfig() {
	s.Run("returns Istio configuration list", func() {
		endpoint := kiali.IstioConfigEndpoint + "?validate=true"
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestValidations() {
	s.Run("returns validations list", func() {
		endpoint := fmt.Sprintf("%s?namespaces=%s", kiali.ValidationsEndpoint, s.testNS)
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		// Validations endpoint can return either an array or an object
		var data interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestServices() {
	s.Run("returns services list with expected structure", func() {
		endpoint := kiali.ServicesEndpoint + "?health=true&istioResources=true&rateInterval=10m&onlyDefinitions=false"
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		// Services endpoint returns an object or array
		var data interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestServiceDetails() {
	s.Run("returns service details with expected structure", func() {
		endpoint := formatEndpoint(kiali.ServiceDetailsEndpoint, s.testNS, s.testService) + "?validate=true&rateInterval=10m"
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.Contains(data, "service")
		service, ok := data["service"].(map[string]interface{})
		s.True(ok)
		s.Contains(service, "name")
		s.Contains(service, "namespace")
	})
}

func (s *ContractTestSuite) TestServiceMetrics() {
	s.Run("returns service metrics with expected structure", func() {
		endpoint := formatEndpoint(kiali.ServiceMetricsEndpoint, s.testNS, s.testService) + "?duration=10&step=15"
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestWorkloads() {
	s.Run("returns workloads list with expected structure", func() {
		endpoint := kiali.WorkloadsEndpoint + "?health=true&istioResources=true&rateInterval=10m"
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestWorkloadDetails() {
	s.Run("returns workload details with expected structure", func() {
		if s.testWorkload == "" {
			s.T().Skip("TEST_WORKLOAD not set, skipping workload details test")
			return
		}

		endpoint := formatEndpoint(kiali.WorkloadDetailsEndpoint, s.testNS, s.testWorkload) + "?validate=true&rateInterval=10m&health=true"
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.Contains(data, "name")
		s.Contains(data, "namespace")
	})
}

func (s *ContractTestSuite) TestWorkloadMetrics() {
	s.Run("returns workload metrics with expected structure", func() {
		if s.testWorkload == "" {
			s.T().Skip("TEST_WORKLOAD not set, skipping workload metrics test")
			return
		}

		endpoint := formatEndpoint(kiali.WorkloadMetricsEndpoint, s.testNS, s.testWorkload) + "?duration=10&step=15"
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestPodDetails() {
	s.Run("returns pod details with expected structure", func() {
		// First, get workload details to extract a pod name
		var podName string
		if s.testPod != "" {
			podName = s.testPod
		} else if s.testWorkload != "" {
			// Get workload details to find a pod
			workloadEndpoint := formatEndpoint(kiali.WorkloadDetailsEndpoint, s.testNS, s.testWorkload) + "?validate=true&rateInterval=10m&health=true"
			resp, body, err := s.apiCall(http.MethodGet, workloadEndpoint, nil)
			if err == nil && resp.StatusCode == http.StatusOK {
				var workloadData map[string]interface{}
				if err := json.Unmarshal(body, &workloadData); err == nil {
					if pods, ok := workloadData["pods"].([]interface{}); ok && len(pods) > 0 {
						if firstPod, ok := pods[0].(map[string]interface{}); ok {
							if name, ok := firstPod["name"].(string); ok && name != "" {
								podName = name
							}
						}
					}
				}
			}
		}

		endpoint := formatEndpoint(kiali.PodDetailsEndpoint, s.testNS, podName)
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.Contains(data, "name", "Pod details should contain 'name' field")
		// Note: namespace may not be in the response, but name and other fields should be present
		s.NotEmpty(data["name"], "Pod name should not be empty")
	})
}

func (s *ContractTestSuite) TestPodLogs() {
	s.Run("returns pod logs", func() {
		// First, get workload details to extract a pod name
		var podName string
		if s.testPod != "" {
			podName = s.testPod
		} else if s.testWorkload != "" {
			// Get workload details to find a pod
			workloadEndpoint := formatEndpoint(kiali.WorkloadDetailsEndpoint, s.testNS, s.testWorkload) + "?validate=true&rateInterval=10m&health=true"
			resp, body, err := s.apiCall(http.MethodGet, workloadEndpoint, nil)
			if err == nil && resp.StatusCode == http.StatusOK {
				var workloadData map[string]interface{}
				if err := json.Unmarshal(body, &workloadData); err == nil {
					if pods, ok := workloadData["pods"].([]interface{}); ok && len(pods) > 0 {
						if firstPod, ok := pods[0].(map[string]interface{}); ok {
							if name, ok := firstPod["name"].(string); ok && name != "" {
								podName = name
							}
						}
					}
				}
			}
		}

		endpoint := formatEndpoint(kiali.PodsLogsEndpoint, s.testNS, podName) + "?container=istio-proxy"
		resp, _, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)

		// Logs endpoint may return 200, 204, or 404
		if resp.StatusCode == http.StatusNotFound {
			s.T().Skip("Pod not found, skipping logs test")
			return
		}
		s.True(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent,
			"Expected status 200 or 204 but got %d for %s", resp.StatusCode, endpoint)
	})
}

func (s *ContractTestSuite) TestAppTraces() {
	s.Run("returns app traces with expected structure", func() {
		if s.testApp == "" {
			s.T().Skip("TEST_APP not set, skipping app traces test")
			return
		}

		tenMinutesAgo := time.Now().Add(-10 * time.Minute)
		startMicros := tenMinutesAgo.UnixMicro()
		tags := url.QueryEscape("{}")
		endpoint := fmt.Sprintf("%s?startMicros=%d&tags=%s&limit=2",
			formatEndpoint(kiali.AppTracesEndpoint, s.testNS, s.testApp), startMicros, tags)
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestServiceTraces() {
	s.Run("returns service traces with expected structure", func() {
		tenMinutesAgo := time.Now().Add(-10 * time.Minute)
		startMicros := tenMinutesAgo.UnixMicro()
		tags := url.QueryEscape("{}")
		endpoint := fmt.Sprintf("%s?startMicros=%d&tags=%s&limit=2",
			formatEndpoint(kiali.ServiceTracesEndpoint, s.testNS, s.testService), startMicros, tags)
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestWorkloadTraces() {
	s.Run("returns workload traces with expected structure", func() {
		if s.testWorkload == "" {
			s.T().Skip("TEST_WORKLOAD not set, skipping workload traces test")
			return
		}

		tenMinutesAgo := time.Now().Add(-10 * time.Minute)
		startMicros := tenMinutesAgo.UnixMicro()
		tags := url.QueryEscape("{}")
		endpoint := fmt.Sprintf("%s?startMicros=%d&tags=%s&limit=2",
			formatEndpoint(kiali.WorkloadTracesEndpoint, s.testNS, s.testWorkload), startMicros, tags)
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestIstioObjectDetails() {
	s.Run("returns Istio object details with expected structure", func() {
		endpoint := formatEndpoint(kiali.IstioObjectEndpoint, s.testNS, "gateway.networking.k8s.io", "v1", "Gateway", "bookinfo-gateway") + "?validate=true&help=true"
		resp, body, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(body, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func (s *ContractTestSuite) TestIstioObjectCRUD() {
	// Create, Patch, and Delete in sequence to ensure proper order
	var createdName string

	s.Run("creates Istio object", func() {
		uniqueName := fmt.Sprintf("service-%d", time.Now().UnixMilli())
		createdName = uniqueName
		endpoint := formatEndpoint(kiali.IstioObjectCreateEndpoint, s.testNS, "networking.istio.io", "v1", "ServiceEntry")

		testResource := map[string]interface{}{
			"apiVersion": "networking.istio.io/v1",
			"kind":       "ServiceEntry",
			"metadata": map[string]interface{}{
				"name":        uniqueName,
				"namespace":   s.testNS,
				"labels":      map[string]interface{}{},
				"annotations": map[string]interface{}{},
			},
			"spec": map[string]interface{}{
				"location":   "MESH_EXTERNAL",
				"resolution": "NONE",
				"ports": []map[string]interface{}{
					{
						"name":     "default",
						"protocol": "HTTP",
						"number":   80,
					},
				},
				"hosts": []string{"service.com"},
			},
		}

		bodyBytes, err := json.Marshal(testResource)
		s.Require().NoError(err)

		resp, respBody, err := s.apiCall(http.MethodPost, endpoint, bodyBytes)
		s.Require().NoError(err)
		s.True(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated,
			"Expected status 200 or 201 but got %d for %s", resp.StatusCode, endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(respBody, &data)
		s.NoError(err)
		if metadata, ok := data["metadata"].(map[string]interface{}); ok {
			if name, ok := metadata["name"].(string); ok {
				createdName = name
			}
		}

		// Wait for the resource to be available before proceeding
		s.waitForResource(createdName)
	})

	s.Run("updates Istio object", func() {
		if createdName == "" {
			s.T().Skip("No ServiceEntry was created, skipping PATCH test")
			return
		}

		// Ensure resource is available before patching
		s.waitForResource(createdName)

		endpoint := formatEndpoint(kiali.IstioObjectEndpoint, s.testNS, "networking.istio.io", "v1", "ServiceEntry", createdName)
		patchData := map[string]interface{}{
			"spec": map[string]interface{}{
				"hosts": []string{"service2.com"},
			},
		}

		bodyBytes, err := json.Marshal(patchData)
		s.Require().NoError(err)

		resp, respBody, err := s.apiCall(http.MethodPatch, endpoint, bodyBytes)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(respBody, &data)
		s.NoError(err)
		s.NotNil(data)
	})

	s.Run("deletes Istio object", func() {
		if createdName == "" {
			s.T().Skip("No ServiceEntry was created, skipping DELETE test")
			return
		}

		endpoint := formatEndpoint(kiali.IstioObjectEndpoint, s.testNS, "networking.istio.io", "v1", "ServiceEntry", createdName)
		resp, _, err := s.apiCall(http.MethodDelete, endpoint, nil)
		s.Require().NoError(err)
		s.True(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent,
			"Expected status 200 or 204 but got %d for %s", resp.StatusCode, endpoint)
	})
}

// waitForResource waits for a ServiceEntry to be available in Kiali
func (s *ContractTestSuite) waitForResource(name string) {
	maxRetries := 30
	retryDelay := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		endpoint := formatEndpoint(kiali.IstioObjectEndpoint, s.testNS, "networking.istio.io", "v1", "ServiceEntry", name)
		resp, _, err := s.apiCall(http.MethodGet, endpoint, nil)
		if err == nil && resp.StatusCode == http.StatusOK {
			return // Resource is available
		}
		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}
	s.T().Logf("Warning: Resource %s may not be fully available yet after %d retries", name, maxRetries)
}

func (s *ContractTestSuite) TestTraceDetails() {
	s.Run("returns trace details with expected structure", func() {
		// First, get a trace from service traces endpoint to obtain a valid traceId
		tenMinutesAgo := time.Now().Add(-10 * time.Minute)
		startMicros := tenMinutesAgo.UnixMicro()
		tags := url.QueryEscape("{}")
		tracesEndpoint := fmt.Sprintf("%s?startMicros=%d&tags=%s&limit=2",
			formatEndpoint(kiali.ServiceTracesEndpoint, s.testNS, s.testService), startMicros, tags)

		resp, body, err := s.apiCall(http.MethodGet, tracesEndpoint, nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			s.T().Skip("Cannot get traces list, skipping trace details test")
			return
		}

		// Extract traceId from the traces response
		var tracesData map[string]interface{}
		err = json.Unmarshal(body, &tracesData)
		if err != nil {
			s.T().Skip("Invalid traces response, skipping trace details test")
			return
		}

		var traceID string
		if dataArray, ok := tracesData["data"].([]interface{}); ok && len(dataArray) > 0 {
			if firstTrace, ok := dataArray[0].(map[string]interface{}); ok {
				if id, ok := firstTrace["traceID"].(string); ok && id != "" {
					traceID = id
				}
			}
		}

		// Try workload traces as fallback
		if traceID == "" && s.testWorkload != "" {
			workloadTracesEndpoint := fmt.Sprintf("%s?startMicros=%d&tags=%s&limit=2",
				formatEndpoint(kiali.WorkloadTracesEndpoint, s.testNS, s.testWorkload), startMicros, tags)
			resp, body, err := s.apiCall(http.MethodGet, workloadTracesEndpoint, nil)
			if err == nil && resp.StatusCode == http.StatusOK {
				var tracesData map[string]interface{}
				if err := json.Unmarshal(body, &tracesData); err == nil {
					if dataArray, ok := tracesData["data"].([]interface{}); ok && len(dataArray) > 0 {
						if firstTrace, ok := dataArray[0].(map[string]interface{}); ok {
							if id, ok := firstTrace["traceID"].(string); ok && id != "" {
								traceID = id
							}
						}
					}
				}
			}
		}

		// Try app traces as fallback
		if traceID == "" && s.testApp != "" {
			appTracesEndpoint := fmt.Sprintf("%s?startMicros=%d&tags=%s&limit=2",
				formatEndpoint(kiali.AppTracesEndpoint, s.testNS, s.testApp), startMicros, tags)
			resp, body, err := s.apiCall(http.MethodGet, appTracesEndpoint, nil)
			if err == nil && resp.StatusCode == http.StatusOK {
				var tracesData map[string]interface{}
				if err := json.Unmarshal(body, &tracesData); err == nil {
					if dataArray, ok := tracesData["data"].([]interface{}); ok && len(dataArray) > 0 {
						if firstTrace, ok := dataArray[0].(map[string]interface{}); ok {
							if id, ok := firstTrace["traceID"].(string); ok && id != "" {
								traceID = id
							}
						}
					}
				}
			}
		}

		if traceID == "" {
			s.T().Skip("No valid traceId found in traces response, skipping trace details test")
			return
		}

		// Now use the traceId to get trace details
		endpoint := fmt.Sprintf("/api/traces/%s", url.PathEscape(traceID))
		resp, respBody, err := s.apiCall(http.MethodGet, endpoint, nil)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode, "Expected status 200 for %s", endpoint)

		var data map[string]interface{}
		err = json.Unmarshal(respBody, &data)
		s.NoError(err)
		s.NotNil(data)
	})
}

func TestContract(t *testing.T) {
	suite.Run(t, new(ContractTestSuite))
}
