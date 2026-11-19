package alertmanagement

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

const (
	requestTimeout   = 30 * time.Second
	maxResponseSize  = 10 * 1024 * 1024
	clientMetricName = "alert-management"
)

// APIError is a non-2xx response from the monitoring-plugin management API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("management API returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("management API returned HTTP %d: %s", e.StatusCode, e.Message)
}

type managementClient struct {
	httpClient *http.Client
	baseURL    string
}

func newManagementClient(httpClient *http.Client, baseURL string) *managementClient {
	return &managementClient{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func pathWithQuery(path string, query urlpkg.Values) string {
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func (c *managementClient) listAlerts(ctx context.Context, query urlpkg.Values) (prometheusAlertsResponse, error) {
	var out prometheusAlertsResponse
	err := c.doJSON(ctx, http.MethodGet, pathWithQuery(alertsAPIPath, query), nil, http.StatusOK, &out)
	return out, err
}

func (c *managementClient) listRules(ctx context.Context, query urlpkg.Values) (prometheusRulesResponse, error) {
	var out prometheusRulesResponse
	err := c.doJSON(ctx, http.MethodGet, pathWithQuery(rulesAPIPath, query), nil, http.StatusOK, &out)
	return out, err
}

func (c *managementClient) createRule(ctx context.Context, body createAPIRequest) (createAPIResponse, error) {
	var out createAPIResponse
	err := c.doJSON(ctx, http.MethodPost, rulesAPIPath, body, http.StatusCreated, &out)
	return out, err
}

func (c *managementClient) updateRules(ctx context.Context, body any) (bulkMutationAPIResponse, error) {
	var out bulkMutationAPIResponse
	err := c.doJSON(ctx, http.MethodPatch, rulesAPIPath, body, http.StatusOK, &out)
	return out, err
}

func (c *managementClient) deleteRules(ctx context.Context, ruleIDs []string) (bulkMutationAPIResponse, error) {
	var out bulkMutationAPIResponse
	err := c.doJSON(ctx, http.MethodDelete, rulesAPIPath, map[string]any{"ruleIds": ruleIDs}, http.StatusOK, &out)
	return out, err
}

func (c *managementClient) previewRule(ctx context.Context, body map[string]any) (map[string]any, error) {
	var out map[string]any
	err := c.doJSON(ctx, http.MethodPost, previewAPIPath, body, http.StatusOK, &out)
	return out, err
}

func (c *managementClient) doJSON(ctx context.Context, method, path string, body any, wantStatus int, dest any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			klogutil.FromContext(ctx).Error(err, "Failed to close management API response")
		}
	}()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if int64(len(raw)) > maxResponseSize {
		return fmt.Errorf("response exceeds maximum size of %d bytes", maxResponseSize)
	}

	if resp.StatusCode != wantStatus {
		return parseAPIError(resp.StatusCode, raw)
	}
	if dest == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func parseAPIError(statusCode int, raw []byte) error {
	msg := strings.TrimSpace(string(raw))
	var parsed errorAPIResponse
	if err := json.Unmarshal(raw, &parsed); err == nil && parsed.Error != "" {
		msg = parsed.Error
	}
	if msg == "" {
		msg = http.StatusText(statusCode)
	}
	return &APIError{StatusCode: statusCode, Message: msg}
}
