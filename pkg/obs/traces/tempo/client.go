package tempo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	urlpkg "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type TempoClient struct {
	httpClient *http.Client
	baseURL    string
	limit      func() (int64, error)
}

const RequestTimeout = 30 * time.Second

func NewTempoClient(httpClient *http.Client, url string, limit func() (int64, error)) *TempoClient {
	if limit == nil {
		limit = kubernetes.LiveBackendLimit(nil)
	}
	return &TempoClient{
		httpClient: httpClient,
		baseURL:    url,
		limit:      limit,
	}
}

func (c *TempoClient) backendLimit() (int64, error) {
	if c == nil || c.limit == nil {
		return kubernetes.BackendLimit(nil)
	}
	return c.limit()
}

func (c *TempoClient) doRequest(req *http.Request) (body string, err error) {
	limit, err := c.backendLimit()
	if err != nil {
		return "", err
	}

	// Use LLM-friendly format
	req.Header.Set("Accept", "application/vnd.grafana.llm")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	bodyBytes, err := kubernetes.ReadCapped(resp.Body, limit)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	// observatorium/api redirects to OIDC page if token is missing or invalid
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		return "", fmt.Errorf("invalid authentication token")
	}

	return string(bodyBytes), nil
}

type QueryV2Options struct {
	Start int64 // Unix epoch seconds
	End   int64 // Unix epoch seconds
}

func (c *TempoClient) QueryV2(ctx context.Context, traceID string, opts QueryV2Options) (string, error) {
	url := fmt.Sprintf("%s/api/v2/traces/%s", c.baseURL, urlpkg.PathEscape(traceID))
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	if opts.Start != 0 {
		q.Add("start", strconv.FormatInt(opts.Start, 10))
	}
	if opts.End != 0 {
		q.Add("end", strconv.FormatInt(opts.End, 10))
	}
	req.URL.RawQuery = q.Encode()

	return c.doRequest(req)
}

type SearchOptions struct {
	Query string // TraceQL query
	Limit int    // Maximum search results (default: 20)
	Start int64  // Unix epoch seconds
	End   int64  // Unix epoch seconds
	Spss  int    // Spans per span-set limit (default: 3)
}

func (c *TempoClient) Search(ctx context.Context, opts SearchOptions) (string, error) {
	url := fmt.Sprintf("%s/api/search", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	if opts.Query != "" {
		q.Add("q", opts.Query)
	}
	if opts.Limit != 0 {
		q.Add("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Start != 0 {
		q.Add("start", strconv.FormatInt(opts.Start, 10))
	}
	if opts.End != 0 {
		q.Add("end", strconv.FormatInt(opts.End, 10))
	}
	if opts.Spss != 0 {
		q.Add("spss", strconv.Itoa(opts.Spss))
	}
	req.URL.RawQuery = q.Encode()

	return c.doRequest(req)
}

type SearchTagsV2Options struct {
	Scope          string // resource, span, intrinsic, event, link, or instrumentation
	Query          string // TraceQL query for filtering tag names
	Start          int64  // Unix epoch seconds
	End            int64  // Unix epoch seconds
	Limit          int    // Maximum number of tag names per scope
	MaxStaleValues int    // Search termination threshold for stale values
}

func (c *TempoClient) SearchTagsV2(ctx context.Context, opts SearchTagsV2Options) (string, error) {
	url := fmt.Sprintf("%s/api/v2/search/tags", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	if opts.Scope != "" {
		q.Add("scope", opts.Scope)
	}
	if opts.Query != "" {
		q.Add("q", opts.Query)
	}
	if opts.Start != 0 {
		q.Add("start", strconv.FormatInt(opts.Start, 10))
	}
	if opts.End != 0 {
		q.Add("end", strconv.FormatInt(opts.End, 10))
	}
	if opts.Limit != 0 {
		q.Add("limit", strconv.Itoa(opts.Limit))
	}
	if opts.MaxStaleValues != 0 {
		q.Add("maxStaleValues", strconv.Itoa(opts.MaxStaleValues))
	}
	req.URL.RawQuery = q.Encode()

	return c.doRequest(req)
}

type SearchTagValuesV2Options struct {
	Query          string // TraceQL query for filtering tag values
	Start          int64  // Unix epoch seconds
	End            int64  // Unix epoch seconds
	Limit          int    // Maximum number of tag values to return
	MaxStaleValues int    // Search termination threshold for stale values
}

func (c *TempoClient) SearchTagValuesV2(ctx context.Context, tag string, opts SearchTagValuesV2Options) (string, error) {
	url := fmt.Sprintf("%s/api/v2/search/tag/%s/values", c.baseURL, urlpkg.PathEscape(tag))
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	if opts.Query != "" {
		q.Add("q", opts.Query)
	}
	if opts.Start != 0 {
		q.Add("start", strconv.FormatInt(opts.Start, 10))
	}
	if opts.End != 0 {
		q.Add("end", strconv.FormatInt(opts.End, 10))
	}
	if opts.Limit != 0 {
		q.Add("limit", strconv.Itoa(opts.Limit))
	}
	if opts.MaxStaleValues != 0 {
		q.Add("maxStaleValues", strconv.Itoa(opts.MaxStaleValues))
	}
	req.URL.RawQuery = q.Encode()

	return c.doRequest(req)
}
