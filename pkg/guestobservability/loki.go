package guestobservability

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/tlsutil"
)

const (
	LokiQueryRangeEndpoint = "/loki/api/v1/query_range"

	DefaultLokiLimit     = 100
	MaxLokiLimit         = 1000
	DefaultLokiDirection = "backward"

	DefaultLokiLookback = time.Hour
	DefaultLokiTimeout  = 30 * time.Second

	maxLokiResponseBodySize = 4 << 20 // 4 MiB
)

var errLokiRedirectsNotAllowed = errors.New(
	"redirects are not allowed for Loki API requests",
)

// LokiQueryRangeRequest represents a model-facing Loki range query.
//
// Start and End are RFC3339 timestamps with an explicit timezone.
// They are normalized to UTC before being sent to Loki.
type LokiQueryRangeRequest struct {
	Query     string
	Start     string
	End       string
	Limit     int
	Direction string
	Step      string
}

// Loki is an HTTP client for the Loki HTTP API.
type Loki struct {
	baseURL              string
	tenant               string
	insecure             bool
	certificateAuthority string
	tlsMinVersion        string
	tlsCipherSuites      []string
	requireTLS           func() bool
}

// NewLoki creates a Loki client using guest-observability
// toolset configuration.
func NewLoki(
	configProvider api.BaseConfig,
) (*Loki, error) {
	if configProvider == nil {
		return nil, errors.New("configuration provider is required")
	}

	extended, ok := configProvider.GetToolsetConfig(ToolsetName)
	if !ok {
		return nil, errors.New(
			"guest-observability toolset configuration is required",
		)
	}

	guestConfig, ok := extended.(*Config)
	if !ok || guestConfig == nil {
		return nil, errors.New(
			"invalid guest-observability toolset configuration",
		)
	}

	return newLokiFromConfig(
		guestConfig,
		configProvider.GetTLSMinVersionConfig(),
		configProvider.GetTLSCipherSuitesConfig(),
		configProvider.IsRequireTLS,
	)
}

func newLokiFromConfig(
	guestConfig *Config,
	tlsMinVersion string,
	tlsCipherSuites []string,
	requireTLS func() bool,
) (*Loki, error) {
	if guestConfig == nil {
		return nil, errors.New("guest-observability config is nil")
	}

	if err := guestConfig.Loki.Validate(); err != nil {
		return nil, err
	}

	client := &Loki{
		baseURL:              strings.TrimRight(guestConfig.Loki.URL, "/"),
		tenant:               strings.TrimSpace(guestConfig.Loki.Tenant),
		insecure:             guestConfig.Loki.Insecure,
		certificateAuthority: guestConfig.Loki.CertificateAuthority,
		tlsMinVersion:        tlsMinVersion,
		tlsCipherSuites:      tlsCipherSuites,
		requireTLS:           requireTLS,
	}

	return client, nil
}

// QueryRange executes a bounded Loki query_range request.
func (l *Loki) QueryRange(
	ctx context.Context,
	request LokiQueryRangeRequest,
) (string, error) {
	logger := klogutil.FromContext(ctx)

	values, err := prepareLokiQueryRange(request, time.Now().UTC())
	if err != nil {
		return "", err
	}

	requestURL, err := l.queryRangeURL(values)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		requestURL,
		nil,
	)
	if err != nil {
		return "", err
	}
	if l.tenant != "" {
		req.Header.Set("X-Scope-OrgID", l.tenant)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Kubernetes-MCP-Server", "true")

	httpClient, err := l.createHTTPClient()
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.V(1).Info(
			"Loki request failed",
			"host", req.URL.Host,
			"error", err,
		)

		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf(
				"Loki query canceled or timed out: %w",
				err,
			)
		}

		return "", fmt.Errorf("Loki query failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(
		io.LimitReader(
			resp.Body,
			maxLokiResponseBodySize+1,
		),
	)
	if err != nil {
		logger.V(1).Info(
			"Failed to read Loki response",
			"host", req.URL.Host,
			"error", err,
		)

		return "", fmt.Errorf(
			"failed to read Loki response: %w",
			err,
		)
	}

	if len(body) > maxLokiResponseBodySize {
		logger.V(1).Info(
			"Loki response exceeded maximum allowed size",
			"host", req.URL.Host,
			"maximum_bytes", maxLokiResponseBodySize,
		)

		return "", fmt.Errorf(
			"Loki response exceeded maximum size of %d bytes; narrow the query time range or reduce the result limit",
			maxLokiResponseBodySize,
		)
	}

	if resp.StatusCode < http.StatusOK ||
		resp.StatusCode >= http.StatusMultipleChoices {
		logger.V(1).Info(
			"Loki API returned non-success status",
			"host", req.URL.Host,
			"status_code", resp.StatusCode,
		)

		return "", fmt.Errorf(
			"Loki API returned HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	return string(body), nil
}

func prepareLokiQueryRange(
	request LokiQueryRangeRequest,
	now time.Time,
) (url.Values, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, errors.New("Loki query is required")
	}

	start, end, err := resolveLokiTimeRange(
		request.Start,
		request.End,
		now,
	)
	if err != nil {
		return nil, err
	}

	limit := request.Limit
	if limit == 0 {
		limit = DefaultLokiLimit
	}

	if limit < 1 || limit > MaxLokiLimit {
		return nil, fmt.Errorf(
			"Loki limit must be between 1 and %d",
			MaxLokiLimit,
		)
	}

	direction := strings.ToLower(
		strings.TrimSpace(request.Direction),
	)
	if direction == "" {
		direction = DefaultLokiDirection
	}

	if direction != "forward" &&
		direction != "backward" {
		return nil, errors.New(
			"Loki direction must be forward or backward",
		)
	}

	values := url.Values{}

	values.Set("query", query)
	values.Set(
		"start",
		start.UTC().Format(time.RFC3339Nano),
	)
	values.Set(
		"end",
		end.UTC().Format(time.RFC3339Nano),
	)
	values.Set("limit", strconv.Itoa(limit))
	values.Set("direction", direction)

	if step := strings.TrimSpace(request.Step); step != "" {
		values.Set("step", step)
	}

	return values, nil
}

func resolveLokiTimeRange(
	startValue string,
	endValue string,
	now time.Time,
) (time.Time, time.Time, error) {
	now = now.UTC()

	var (
		start time.Time
		end   time.Time
		err   error
	)

	if strings.TrimSpace(endValue) == "" {
		end = now
	} else {
		end, err = parseLokiTimestamp(endValue)
		if err != nil {
			return time.Time{}, time.Time{},
				fmt.Errorf("invalid Loki end timestamp: %w", err)
		}
	}

	if strings.TrimSpace(startValue) == "" {
		start = end.Add(-DefaultLokiLookback)
	} else {
		start, err = parseLokiTimestamp(startValue)
		if err != nil {
			return time.Time{}, time.Time{},
				fmt.Errorf("invalid Loki start timestamp: %w", err)
		}
	}

	if !start.Before(end) {
		return time.Time{}, time.Time{},
			errors.New("Loki start timestamp must be before end timestamp")
	}

	return start.UTC(), end.UTC(), nil
}

func parseLokiTimestamp(value string) (time.Time, error) {
	return time.Parse(
		time.RFC3339Nano,
		strings.TrimSpace(value),
	)
}

func (l *Loki) queryRangeURL(
	values url.Values,
) (string, error) {
	if l == nil || strings.TrimSpace(l.baseURL) == "" {
		return "", errors.New("Loki client is not initialized")
	}

	baseURL, err := url.Parse(l.baseURL)
	if err != nil {
		return "", fmt.Errorf(
			"invalid Loki base URL: %w",
			err,
		)
	}

	requestURL, err := url.JoinPath(
		baseURL.String(),
		LokiQueryRangeEndpoint,
	)
	if err != nil {
		return "", fmt.Errorf(
			"failed to build Loki query URL: %w",
			err,
		)
	}

	u, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf(
			"failed to parse Loki query URL: %w",
			err,
		)
	}

	u.RawQuery = values.Encode()

	return u.String(), nil
}

func (l *Loki) createHTTPClient() (*http.Client, error) {
	var tlsOptions []tlsutil.TLSConfigOption

	if l.insecure {
		tlsOptions = append(
			tlsOptions,
			tlsutil.WithInsecureSkipVerify(true),
		)
	}

	if caValue := strings.TrimSpace(
		l.certificateAuthority,
	); caValue != "" {
		caPEM, err := os.ReadFile(caValue)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to read Loki certificate authority %q: %w",
				caValue,
				err,
			)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf(
				"failed to parse Loki certificate authority %q",
				caValue,
			)
		}

		tlsOptions = append(
			tlsOptions,
			tlsutil.WithRootCAs(certPool),
		)
	}

	tlsConfig, err := tlsutil.BuildTLSConfig(
		l.tlsMinVersion,
		l.tlsCipherSuites,
		tlsOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to build Loki TLS configuration: %w",
			err,
		)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:       tlsConfig,
			ResponseHeaderTimeout: DefaultLokiTimeout,
		},
		Timeout: DefaultLokiTimeout,
		CheckRedirect: func(
			_ *http.Request,
			_ []*http.Request,
		) error {
			return errLokiRedirectsNotAllowed
		},
	}

	if l.requireTLS != nil {
		client = config.NewTLSEnforcingClient(
			client,
			l.requireTLS,
		)
	}

	return client, nil
}
