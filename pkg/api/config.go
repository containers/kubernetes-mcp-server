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
	// If token exchange is configured,
	// the token is exchanged first before being passed through.
	ClusterAuthPassthrough = "passthrough"

	// ClusterAuthKubeconfig uses kubeconfig credentials (e.g., ServiceAccount token).
	// Use when cluster auth is separate from MCP client auth.
	ClusterAuthKubeconfig = "kubeconfig"
)

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

type TokenExchangeClientAuthMethod string

const (
	TokenExchangeClientAuthMethodSecretBasic TokenExchangeClientAuthMethod = "client_secret_basic"
	TokenExchangeClientAuthMethodSecretPost  TokenExchangeClientAuthMethod = "client_secret_post"
	TokenExchangeClientAuthMethodPrivateKey  TokenExchangeClientAuthMethod = "private_key_jwt"
	TokenExchangeClientAuthMethodJWTFile     TokenExchangeClientAuthMethod = "jwt_file"
)
