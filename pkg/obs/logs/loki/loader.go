package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	promapi "github.com/prometheus/client_golang/api"
)

const (
	labelsEndpoint      = "/loki/api/v1/labels"
	labelValuesEndpoint = "/loki/api/v1/label/%s/values"
	queryRangeEndpoint  = "/loki/api/v1/query_range"

	requestTimeout  = 60 * time.Second
	defaultLimit    = 100
	maxErrBodyBytes = 64 * 1024
)

// Loader defines the interface for querying Loki.
type Loader interface {
	LabelNames(ctx context.Context, start, end time.Time) ([]string, error)
	LabelValues(ctx context.Context, label string, start, end time.Time) ([]string, error)
	QueryRange(ctx context.Context, input QueryRangeInput) (QueryRangeResult, error)
}

// QueryRangeInput is the query payload for Loki /query_range.
type QueryRangeInput struct {
	Query     string
	Start     time.Time
	End       time.Time
	Limit     int
	Direction string
}

// Entry is a log line with timestamp.
type Entry struct {
	Timestamp string `json:"timestamp"`
	Line      string `json:"line"`
}

// Stream is a Loki stream with labels and entries.
type Stream struct {
	Labels  map[string]string `json:"labels"`
	Entries []Entry           `json:"entries"`
}

// QueryRangeResult is the normalized Loki query_range result.
type QueryRangeResult struct {
	ResultType string   `json:"resultType"`
	Streams    []Stream `json:"streams"`
}

// RealLoader is a real Loki loader implementation.
type RealLoader struct {
	baseURL string
	tenant  string
	client  *http.Client
}

var _ Loader = (*RealLoader)(nil)

func NewLoader(apiConfig promapi.Config, tenant string) (*RealLoader, error) {
	if strings.TrimSpace(apiConfig.Address) == "" {
		return nil, fmt.Errorf("loki URL is required")
	}

	baseURL := strings.TrimSuffix(apiConfig.Address, "/")
	httpClient := &http.Client{
		Timeout: requestTimeout,
	}
	if apiConfig.RoundTripper != nil {
		httpClient.Transport = apiConfig.RoundTripper
	}

	return &RealLoader{
		baseURL: baseURL,
		tenant:  strings.TrimSpace(tenant),
		client:  httpClient,
	}, nil
}

func (l *RealLoader) LabelNames(ctx context.Context, start, end time.Time) ([]string, error) {
	endpoint := labelsEndpoint
	params := buildTimeParams(start, end)

	var response struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
		Error  string   `json:"error"`
	}
	if err := l.getJSON(ctx, endpoint, params, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("loki labels request failed: %s", response.Error)
	}
	return response.Data, nil
}

func (l *RealLoader) LabelValues(ctx context.Context, label string, start, end time.Time) ([]string, error) {
	if strings.TrimSpace(label) == "" {
		return nil, fmt.Errorf("label is required")
	}
	endpoint := fmt.Sprintf(labelValuesEndpoint, url.PathEscape(label))
	params := buildTimeParams(start, end)

	var response struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
		Error  string   `json:"error"`
	}
	if err := l.getJSON(ctx, endpoint, params, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("loki label values request failed: %s", response.Error)
	}
	return response.Data, nil
}

func (l *RealLoader) QueryRange(ctx context.Context, input QueryRangeInput) (QueryRangeResult, error) {
	params := buildTimeParams(input.Start, input.End)
	params.Set("query", input.Query)
	params.Set("direction", input.Direction)
	params.Set("limit", strconv.Itoa(input.Limit))

	var response struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			} `json:"result"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := l.getJSON(ctx, queryRangeEndpoint, params, &response); err != nil {
		return QueryRangeResult{}, err
	}
	if response.Status != "success" {
		return QueryRangeResult{}, fmt.Errorf("loki query_range request failed: %s", response.Error)
	}

	streams := make([]Stream, 0, len(response.Data.Result))
	for _, result := range response.Data.Result {
		entries := make([]Entry, 0, len(result.Values))
		for _, raw := range result.Values {
			if len(raw) < 2 {
				continue
			}
			entries = append(entries, Entry{
				Timestamp: raw[0],
				Line:      raw[1],
			})
		}
		streams = append(streams, Stream{
			Labels:  result.Stream,
			Entries: entries,
		})
	}

	return QueryRangeResult{
		ResultType: response.Data.ResultType,
		Streams:    streams,
	}, nil
}

func buildTimeParams(start, end time.Time) url.Values {
	params := url.Values{}
	if !start.IsZero() {
		params.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	}
	if !end.IsZero() {
		params.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	}
	return params
}

func (l *RealLoader) requestURL(endpoint string) string {
	// Gateway URL ending with /api/logs/v1 — insert tenant in path
	// (used by OpenShift Loki gateway with tenant-based path routing)
	if strings.HasSuffix(l.baseURL, "/api/logs/v1") && l.tenant != "" {
		return l.baseURL + "/" + url.PathEscape(l.tenant) + endpoint
	}
	// URL already includes /api/logs/v1/<tenant>, or standard Loki URL
	// (for standard Loki, tenant is sent via X-Scope-OrgID header)
	return l.baseURL + endpoint
}

func (l *RealLoader) getJSON(ctx context.Context, endpoint string, params url.Values, output any) error {
	u := l.requestURL(endpoint)
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	if l.tenant != "" {
		req.Header.Set("X-Scope-OrgID", l.tenant)
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("loki request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	slog.Debug("Backend call completed",
		"backend", "loki",
		"endpoint", endpoint,
		"status_code", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes))
		if err != nil {
			return fmt.Errorf("loki request failed with status %d: failed to read response body: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("loki request failed with status %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
		return fmt.Errorf("failed to decode loki response: %w", err)
	}
	return nil
}
