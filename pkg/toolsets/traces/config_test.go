package traces

import (
	"testing"

	serverconfig "github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/require"

	"github.com/containers/kubernetes-mcp-server/pkg/obs/openshift"
)

// TestTOMLUseRouteInstallsResolver is the embedder contract: openshift-mcp-server
// (and peers) parse use_route via RegisterToolsetConfig, not via obs-mcp main.go.
// use_route=true must leave a non-nil OpenShift Resolver on the parsed config.
func TestTOMLUseRouteInstallsResolver(t *testing.T) {
	t.Run("use_route true installs RouteClient", func(t *testing.T) {
		serverCfg, err := serverconfig.ReadToml(t.Context(), []byte(`
[toolset_configs."observability/traces"]
use_route = true
`), serverconfig.WithBaseDefault())
		require.NoError(t, err)

		ext, ok := serverCfg.GetToolsetConfig(ToolsetName)
		require.True(t, ok, "toolset config should be registered and parsed")

		tracesCfg, ok := ext.(*Config)
		require.True(t, ok)
		require.True(t, tracesCfg.UseRoute)
		require.NotNil(t, tracesCfg.Resolver, "use_route=true must install a Resolver for library consumers")
		_, ok = tracesCfg.Resolver.(*openshift.RouteClient)
		require.True(t, ok)
	})

	t.Run("use_route false leaves Resolver nil", func(t *testing.T) {
		serverCfg, err := serverconfig.ReadToml(t.Context(), []byte(`
[toolset_configs."observability/traces"]
use_route = false
`), serverconfig.WithBaseDefault())
		require.NoError(t, err)

		ext, ok := serverCfg.GetToolsetConfig(ToolsetName)
		require.True(t, ok)
		tracesCfg := ext.(*Config)
		require.False(t, tracesCfg.UseRoute)
		require.Nil(t, tracesCfg.Resolver)
	})
}

func TestValidateInstallsResolver(t *testing.T) {
	cfg := &Config{UseRoute: true}
	require.Nil(t, cfg.Resolver)
	require.NoError(t, cfg.Validate())
	require.NotNil(t, cfg.Resolver)
	_, ok := cfg.Resolver.(*openshift.RouteClient)
	require.True(t, ok)
}
