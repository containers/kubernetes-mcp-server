package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

// StaticConfig is the configuration for the server.
// It allows to configure server specific settings and tools to be enabled or disabled.
type StaticConfig struct {
	DeniedResources []GroupVersionKind `toml:"denied_resources"`

	LogLevel   int    `toml:"log_level,omitempty"`
	Port       string `toml:"port,omitempty"`
	SSEBaseURL string `toml:"sse_base_url,omitempty"`
	KubeConfig string `toml:"kubeconfig,omitempty"`
	ListOutput string `toml:"list_output,omitempty"`
	// When true, expose only tools annotated with readOnlyHint=true
	ReadOnly bool `toml:"read_only,omitempty"`
	// When true, disable tools annotated with destructiveHint=true
	DisableDestructive   bool     `toml:"disable_destructive,omitempty"`
	EnabledTools         []string `toml:"enabled_tools,omitempty"`
	DisabledTools        []string `toml:"disabled_tools,omitempty"`
	RequireOAuth         bool     `toml:"require_oauth,omitempty"`
	AuthorizationURL     string   `toml:"authorization_url,omitempty"`
	JwksURL              string   `toml:"jwks_url,omitempty"`
	CertificateAuthority string   `toml:"certificate_authority,omitempty"`
	ServerURL            string   `toml:"server_url,omitempty"`
}

type GroupVersionKind struct {
	Group   string `toml:"group"`
	Version string `toml:"version"`
	Kind    string `toml:"kind,omitempty"`
}

// ReadConfig reads the toml file and returns the StaticConfig.
func ReadConfig(configPath string) (*StaticConfig, error) {
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config *StaticConfig
	err = toml.Unmarshal(configData, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
