package config

import "fmt"

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
// Each extended config manager registers a factory function to parse its config from TOML primitives.
type ExtendedConfig interface {
	// Validate validates the extended configuration. Returns an error if the configuration is invalid.
	Validate() error
}

// RequireTLSValidator is implemented by extended configs whose TLS settings
// must stay consistent with the process-wide require_tls option.
type RequireTLSValidator interface {
	ValidateRequireTLS(requireTLS bool) error
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

// Prompt represents the metadata and content of an MCP prompt.
// See MCP specification: https://spec.modelcontextprotocol.io/specification/server/prompts/
type Prompt struct {
	Name        string           `json:"name" toml:"name"`
	Title       string           `json:"title,omitempty" toml:"title,omitempty"`
	Description string           `json:"description,omitempty" toml:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty" toml:"arguments,omitempty"`
	Templates   []PromptTemplate `json:"messages,omitempty" toml:"messages,omitempty"`
}

// PromptArgument defines a parameter that can be passed to a prompt.
// See MCP specification: https://spec.modelcontextprotocol.io/specification/server/prompts/
type PromptArgument struct {
	Name        string `json:"name" toml:"name"`
	Description string `json:"description,omitempty" toml:"description,omitempty"`
	Required    bool   `json:"required" toml:"required"`
}

// PromptTemplate represents a message template from configuration with placeholders like {{arg}}.
// This is used for configuration parsing and gets rendered into PromptMessage at runtime.
type PromptTemplate struct {
	Role    string `json:"role" toml:"role"`
	Content string `json:"content" toml:"content"`
}

// ConfirmationRule defines a rule for prompting the user before an action.
// Rules are classified as tool-level or kube-level based on which fields are set.
// A rule must not have both tool-level and kube-level fields set.
type ConfirmationRule struct {
	// Tool-level fields
	Tool        string `toml:"tool,omitempty"`
	Destructive *bool  `toml:"destructive,omitempty"`
	// Kube-level fields
	Verb      string `toml:"verb,omitempty"`
	Kind      string `toml:"kind,omitempty"`
	Group     string `toml:"group,omitempty"`
	Version   string `toml:"version,omitempty"`
	Name      string `toml:"name,omitempty"`
	Namespace string `toml:"namespace,omitempty"`
	// Common fields
	Message string `toml:"message"`
}

// IsToolLevel returns true if the rule targets MCP tool invocations.
func (r *ConfirmationRule) IsToolLevel() bool {
	return r.Tool != "" || r.Destructive != nil
}

// IsKubeLevel returns true if the rule targets Kubernetes API requests.
func (r *ConfirmationRule) IsKubeLevel() bool {
	return r.Verb != "" || r.Kind != "" || r.Group != "" || r.Version != "" || r.Name != "" || r.Namespace != ""
}

// Validate checks that the rule is well-formed.
// A rule must be either tool-level or kube-level (not both, and not neither).
// Tool-level rules must not contain kube-level-only fields and vice versa.
func (r *ConfirmationRule) Validate() error {
	if r.IsToolLevel() && r.IsKubeLevel() {
		return fmt.Errorf("confirmation rule mixes tool-level fields (tool, destructive) with kube-level fields (verb, kind, group, version, name, namespace): %q", r.Message)
	}
	if !r.IsToolLevel() && !r.IsKubeLevel() {
		return fmt.Errorf("confirmation rule must set at least one tool-level field (tool, destructive) or kube-level field (verb, kind, group, version, name, namespace): %q", r.Message)
	}
	return nil
}
