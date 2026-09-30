package netobserv

import (
	"testing"

	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/stretchr/testify/require"
)

func TestMergeManualConfigOverridesOnlyConfiguredBackends(t *testing.T) {
	lokiEnabled := false
	cfg := mergeManualConfig(netobservclient.EffectiveConfig{
		Found:             true,
		LokiEnabled:       true,
		LokiMode:          "LokiStack",
		PrometheusEnabled: true,
		PrometheusMode:    "Auto",
	}, &netobservclient.Config{
		LokiEnabled:    &lokiEnabled,
		PrometheusMode: "Manual",
	})

	require.True(t, cfg.Found)
	require.False(t, cfg.LokiEnabled)
	require.Equal(t, "LokiStack", cfg.LokiMode)
	require.True(t, cfg.PrometheusEnabled)
	require.Equal(t, "Manual", cfg.PrometheusMode)
}
