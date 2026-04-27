package api

const (
	ClusterProviderKubeConfig = "kubeconfig"
	ClusterProviderInCluster  = "in-cluster"
	ClusterProviderDisabled   = "disabled"
	ClusterProviderKcp        = "kcp"
)

// ClusterAuthMode constants define how the MCP server authenticates to the cluster.
const (
	// ClusterAuthPassthrough passes the OAuth token to the cluster.
	// If token exchange is configured (token_exchange_strategy or sts_audience),
	// the token is exchanged first before being passed through.
	ClusterAuthPassthrough = "passthrough"

	// ClusterAuthKubeconfig uses kubeconfig credentials (e.g., ServiceAccount token).
	// Use when cluster auth is separate from MCP client auth.
	ClusterAuthKubeconfig = "kubeconfig"
)

// ClusterAuthProvider provides configuration for how the MCP server authenticates to clusters.
type ClusterAuthProvider interface {
	// GetClusterAuthMode returns the raw cluster authentication mode from config.
	// Returns empty string if not explicitly set.
	GetClusterAuthMode() string
	// ResolveClusterAuthMode returns the effective cluster auth mode.
	// If explicitly set, returns that value. Otherwise auto-detects based on require_oauth.
	ResolveClusterAuthMode() string
}

type ClusterProvider interface {
	// GetClusterProviderStrategy returns the cluster provider strategy (if configured).
	GetClusterProviderStrategy() string
	// GetKubeConfigPath returns the path to the kubeconfig file (if configured).
	GetKubeConfigPath() string
}

// ExtendedConfig is the interface that all configuration extensions must implement.
// Each extended config manager registers a factory function to parse its config from TOML primitives
type ExtendedConfig interface {
	// Validate validates the extended configuration.  Returns an error if the configuration is invalid.
	Validate() error
}

type ExtendedConfigProvider interface {
	// GetProviderConfig returns the extended configuration for the given provider strategy.
	// The boolean return value indicates whether the configuration was found.
	GetProviderConfig(strategy string) (ExtendedConfig, bool)
	// GetToolsetConfig returns the extended configuration for the given toolset name.
	// The boolean return value indicates whether the configuration was found.
	GetToolsetConfig(name string) (ExtendedConfig, bool)
}

type GroupVersionKind struct {
	Group   string `json:"group" toml:"group"`
	Version string `json:"version" toml:"version"`
	Kind    string `json:"kind,omitempty" toml:"kind,omitempty"`
}

type DeniedResourcesProvider interface {
	// GetDeniedResources returns a list of GroupVersionKinds that are denied.
	GetDeniedResources() []GroupVersionKind
}

type StsConfigProvider interface {
	GetStsClientId() string
	GetStsClientSecret() string
	GetStsAudience() string
	GetStsScopes() []string
	GetStsStrategy() string
	GetStsAuthStyle() string
	GetStsClientCertFile() string
	GetStsClientKeyFile() string
	GetCertificateAuthority() string
}

// ValidationEnabledProvider provides access to validation enabled setting.
type ValidationEnabledProvider interface {
	IsValidationEnabled() bool
}

// RequireTLSProvider provides access to require_tls setting.
type RequireTLSProvider interface {
	IsRequireTLS() bool
}

// RequireOAuthProvider provides access to require_oauth setting.
type RequireOAuthProvider interface {
	IsRequireOAuth() bool
}

type BaseConfig interface {
	ClusterAuthProvider
	ClusterProvider
	ConfirmationRulesProvider
	DeniedResourcesProvider
	ExtendedConfigProvider
	StsConfigProvider
	ValidationEnabledProvider
	RequireTLSProvider
	RequireOAuthProvider
}
