package alertmanager

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/prometheus/alertmanager/api/v2/client"
	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/client_golang/api"

	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/auth"
)

// Loader defines the interface for querying Alertmanager
type Loader interface {
	GetAlerts(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filter []string, receiver string) (models.GettableAlerts, error)
	GetSilences(ctx context.Context, filter []string) (models.GettableSilences, error)
	GetSilence(ctx context.Context, id string) (*models.GettableSilence, error)
	PostSilence(ctx context.Context, silence *models.PostableSilence) (string, error)
	DeleteSilence(ctx context.Context, id string) error
}

// RealLoader implements Loader
type RealLoader struct {
	client *client.AlertmanagerAPI
}

// Ensure RealLoader implements Loader at compile time
var _ Loader = (*RealLoader)(nil)

func NewAlertmanagerClient(apiConfig api.Config) (*RealLoader, error) {
	parsedURL, err := url.Parse(apiConfig.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Alertmanager URL: %w", err)
	}

	host := parsedURL.Host
	if host == "" {
		host = strings.TrimPrefix(apiConfig.Address, "//")
	}

	scheme := parsedURL.Scheme
	if scheme == "" {
		scheme = "http"
	}

	cfg := client.DefaultTransportConfig().
		WithHost(host).
		WithSchemes([]string{scheme})

	rt := apiConfig.RoundTripper
	if rt == nil {
		rt = http.DefaultTransport
	}
	httpClient := auth.NewHTTPClient(rt, 0)
	transport := httptransport.NewWithClient(cfg.Host, cfg.BasePath, cfg.Schemes, httpClient)
	c := client.New(transport, nil)

	return &RealLoader{
		client: c,
	}, nil
}

func (a *RealLoader) GetAlerts(ctx context.Context, active, silenced, inhibited, unprocessed *bool, filter []string, receiver string) (models.GettableAlerts, error) {
	params := alert.NewGetAlertsParams().WithContext(ctx)

	if active != nil {
		params = params.WithActive(active)
	}
	if silenced != nil {
		params = params.WithSilenced(silenced)
	}
	if inhibited != nil {
		params = params.WithInhibited(inhibited)
	}
	if unprocessed != nil {
		params = params.WithUnprocessed(unprocessed)
	}
	if len(filter) > 0 {
		params = params.WithFilter(filter)
	}
	if receiver != "" {
		params = params.WithReceiver(&receiver)
	}

	start := time.Now()
	resp, err := a.client.Alert.GetAlerts(params)
	duration := time.Since(start)
	if err != nil {
		klogutil.FromContext(ctx).Error(err, "Backend call failed", "backend", "alertmanager", "operation", "alerts",
			"duration_ms", duration.Milliseconds())
		return nil, fmt.Errorf("error fetching alerts: %w", err)
	}
	klogutil.FromContext(ctx).V(4).Info("Backend call completed", "backend", "alertmanager", "operation", "alerts",
		"duration_ms", duration.Milliseconds(), "result_count", len(resp.Payload))

	return resp.Payload, nil
}

func (a *RealLoader) GetSilences(ctx context.Context, filter []string) (models.GettableSilences, error) {
	params := silence.NewGetSilencesParams().WithContext(ctx)

	if len(filter) > 0 {
		params = params.WithFilter(filter)
	}

	start := time.Now()
	resp, err := a.client.Silence.GetSilences(params)
	duration := time.Since(start)
	if err != nil {
		klogutil.FromContext(ctx).Error(err, "Backend call failed", "backend", "alertmanager", "operation", "silences",
			"duration_ms", duration.Milliseconds())
		return nil, fmt.Errorf("error fetching silences: %w", err)
	}
	klogutil.FromContext(ctx).V(4).Info("Backend call completed", "backend", "alertmanager", "operation", "silences",
		"duration_ms", duration.Milliseconds(), "result_count", len(resp.Payload))

	return resp.Payload, nil
}

func (a *RealLoader) GetSilence(ctx context.Context, id string) (*models.GettableSilence, error) {
	params := silence.NewGetSilenceParams().WithContext(ctx).WithSilenceID(strfmt.UUID(id))
	start := time.Now()
	resp, err := a.client.Silence.GetSilence(params)
	duration := time.Since(start)
	if err != nil {
		klogutil.FromContext(ctx).Error(err, "Backend call failed", "backend", "alertmanager", "operation", "get_silence",
			"duration_ms", duration.Milliseconds())
		return nil, fmt.Errorf("error fetching silence %s: %w", id, err)
	}
	if resp == nil || resp.Payload == nil {
		return nil, fmt.Errorf("alertmanager returned an empty silence for %s", id)
	}
	klogutil.FromContext(ctx).V(4).Info("Backend call completed", "backend", "alertmanager", "operation", "get_silence",
		"duration_ms", duration.Milliseconds())
	return resp.Payload, nil
}

func (a *RealLoader) PostSilence(ctx context.Context, body *models.PostableSilence) (string, error) {
	params := silence.NewPostSilencesParams().WithContext(ctx).WithSilence(body)
	start := time.Now()
	resp, err := a.client.Silence.PostSilences(params)
	duration := time.Since(start)
	if err != nil {
		klogutil.FromContext(ctx).Error(err, "Backend call failed", "backend", "alertmanager", "operation", "post_silence",
			"duration_ms", duration.Milliseconds())
		return "", fmt.Errorf("error posting silence: %w", err)
	}
	if resp == nil || resp.Payload == nil || resp.Payload.SilenceID == "" {
		return "", fmt.Errorf("alertmanager returned an empty silence id")
	}
	klogutil.FromContext(ctx).V(4).Info("Backend call completed", "backend", "alertmanager", "operation", "post_silence",
		"duration_ms", duration.Milliseconds(), "silence_id", resp.Payload.SilenceID)
	return resp.Payload.SilenceID, nil
}

func (a *RealLoader) DeleteSilence(ctx context.Context, id string) error {
	params := silence.NewDeleteSilenceParams().WithContext(ctx).WithSilenceID(strfmt.UUID(id))
	start := time.Now()
	_, err := a.client.Silence.DeleteSilence(params)
	duration := time.Since(start)
	if err != nil {
		klogutil.FromContext(ctx).Error(err, "Backend call failed", "backend", "alertmanager", "operation", "delete_silence",
			"duration_ms", duration.Milliseconds())
		return fmt.Errorf("error deleting silence %s: %w", id, err)
	}
	klogutil.FromContext(ctx).V(4).Info("Backend call completed", "backend", "alertmanager", "operation", "delete_silence",
		"duration_ms", duration.Milliseconds(), "silence_id", id)
	return nil
}
