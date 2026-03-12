package api

const (
	ClusterProviderKubeConfig = "kubeconfig"
	ClusterProviderInCluster  = "in-cluster"
	ClusterProviderDisabled   = "disabled"
	ClusterProviderKcp        = "kcp"
)

type AuthProvider interface {
	// IsRequireOAuth indicates whether OAuth authentication is required.
	IsRequireOAuth() bool
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

// ConfigProvider provides read-only access to configuration from tool and prompt handlers.
// This interface is embedded in ToolHandlerParams and PromptHandlerParams, allowing
// toolsets to access configuration at execution time (not just at initialization).
type ConfigProvider interface {
	// GetClusterProviderStrategy returns the cluster provider strategy (e.g. "kubeconfig", "in-cluster").
	GetClusterProviderStrategy() string
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
}

// ValidationEnabledProvider provides access to validation enabled setting.
type ValidationEnabledProvider interface {
	IsValidationEnabled() bool
}

type BaseConfig interface {
	AuthProvider
	ClusterProvider
	DeniedResourcesProvider
	ConfigProvider
	StsConfigProvider
	ValidationEnabledProvider
}
