package alertmanagement

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	serverconfig "github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/require"

	"github.com/containers/kubernetes-mcp-server/pkg/obs/auth"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/toolcfg"
)

func TestConfigValidateAndAuthMode(t *testing.T) {
	require.NoError(t, (&Config{}).Validate())
	require.EqualError(t, (&Config{AuthMode: "oauth"}).Validate(),
		`invalid auth_mode: "oauth" (valid options: "header", "kubeconfig")`)
	require.Equal(t, auth.AuthModeHeader, (&Config{}).GetAuthMode())
	require.Equal(t, auth.AuthModeKubeConfig, (&Config{AuthMode: auth.AuthModeKubeConfig}).GetAuthMode())
}

func TestGetConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[toolset_configs."observability/alert-management"]
auth_mode = "kubeconfig"
management_api_url = "https://management.example.com"
`), 0o600))
	cfg, err := serverconfig.Read(t.Context(), path, "")
	require.NoError(t, err)
	params := api.ToolHandlerParams{Context: t.Context(), Config: cfg}
	require.Equal(t, "https://management.example.com", GetConfig(params).ManagementAPIURL)
	require.Equal(t, auth.AuthModeKubeConfig, GetConfig(params).AuthMode)

	standalone := &Config{ManagementAPIURL: "https://standalone.example.com"}
	params.Context = toolcfg.With(t.Context(), ToolsetName, standalone)
	require.Same(t, standalone, GetConfig(params))
	require.Same(t, DefaultConfig, GetConfig(api.ToolHandlerParams{Context: t.Context()}))
}

func TestConfigRejectsInsecureTLS(t *testing.T) {
	require.ErrorContains(t, (&Config{Insecure: true}).Validate(), "insecure TLS is not supported")
}
