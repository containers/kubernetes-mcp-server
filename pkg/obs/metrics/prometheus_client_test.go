package metrics

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/client-go/rest"

	"github.com/rhobs/obs-mcp/pkg/auth"
	"github.com/rhobs/obs-mcp/pkg/toolcfg"
)

type mockKubernetesClient struct {
	api.KubernetesClient
	restConfig *rest.Config
}

func (m *mockKubernetesClient) RESTConfig() *rest.Config {
	return m.restConfig
}

type mockToolCallRequest struct {
	arguments map[string]any
}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return m.arguments
}

func newTestParams(ctx context.Context, restConfig *rest.Config, cfg *Config) api.ToolHandlerParams {
	if cfg != nil {
		ctx = toolcfg.With(ctx, ToolsetName, cfg)
	}
	return api.ToolHandlerParams{
		Context:          ctx,
		KubernetesClient: &mockKubernetesClient{restConfig: restConfig},
		Request:          &mockToolCallRequest{},
	}
}

func TestGetConfig_DefaultAuthMode(t *testing.T) {
	params := newTestParams(context.Background(), &rest.Config{}, nil)
	cfg := getConfig(params)
	if cfg.GetAuthMode() != auth.AuthModeHeader {
		t.Errorf("expected default auth mode %q, got %q", auth.AuthModeHeader, cfg.GetAuthMode())
	}
}

func TestGetConfig_CustomAuthMode(t *testing.T) {
	cfg := &Config{AuthMode: auth.AuthModeKubeConfig}
	params := newTestParams(context.Background(), &rest.Config{}, cfg)
	got := getConfig(params)
	if got.GetAuthMode() != auth.AuthModeKubeConfig {
		t.Errorf("expected auth mode %q, got %q", auth.AuthModeKubeConfig, got.GetAuthMode())
	}
}
