package logs

import (
	"testing"

	serverconfig "github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/require"

	"github.com/rhobs/obs-mcp/pkg/openshift"
)

// TestTOMLUseRouteInstallsResolver is the embedder contract: openshift-mcp-server
// (and peers) parse use_route via RegisterToolsetConfig, not via obs-mcp main.go.
// use_route=true must leave a non-nil OpenShift Resolver on the parsed config.
func TestTOMLUseRouteInstallsResolver(t *testing.T) {
	t.Run("use_route true installs LogsGatewayResolver", func(t *testing.T) {
		serverCfg, err := serverconfig.ReadToml(t.Context(), []byte(`
[toolset_configs."observability/logs"]
use_route = true
`), serverconfig.WithBaseDefault())
		require.NoError(t, err)

		ext, ok := serverCfg.GetToolsetConfig(ToolsetName)
		require.True(t, ok, "toolset config should be registered and parsed")

		logsCfg, ok := ext.(*Config)
		require.True(t, ok)
		require.True(t, logsCfg.UseRoute)
		require.NotNil(t, logsCfg.Resolver, "use_route=true must install a Resolver for library consumers")
		_, ok = logsCfg.Resolver.(*openshift.LogsGatewayResolver)
		require.True(t, ok)
	})

	t.Run("use_route false leaves Resolver nil", func(t *testing.T) {
		serverCfg, err := serverconfig.ReadToml(t.Context(), []byte(`
[toolset_configs."observability/logs"]
use_route = false
`), serverconfig.WithBaseDefault())
		require.NoError(t, err)

		ext, ok := serverCfg.GetToolsetConfig(ToolsetName)
		require.True(t, ok)
		logsCfg := ext.(*Config)
		require.False(t, logsCfg.UseRoute)
		require.Nil(t, logsCfg.Resolver)
	})
}

func TestValidateInstallsResolver(t *testing.T) {
	cfg := &Config{UseRoute: true}
	require.Nil(t, cfg.Resolver)
	require.NoError(t, cfg.Validate())
	require.NotNil(t, cfg.Resolver)
	_, ok := cfg.Resolver.(*openshift.LogsGatewayResolver)
	require.True(t, ok)
}
