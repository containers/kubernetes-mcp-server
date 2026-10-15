//go:build netobserv_contract

package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ContractTestSuite struct {
	suite.Suite
	baseURL    string
	token      string
	namespace  string
	httpClient *http.Client
}

func (s *ContractTestSuite) SetupSuite() {
	s.baseURL = os.Getenv("NETOBSERV_URL")
	if s.baseURL == "" {
		s.baseURL = "http://localhost:9001"
	}
	s.token = os.Getenv("NETOBSERV_TOKEN")
	s.namespace = os.Getenv("TEST_NAMESPACE")
	if s.namespace == "" {
		s.namespace = "default"
	}
	s.httpClient = &http.Client{Timeout: 10 * time.Second}
}

func (s *ContractTestSuite) callEndpoint(method, path string, queryParams map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(method, s.baseURL+path, nil)
	if err != nil {
		return nil, err
	}

	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	if queryParams != nil {
		q := req.URL.Query()
		for k, v := range queryParams {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	return s.httpClient.Do(req)
}

// TestListFlowsEndpoint validates /api/loki/flow/records is registered
func (s *ContractTestSuite) TestListFlowsEndpoint() {
	resp, err := s.callEndpoint("GET", "/api/loki/flow/records", map[string]string{
		"namespace": s.namespace,
		"startTime": fmt.Sprintf("%d", time.Now().Add(-5*time.Minute).Unix()),
		"endTime":   fmt.Sprintf("%d", time.Now().Unix()),
	})
	s.NoError(err, "HTTP request should succeed")
	defer resp.Body.Close()

	s.NotEqual(404, resp.StatusCode, "/api/loki/flow/records should be registered")

	// If successful, validate response structure
	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		s.NoError(err)

		var result map[string]interface{}
		s.NoError(json.Unmarshal(body, &result), "response should be valid JSON")
		s.Contains(result, "result", "response should have 'result' field")
	}
}

// TestGetMetricsEndpoint validates /api/flow/metrics is registered
func (s *ContractTestSuite) TestGetMetricsEndpoint() {
	resp, err := s.callEndpoint("GET", "/api/flow/metrics", map[string]string{
		"aggregateBy": "namespace",
		"type":        "Bytes",
		"function":    "rate",
		"startTime":   fmt.Sprintf("%d", time.Now().Add(-5*time.Minute).Unix()),
		"endTime":     fmt.Sprintf("%d", time.Now().Unix()),
	})
	if !s.NoError(err, "HTTP request should succeed") {
		return
	}
	defer resp.Body.Close()

	if !s.Equal(http.StatusOK, resp.StatusCode, "/api/flow/metrics should accept a valid metrics query") {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if !s.NoError(err) {
		return
	}

	var result map[string]interface{}
	if !s.NoError(json.Unmarshal(body, &result), "response should be valid JSON") {
		return
	}
	s.Equal("success", result["status"], "response should report success")
	s.Contains(result, "data", "response should have 'data' field")
}

// TestExportFlowsEndpoint validates /api/loki/export is registered
func (s *ContractTestSuite) TestExportFlowsEndpoint() {
	resp, err := s.callEndpoint("GET", "/api/loki/export", map[string]string{
		"namespace": s.namespace,
		"startTime": fmt.Sprintf("%d", time.Now().Add(-5*time.Minute).Unix()),
		"endTime":   fmt.Sprintf("%d", time.Now().Unix()),
	})
	s.NoError(err, "HTTP request should succeed")
	defer resp.Body.Close()

	s.NotEqual(404, resp.StatusCode, "/api/loki/export should be registered")

	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		s.NoError(err)
		bodyStr := string(body)
		s.Contains(bodyStr, "TimeFlowStartMs", "CSV should have expected headers")
	}
}

// TestStatusEndpoint validates /api/status endpoint
func (s *ContractTestSuite) TestStatusEndpoint() {
	resp, err := s.callEndpoint("GET", "/api/status", nil)
	s.NoError(err, "HTTP request should succeed")
	defer resp.Body.Close()

	s.Equal(200, resp.StatusCode, "/api/status should return 200")

	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		s.NoError(err)

		var status map[string]interface{}
		s.NoError(json.Unmarshal(body, &status), "response should be valid JSON")
		s.Contains(status, "loki", "response should have 'loki' field")
		s.Contains(status, "prometheus", "response should have 'prometheus' field")
	}
}

// TestErrorHandling validates plugin returns proper errors
func (s *ContractTestSuite) TestErrorHandling() {
	// Invalid time range should return error (not 500)
	resp, err := s.callEndpoint("GET", "/api/loki/flow/records", map[string]string{
		"startTime": "invalid",
		"endTime":   "invalid",
	})
	s.NoError(err)
	defer resp.Body.Close()

	// Should return 4xx (client error), not 500
	s.True(resp.StatusCode >= 400 && resp.StatusCode < 500,
		"invalid params should return 4xx, got %d", resp.StatusCode)
}

func TestContractSuite(t *testing.T) {
	suite.Run(t, new(ContractTestSuite))
}
