package toolset_tools

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/client-go/rest"

	"github.com/rhobs/obs-mcp/pkg/auth"
	"github.com/rhobs/obs-mcp/pkg/metrics"
)

type mockKubernetesClient struct {
	api.KubernetesClient
	restConfig *rest.Config
}

func (m *mockKubernetesClient) RESTConfig() *rest.Config {
	return m.restConfig
}

type mockConfigProvider struct {
	api.BaseConfig
	config *metrics.Config
}

func (m *mockConfigProvider) GetProviderConfig(string) (api.ExtendedConfig, bool) {
	return nil, false
}

func (m *mockConfigProvider) GetToolsetConfig(name string) (api.ExtendedConfig, bool) {
	if name == metrics.ToolsetName && m.config != nil {
		return m.config, true
	}
	return nil, false
}

type mockToolCallRequest struct{}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return nil
}

func newTestParams(ctx context.Context, restConfig *rest.Config, cfg *metrics.Config) api.ToolHandlerParams {
	return api.ToolHandlerParams{
		Context:          ctx,
		KubernetesClient: &mockKubernetesClient{restConfig: restConfig},
		BaseConfig:       &mockConfigProvider{config: cfg},
		ToolCallRequest:  &mockToolCallRequest{},
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
	cfg := &metrics.Config{AuthMode: auth.AuthModeKubeConfig}
	params := newTestParams(context.Background(), &rest.Config{}, cfg)
	got := getConfig(params)
	if got.GetAuthMode() != auth.AuthModeKubeConfig {
		t.Errorf("expected auth mode %q, got %q", auth.AuthModeKubeConfig, got.GetAuthMode())
	}
}
