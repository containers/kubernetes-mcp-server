package netobserv

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveConfigFromFlowCollector(t *testing.T) {
	cfg, err := effectiveConfigFromFlowCollector(map[string]any{
		"spec": map[string]any{
			"loki":          map[string]any{"enable": false, "mode": "Manual"},
			"prometheus":    map[string]any{"querier": map[string]any{"enable": true, "mode": "Auto"}},
			"namespace":     "observability",
			"consolePlugin": map[string]any{"port": float64(9001)},
		},
	})
	require.NoError(t, err)
	require.True(t, cfg.Found)
	require.False(t, cfg.LokiEnabled)
	require.Equal(t, "Manual", cfg.LokiMode)
	require.True(t, cfg.PrometheusEnabled)
	require.Equal(t, "Auto", cfg.PrometheusMode)
	require.Equal(t, "observability", cfg.Namespace)
	require.Equal(t, 9001, cfg.Port)
}

func TestEffectiveConfigFromFlowCollectorDefaultsBackendsOn(t *testing.T) {
	cfg, err := effectiveConfigFromFlowCollector(map[string]any{"spec": map[string]any{}})
	require.NoError(t, err)
	require.True(t, cfg.Found)
	require.True(t, cfg.LokiEnabled)
	require.True(t, cfg.PrometheusEnabled)
}

func TestEffectiveConfigFromFlowCollectorRejectsInvalidFieldType(t *testing.T) {
	_, err := effectiveConfigFromFlowCollector(map[string]any{
		"spec": map[string]any{"loki": map[string]any{"enable": "true"}},
	})
	require.ErrorContains(t, err, "spec.loki.enable must be boolean")
}
