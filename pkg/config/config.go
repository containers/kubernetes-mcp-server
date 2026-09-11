package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

const (
	// Environment variable names for TLS configuration.
	EnvTLSMinVersion   = "TLS_MIN_VERSION"
	EnvTLSCipherSuites = "TLS_CIPHER_SUITES"

	// DefaultRateLimitBurst is the default burst size used when rate_limit_rps is
	// set but rate_limit_burst is not specified (zero value).
	DefaultRateLimitBurst = 10

	extensionToolsetTable  = "toolset_configs"
	extensionProviderTable = "cluster_provider_configs"
)

// ConfigPathEnvName is the environment variable whose value is the path to the
// main configuration TOML file. It is ignored if --config is provided.
// Downstream builds may change this in defaultOverrides.
var ConfigPathEnvName = "K8S_MCP_CONFIG_PATH"

// ToolOverride contains per-tool configuration overrides.
type ToolOverride struct {
	// Description replaces the tool's advertised description when non-empty.
	Description string `toml:"description,omitempty"`
}

// HTTPConfig contains inbound HTTP server options for timeouts, size limits, and rate limiting.
type HTTPConfig struct {
	// ReadHeaderTimeout is the amount of time allowed to read request headers.
	// This is the primary defense against Slowloris attacks.
	ReadHeaderTimeout Option[time.Duration]
	// MaxBodyBytes is the maximum size of request body in bytes.
	// MCP payloads (tools/call with Kubernetes manifests) can be large,
	// so the default is 16MB to accommodate CRDs and ConfigMaps.
	MaxBodyBytes Option[int64]
	// RateLimitRPS is the maximum number of requests per second per session.
	// When set to 0 (default), rate limiting is disabled.
	RateLimitRPS Option[float64]
	// RateLimitBurst is the maximum burst size for rate limiting per session.
	// Allows short bursts of requests above the rate limit.
	// Only effective when rate_limit_rps > 0.
	// When zero, the rate limiting middleware applies DefaultRateLimitBurst.
	RateLimitBurst Option[int]
}

// newHTTPConfig returns HTTPConfig with Option metadata and defaults.
func newHTTPConfig() HTTPConfig {
	return HTTPConfig{
		ReadHeaderTimeout: opt("read_header_timeout", 10*time.Second).desc("Max duration to read request headers"),
		MaxBodyBytes:      opt("max_body_bytes", int64(16<<20)).reload().desc("Max request body size in bytes"),
		RateLimitRPS: opt("rate_limit_rps", 0.0).reload().validate(func(v float64) error {
			if v < 0 {
				return fmt.Errorf("rate_limit_rps must not be negative (got %v)", v)
			}
			return nil
		}).desc("Max requests per second per session (0 disables)"),
		RateLimitBurst: opt("rate_limit_burst", 0).reload().validate(func(v int) error {
			if v < 0 {
				return fmt.Errorf("rate_limit_burst must not be negative (got %d)", v)
			}
			return nil
		}).desc("Max burst size for rate limiting per session"),
	}
}

// TelemetryConfig contains OpenTelemetry options.
// Values can be set via TOML or environment variables; env names stay OTEL_*
// and take precedence over TOML when set.
type TelemetryConfig struct {
	// Enabled explicitly enables or disables telemetry.
	// If nil (not set), telemetry is auto-enabled when Endpoint is configured.
	// If explicitly set to false, telemetry is disabled even if env vars are set.
	Enabled Option[*bool]
	// Endpoint is the OTLP endpoint URL (e.g., "http://localhost:4317").
	// Overridden by OTEL_EXPORTER_OTLP_ENDPOINT when that env var is non-empty.
	Endpoint Option[string]
	// Protocol specifies the OTLP protocol: "grpc" (default) or "http/protobuf".
	// Overridden by OTEL_EXPORTER_OTLP_PROTOCOL when that env var is non-empty.
	Protocol Option[string]
	// TracesSampler specifies the trace sampling strategy.
	// Supported values: "always_on", "always_off", "traceidratio",
	// "parentbased_always_on", "parentbased_traceidratio".
	// Overridden by OTEL_TRACES_SAMPLER when that env var is non-empty.
	TracesSampler Option[string]
	// TracesSamplerArg is the sampling ratio for ratio-based samplers (0.0 to 1.0).
	// Overridden by OTEL_TRACES_SAMPLER_ARG when that env var is non-empty.
	TracesSamplerArg Option[*float64]
	// LogsExporter is the OTLP logs exporter. Set to "none" to disable log export.
	// Overridden by OTEL_LOGS_EXPORTER when that env var is non-empty.
	LogsExporter Option[string]
	// MetricsExporter is the OTLP metrics exporter. Set to "none" to disable metrics export.
	// Overridden by OTEL_METRICS_EXPORTER when that env var is non-empty.
	MetricsExporter Option[string]
}

// newTelemetryConfig returns TelemetryConfig with Option metadata and defaults.
func newTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{
		Enabled:          opt[*bool]("enabled", nil).desc("Explicitly enable or disable telemetry"),
		Endpoint:         opt("endpoint", "").env("OTEL_EXPORTER_OTLP_ENDPOINT", parseEnvString).desc("OTLP endpoint URL"),
		Protocol:         opt("protocol", "grpc").env("OTEL_EXPORTER_OTLP_PROTOCOL", parseEnvString).desc("OTLP protocol (grpc or http/protobuf)"),
		TracesSampler:    opt("traces_sampler", "").env("OTEL_TRACES_SAMPLER", parseEnvString).desc("Trace sampling strategy"),
		TracesSamplerArg: opt[*float64]("traces_sampler_arg", nil).env("OTEL_TRACES_SAMPLER_ARG", parseEnvFloat64Ptr).desc("Sampling ratio for ratio-based samplers"),
		LogsExporter:     opt("logs_exporter", "").env("OTEL_LOGS_EXPORTER", parseEnvString).desc("OTLP logs exporter (set to none to disable)"),
		MetricsExporter:  opt("metrics_exporter", "").env("OTEL_METRICS_EXPORTER", parseEnvString).desc("OTLP metrics exporter (set to none to disable)"),
	}
}

// IsEnabled returns true if telemetry should be enabled.
// Logic:
//   - If Enabled is explicitly set to false, return false (explicit disable)
//   - If Enabled is explicitly set to true, return true only if endpoint is available
//   - If Enabled is nil (not set), return true if endpoint is available (auto-enable)
func (c *TelemetryConfig) IsEnabled() bool {
	if c.Enabled.Get() != nil && !*c.Enabled.Get() {
		return false
	}
	return c.Endpoint.Get() != ""
}

// TokenExchangeClientAuth is nested under [token_exchange.client_auth].
type TokenExchangeClientAuth struct {
	// Method is the token exchange client authentication method
	// (e.g. client_secret_post, private_key_jwt).
	Method Option[string]
	// ClientID is the OAuth client ID used for token exchange.
	ClientID Option[string]
	// ClientSecret is the OAuth client secret used for token exchange.
	ClientSecret Option[string]
	// CertificateFile is a client certificate PEM file for JWT assertion auth.
	CertificateFile Option[string]
	// PrivateKeyFile is a client private key PEM file for JWT assertion auth.
	PrivateKeyFile Option[string]
	// TokenFile is a path to a JWT from an external identity provider.
	TokenFile Option[string]
}

// newTokenExchangeClientAuth returns TokenExchangeClientAuth with Option metadata and defaults.
func newTokenExchangeClientAuth() TokenExchangeClientAuth {
	return TokenExchangeClientAuth{
		Method:          opt("method", "").reload().desc("Token exchange client authentication method"),
		ClientID:        opt("client_id", "").reload().desc("OAuth client ID for token exchange"),
		ClientSecret:    opt("client_secret", "").reload().secret().desc("OAuth client secret for token exchange"),
		CertificateFile: opt("certificate_file", "").reload().desc("Client certificate PEM file for JWT assertion auth"),
		PrivateKeyFile:  opt("private_key_file", "").reload().desc("Client private key PEM file for JWT assertion auth"),
		TokenFile:       opt("token_file", "").reload().desc("Path to a JWT from an external identity provider"),
	}
}

// configured reports whether any client-auth field is set.
func (c *TokenExchangeClientAuth) configured() bool {
	return c.Method.Get() != "" || c.ClientID.Get() != "" || c.ClientSecret.Get() != "" ||
		c.CertificateFile.Get() != "" || c.PrivateKeyFile.Get() != "" || c.TokenFile.Get() != ""
}

// TokenExchangeConfig is nested under [token_exchange].
// A missing block leaves OAuth tokens unchanged.
type TokenExchangeConfig struct {
	// Strategy selects the token exchange implementation (e.g. rfc8693, keycloak-v1).
	Strategy Option[string]
	// Audience is the audience requested during token exchange.
	Audience Option[string]
	// Scopes are the scopes requested during token exchange.
	Scopes Option[[]string]
	// SubjectTokenType is an RFC 8693 subject_token_type override.
	SubjectTokenType Option[string]
	// RequestedTokenType is an RFC 8693 requested_token_type override.
	RequestedTokenType Option[string]
	// ClientAuth is nested under [token_exchange.client_auth].
	ClientAuth TokenExchangeClientAuth `toml:"client_auth"`
	// present is true when a [token_exchange] table appeared in TOML,
	// even if every field still has its default.
	present bool
}

// newTokenExchangeConfig returns TokenExchangeConfig with Option metadata and defaults.
func newTokenExchangeConfig() TokenExchangeConfig {
	return TokenExchangeConfig{
		Strategy:           opt("strategy", "").reload().desc("Token exchange strategy"),
		Audience:           opt("audience", "").reload().desc("Audience for token exchange"),
		Scopes:             opt("scopes", []string(nil)).reload().desc("Scopes for token exchange"),
		SubjectTokenType:   opt("subject_token_type", "").reload().desc("RFC 8693 subject_token_type override"),
		RequestedTokenType: opt("requested_token_type", "").reload().desc("RFC 8693 requested_token_type override"),
		ClientAuth:         newTokenExchangeClientAuth(),
	}
}

// GetClientAuth returns the nested client-auth config, or nil when unused.
func (c *TokenExchangeConfig) GetClientAuth() *TokenExchangeClientAuth {
	if !c.ClientAuth.configured() {
		return nil
	}
	return &c.ClientAuth
}

// Config is the resolved server configuration.
// It holds server-specific settings and which tools are enabled or disabled.
type Config struct {
	// LogLevel is logging verbosity (0-9), similar to kubectl.
	LogLevel Option[int]
	// LogFile is a path to a server log file. Required for logging in stdio mode.
	LogFile Option[string]
	// Port starts HTTP mode (Streamable HTTP at /mcp) when set. Empty is stdio.
	Port Option[string]
	// BindAddress is the address to bind the HTTP server to.
	BindAddress Option[string]
	// MetricsPort, when set in HTTP mode, starts a separate server for /metrics, /stats, and /healthz.
	MetricsPort Option[string]
	// ListOutput is the output format for resource list operations (yaml or table).
	ListOutput Option[string]
	// Stateless configures the MCP server to operate in stateless mode.
	// When true, the server will not send notifications to clients (e.g., tools/list_changed, prompts/list_changed).
	// This is useful for container deployments, load balancing, and serverless environments where
	// maintaining client state is not desired or possible. However, this disables dynamic tool
	// and prompt updates, requiring clients to manually refresh their tool/prompt lists.
	// Defaults to false (stateful mode with notifications enabled).
	Stateless Option[bool]
	// ServerInstructions are provided by the MCP server to the MCP client.
	// This can be used to provide specific instructions on how the client should use the server.
	ServerInstructions Option[string]
	// KubeConfig is the path to the Kubernetes configuration file.
	// A non-empty path selects the kubeconfig provider (including from a pod).
	KubeConfig Option[string]
	// ClusterProviderStrategy is how the server finds clusters.
	// If set to "kubeconfig", the clusters will be loaded from those in the kubeconfig.
	// If set to "in-cluster", the server will use the in-cluster config.
	// A non-empty kubeconfig path selects the kubeconfig provider even when this is empty.
	ClusterProviderStrategy Option[string]
	// ClusterAuthMode determines how the MCP server authenticates to the cluster.
	// Valid values: "passthrough" (forward Authorization header, with optional exchange), "kubeconfig" (use kubeconfig credentials).
	// If empty, defaults to passthrough: forwards the token when present, falls back to kubeconfig when absent.
	ClusterAuthMode Option[string]
	// DeniedResources are GVKs that tools must not access.
	DeniedResources Option[[]api.GroupVersionKind]
	// When true, expose only tools annotated with readOnlyHint=true.
	ReadOnly Option[bool]
	// When true, disable tools annotated with destructiveHint=true.
	DisableDestructive Option[bool]
	// ValidationEnabled enables pre-execution validation of tool calls.
	// When enabled, validates resources, schemas, and RBAC before execution.
	// Defaults to false.
	ValidationEnabled Option[bool]
	// EnableTargetCompatibilityToolFilters enables filtering of tools based on
	// cluster target compatibility (e.g., hiding OpenShift-specific tools when
	// connected to a non-OpenShift cluster). This feature is experimental, and
	// this option is subject to change or removal in a future release.
	// Defaults to false.
	EnableTargetCompatibilityToolFilters Option[bool]
	// Toolsets lists the MCP toolsets to enable.
	Toolsets Option[[]string]
	// Tool configuration
	EnabledTools  Option[[]string]
	DisabledTools Option[[]string]
	ToolOverrides Option[map[string]ToolOverride]
	// Prompt configuration
	Prompts Option[[]api.Prompt]
	// ConfirmationFallback is the global default fallback behavior when a client
	// does not support elicitation. Valid values are "deny" and "allow".
	ConfirmationFallback Option[string]
	// ConfirmationRules define rules for prompting the user before dangerous actions.
	ConfirmationRules Option[[]api.ConfirmationRule]
	// TLSCert is the path to the TLS certificate file for HTTPS.
	TLSCert Option[string]
	// TLSKey is the path to the TLS private key file for HTTPS.
	TLSKey Option[string]
	// RequireTLS enforces TLS for all server and client connections.
	// When true, the server will refuse to start without TLS certificates,
	// and outbound connections to non-HTTPS endpoints will be rejected.
	RequireTLS Option[bool]
	// TLSMinVersion is the minimum TLS version to accept (e.g., "1.2", "1.3").
	// Defaults to TLS 1.2 if not set. Overridden by TLS_MIN_VERSION when that env var is non-empty.
	TLSMinVersion Option[string]
	// TLSCipherSuites is a list of supported cipher suites for TLS connections.
	// If empty, Go's default cipher suites are used. Overridden by TLS_CIPHER_SUITES when that env var is non-empty.
	TLSCipherSuites Option[[]string]
	// HTTP server configuration (timeouts, size limits)
	HTTP HTTPConfig `toml:"http"`
	// RequireOAuth indicates whether the server requires OAuth for authentication.
	RequireOAuth Option[bool]
	// OAuthAudience is the valid audience for the OAuth tokens, used for offline JWT claim validation.
	OAuthAudience Option[string]
	// AuthorizationURL is the URL of the OIDC authorization server.
	// It is used for token validation and for STS token exchange.
	AuthorizationURL Option[string]
	// SkipJWTVerification allows the server to accept JWTs without cryptographic
	// signature verification when require_oauth is enabled but no authorization_url
	// is configured (offline-only validation). Only use behind a trusted reverse proxy
	// that performs token verification. When false (default), the server refuses to
	// start if require_oauth is true and authorization_url is empty.
	SkipJWTVerification Option[bool]
	// DisableDynamicClientRegistration indicates whether dynamic client registration is disabled.
	// If true, the .well-known endpoints will not expose the registration endpoint.
	DisableDynamicClientRegistration Option[bool]
	// OAuthScopes are the supported **client** scopes requested during the **client/frontend** OAuth flow.
	OAuthScopes Option[[]string]
	// ServerURL is the public URL of this server (used for well-known OAuth metadata).
	ServerURL Option[string]
	// CertificateAuthority is a CA path used to verify certificates.
	CertificateAuthority Option[string]
	// TrustProxyHeaders allows the server to use X-Forwarded-Host, X-Forwarded-Proto,
	// X-Forwarded-For, and X-Real-IP headers from reverse proxies.
	// Only enable this when the server is behind a trusted reverse proxy.
	// When false (default), the server requires server_url to be set for well-known
	// endpoint metadata and ignores forwarded headers for client IP and scheme detection.
	TrustProxyHeaders Option[bool]
	// TokenExchange configures global token exchange before tokens are passed to the cluster.
	// A missing block leaves OAuth tokens unchanged.
	TokenExchange TokenExchangeConfig `toml:"token_exchange"`
	// Telemetry contains OpenTelemetry configuration options.
	// These can also be configured via OTEL_* environment variables.
	Telemetry TelemetryConfig `toml:"telemetry"`
	// Kubernetes client limits and watcher timings. Env vars are integer milliseconds
	// (or numeric QPS/burst); TOML durations use Go duration syntax.
	// KubeClientQPS is the Kubernetes client QPS limit. 0 leaves client-go defaults.
	KubeClientQPS Option[float32]
	// KubeClientBurst is the Kubernetes client burst. 0 leaves client-go defaults.
	KubeClientBurst Option[int]
	// KubeconfigDebounceWindow is the debounce window for kubeconfig file changes.
	KubeconfigDebounceWindow Option[time.Duration]
	// ClusterStatePollInterval is the poll interval for cluster API discovery changes.
	ClusterStatePollInterval Option[time.Duration]
	// ClusterStateDebounceWindow is the debounce window for cluster state reloads.
	ClusterStateDebounceWindow Option[time.Duration]
	// WorkspacePollInterval is the poll interval for kcp workspace changes.
	WorkspacePollInterval Option[time.Duration]
	// WorkspaceDebounceWindow is the debounce window for kcp workspace reloads.
	WorkspaceDebounceWindow Option[time.Duration]

	// Internal: parsed provider configs (not exposed to TOML as Option fields)
	parsedClusterProviderConfigs map[string]api.ExtendedConfig
	// Internal: parsed toolset configs (not exposed to TOML as Option fields)
	parsedToolsetConfigs map[string]api.ExtendedConfig
	// Internal: which file last set cluster_provider_configs (for reload pinning)
	clusterProviderConfigsSource Source
	// Internal: which file last set toolset_configs
	toolsetConfigsSource Source

	// Internal: the config.toml directory, to help resolve relative file paths
	configDirPath string
	// Internal: known provider strategies, set via WithProviderStrategies
	providerStrategies []string
	// Internal: known token exchange strategies, set via WithTokenExchangeStrategies
	tokenExchangeStrategies []string
}

// BaseDefault returns upstream defaults before downstream defaultOverrides.
func BaseDefault() *Config {
	c := newConfig()
	walkOptions(c, func(o option, _ string) { o.resetToDefault() })
	return c
}

// New returns a Config with Option metadata and defaults, after downstream overrides.
func New() *Config {
	c := newConfig()
	defaultOverrides(c)
	walkOptions(c, func(o option, _ string) { o.resetToDefault() })
	return c
}

// newConfig installs Option metadata. Callers must reset values to Default
// (New / BaseDefault) after optional defaultOverrides.
func newConfig() *Config {
	return &Config{
		LogLevel:                opt("log_level", 0).reload().desc("Log verbosity (0-9)"),
		LogFile:                 opt("log_file", "").reload().desc("Server log file path"),
		Port:                    opt("port", "").desc("HTTP listen port (empty is stdio)"),
		BindAddress:             opt("bind_address", "0.0.0.0").desc("Address to bind the HTTP server"),
		MetricsPort:             opt("metrics_port", "").validate(validateMetricsPortNumber).desc("Separate metrics server port"),
		ListOutput:              opt("list_output", "table").reload().validate(validateListOutput).desc("Output format for resource list operations"),
		Stateless:               opt("stateless", false).desc("Run without tool/prompt change notifications"),
		ServerInstructions:      opt("server_instructions", "").desc("Instructions provided by the MCP server to clients"),
		KubeConfig:              opt("kubeconfig", "").desc("Path to the kubeconfig file"),
		ClusterProviderStrategy: opt("cluster_provider_strategy", "").desc("How the server finds clusters"),
		ClusterAuthMode: opt("cluster_auth_mode", "").reload().validate(validateClusterAuthModeValue).
			desc("How the MCP server authenticates to the cluster"),
		DeniedResources:    opt("denied_resources", []api.GroupVersionKind(nil)).reload().desc("GVKs that tools must not access"),
		ReadOnly:           opt("read_only", false).reload().desc("Expose only tools annotated readOnlyHint=true"),
		DisableDestructive: opt("disable_destructive", false).reload().desc("Disable tools annotated destructiveHint=true"),
		ValidationEnabled:  opt("validation_enabled", false).reload().desc("Enable pre-execution validation of tool calls"),
		EnableTargetCompatibilityToolFilters: opt("experimental_enable_target_compatibility_tool_filters", false).reload().
			desc("Filter tools based on cluster target compatibility"),
		Toolsets:      opt("toolsets", []string{"core", "config"}).reload().desc("MCP toolsets to enable"),
		EnabledTools:  opt("enabled_tools", []string(nil)).reload().desc("If set, only these tools are exposed"),
		DisabledTools: opt("disabled_tools", []string(nil)).reload().desc("Tools to hide"),
		ToolOverrides: opt("tool_overrides", map[string]ToolOverride(nil)).reload().desc("Per-tool configuration overrides"),
		Prompts:       opt("prompts", []api.Prompt(nil)).reload().desc("Custom MCP prompts"),
		ConfirmationFallback: opt("confirmation_fallback", "allow").reload().validate(validateConfirmationFallback).
			desc("Fallback when a client does not support elicitation"),
		ConfirmationRules: opt("confirmation_rules", []api.ConfirmationRule(nil)).reload().validate(validateConfirmationRules).
			desc("Rules for prompting before dangerous actions"),
		TLSCert:    opt("tls_cert", "").validate(validateExistingFile("tls_cert")).desc("Path to TLS certificate file for HTTPS"),
		TLSKey:     opt("tls_key", "").validate(validateExistingFile("tls_key")).desc("Path to TLS private key file for HTTPS"),
		RequireTLS: opt("require_tls", false).desc("Require TLS for server and outbound connections"),
		TLSMinVersion: opt("tls_min_version", "").env(EnvTLSMinVersion, parseEnvString).validate(validateTLSMinVersion).
			desc("Minimum TLS version"),
		TLSCipherSuites: opt("tls_cipher_suites", []string(nil)).env(EnvTLSCipherSuites, parseEnvStringSlice).validate(validateTLSCipherSuites).
			desc("Supported TLS cipher suites"),
		HTTP:                newHTTPConfig(),
		RequireOAuth:        opt("require_oauth", false).reload().desc("Require OAuth authorization"),
		OAuthAudience:       opt("oauth_audience", "").reload().desc("Valid audience for OAuth tokens"),
		AuthorizationURL:    opt("authorization_url", "").reload().desc("OIDC authorization server URL"),
		SkipJWTVerification: opt("skip_jwt_verification", false).reload().desc("Skip JWT signature verification"),
		DisableDynamicClientRegistration: opt("disable_dynamic_client_registration", false).reload().
			desc("Disable OAuth dynamic client registration"),
		OAuthScopes: opt("oauth_scopes", []string(nil)).reload().desc("Client scopes requested during the OAuth flow"),
		ServerURL:   opt("server_url", "").reload().desc("Public URL of this server"),
		CertificateAuthority: opt("certificate_authority", "").reload().validate(validateExistingFile("certificate_authority")).
			desc("CA path to verify certificates"),
		TrustProxyHeaders: opt("trust_proxy_headers", false).reload().desc("Trust X-Forwarded-* headers from reverse proxies"),
		TokenExchange:     newTokenExchangeConfig(),
		Telemetry:         newTelemetryConfig(),
		KubeClientQPS:     opt("kube_client_qps", float32(0)).env("KUBE_CLIENT_QPS", parseEnvFloat32).desc("Kubernetes client QPS limit"),
		KubeClientBurst:   opt("kube_client_burst", 0).env("KUBE_CLIENT_BURST", parseEnvInt).desc("Kubernetes client burst limit"),
		KubeconfigDebounceWindow: opt("kubeconfig_debounce_window", 100*time.Millisecond).
			env("KUBECONFIG_DEBOUNCE_WINDOW_MS", parseEnvDurationMS).desc("Debounce window for kubeconfig file changes"),
		ClusterStatePollInterval: opt("cluster_state_poll_interval", 30*time.Second).
			env("CLUSTER_STATE_POLL_INTERVAL_MS", parseEnvDurationMS).desc("Poll interval for cluster state changes"),
		ClusterStateDebounceWindow: opt("cluster_state_debounce_window", 5*time.Second).
			env("CLUSTER_STATE_DEBOUNCE_WINDOW_MS", parseEnvDurationMS).desc("Debounce window for cluster state changes"),
		WorkspacePollInterval: opt("workspace_poll_interval", 60*time.Second).
			env("WORKSPACE_POLL_INTERVAL_MS", parseEnvDurationMS).desc("Poll interval for kcp workspace changes"),
		WorkspaceDebounceWindow: opt("workspace_debounce_window", 5*time.Second).
			env("WORKSPACE_DEBOUNCE_WINDOW_MS", parseEnvDurationMS).desc("Debounce window for kcp workspace changes"),
	}
}

// GetProviderConfig returns the parsed cluster_provider_configs entry for strategy.
func (c *Config) GetProviderConfig(strategy string) (api.ExtendedConfig, bool) {
	cfg, ok := c.parsedClusterProviderConfigs[strategy]
	return cfg, ok
}

// GetToolsetConfig returns the parsed toolset_configs entry for name.
func (c *Config) GetToolsetConfig(name string) (api.ExtendedConfig, bool) {
	cfg, ok := c.parsedToolsetConfigs[name]
	return cfg, ok
}

// GetTokenExchangeConfig returns the token-exchange config, or nil when the
// [token_exchange] table was omitted and every field is still at its default.
func (c *Config) GetTokenExchangeConfig() *TokenExchangeConfig {
	if !c.TokenExchange.present && !c.tokenExchangeHasNonDefault() {
		return nil
	}
	return &c.TokenExchange
}

// tokenExchangeHasNonDefault reports whether any [token_exchange] option was
// set from TOML, env, or test rather than Default.
func (c *Config) tokenExchangeHasNonDefault() bool {
	found := false
	walkOptions(&c.TokenExchange, func(o option, _ string) {
		if o.Source() != SourceDefault {
			found = true
		}
	})
	return found
}

// ConfigDirPath returns the directory of the main config file (or the drop-in
// dir when --config-dir is used alone). Used to resolve relative paths in
// extension parsers (e.g. Kiali CA files).
func (c *Config) ConfigDirPath() string { return c.configDirPath }

// ResolveClusterAuthMode returns the effective cluster auth mode.
// If explicitly set, returns that value. Otherwise defaults to passthrough,
// which forwards the Authorization header to the cluster when present
// and falls back to kubeconfig credentials when absent.
func (c *Config) ResolveClusterAuthMode() string {
	if mode := c.ClusterAuthMode.Get(); mode != "" {
		return mode
	}
	return api.ClusterAuthPassthrough
}

// WithProviderStrategies sets the known cluster-provider strategies for
// validation. Callers that have access to the provider registry should chain
// this before Validate so that cluster_provider_strategy is checked.
func (c *Config) WithProviderStrategies(strategies []string) *Config {
	c.providerStrategies = strategies
	return c
}

// WithTokenExchangeStrategies sets the known token exchange strategies for
// validation. Callers that have access to the token exchange registry should
// chain this before Validate so that token_exchange.strategy is checked.
func (c *Config) WithTokenExchangeStrategies(strategies []string) *Config {
	c.tokenExchangeStrategies = strategies
	return c
}

// DocumentedOption is Option metadata for generated docs (TOML / env / reload).
type DocumentedOption struct {
	Path        string
	EnvName     string
	Description string
	Default     string
	Reloadable  bool
	Sensitive   bool
}

// DocumentedOptions returns every Config option in walk order for docs generation.
func DocumentedOptions() []DocumentedOption {
	cfg := New()
	var out []DocumentedOption
	walkOptions(cfg, func(o option, path string) {
		if path == "" {
			return
		}
		out = append(out, DocumentedOption{
			Path:        path,
			EnvName:     o.envName(),
			Description: o.description(),
			Default:     o.defaultString(),
			Reloadable:  o.reloadable(),
			Sensitive:   o.sensitive(),
		})
	})
	return out
}

// ReadConfigOpt customizes a Read / ReadToml load.
type ReadConfigOpt func(*loadSettings)

// loadSettings holds optional load-time knobs (config dir, previous Config for reload).
type loadSettings struct {
	dirPath  string
	previous *Config
}

// WithDirPath returns a ReadConfigOpt that sets the config directory path used
// to resolve relative files in extension parsers.
func WithDirPath(path string) ReadConfigOpt {
	return func(s *loadSettings) { s.dirPath = path }
}

// WithPrevious returns a ReadConfigOpt that pins non-reloadable options to
// their previous values when this load is a SIGHUP reload.
func WithPrevious(prev *Config) ReadConfigOpt {
	return func(s *loadSettings) { s.previous = prev }
}

// KeepPreviousIfDefault copies values from prev onto next when next still has
// SourceDefault. Tests use this after ReadToml so suite SetForTest values
// survive unless TOML or env set the option.
func KeepPreviousIfDefault(prev, next *Config) {
	if prev == nil || next == nil {
		return
	}
	prevs := optionsByPath(prev)
	walkOptions(next, func(o option, path string) {
		p, ok := prevs[path]
		if !ok {
			return
		}
		if o.Source() == SourceDefault && p.Source() != SourceDefault {
			o.keepFrom(p)
		}
	})
}

// optionsByPath indexes every Option on c by its dotted TOML path.
func optionsByPath(c *Config) map[string]option {
	m := map[string]option{}
	walkOptions(c, func(o option, path string) {
		m[path] = o
	})
	return m
}

// applyReadOpts folds ReadConfigOpt values into loadSettings.
func applyReadOpts(opts []ReadConfigOpt) loadSettings {
	var settings loadSettings
	for _, opt := range opts {
		opt(&settings)
	}
	return settings
}

// Read reads the toml file, applies drop-in configs from dropInConfigDir
// (only if that path is non-empty), then applies env.
// Loading order: defaults → files → env. Empty dropInConfigDir means no drop-ins.
func Read(ctx context.Context, configPath, dropInConfigDir string, opts ...ReadConfigOpt) (*Config, error) {
	settings := applyReadOpts(opts)

	var configFiles []string
	var configDir string
	logger := klogutil.FromContext(ctx)

	if configPath != "" {
		logger.V(2).Info("Loading main config", "path", configPath)
		configFiles = append(configFiles, configPath)
		absPath, err := filepath.Abs(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve absolute path to config file: %w", err)
		}
		configDir = filepath.Dir(absPath)
	}

	if dropInConfigDir != "" {
		absDropIn, err := filepath.Abs(dropInConfigDir)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve absolute path to config dir: %w", err)
		}
		dropInConfigDir = absDropIn
		if configDir == "" {
			configDir = dropInConfigDir
		}

		dropInFiles, err := loadDropInConfigs(ctx, dropInConfigDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load drop-in configs from %s: %w", dropInConfigDir, err)
		}
		if len(dropInFiles) == 0 {
			logger.V(2).Info("No drop-in config files found", "config_dir", dropInConfigDir)
		} else {
			logger.V(2).Info("Loading drop-in config file(s)", "num_config_files", len(dropInFiles), "config_dir", dropInConfigDir)
		}
		configFiles = append(configFiles, dropInFiles...)
	}

	merged, sources, err := mergeTOMLFiles(ctx, configFiles)
	if err != nil {
		return nil, err
	}

	if settings.dirPath == "" {
		settings.dirPath = configDir
	}
	return resolve(ctx, merged, sources, settings)
}

// loadDropInConfigs lists config files from a drop-in directory.
// Files are processed in lexical (alphabetical) order.
// Only files with a .toml extension are processed; dotfiles are ignored.
func loadDropInConfigs(ctx context.Context, dropInConfigDir string) ([]string, error) {
	logger := klogutil.FromContext(ctx)
	info, err := os.Stat(dropInConfigDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.V(2).Info("Drop-in config directory does not exist, skipping", "config_dir", dropInConfigDir)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat drop-in directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("drop-in config path is not a directory: %s", dropInConfigDir)
	}
	return getSortedConfigFiles(ctx, dropInConfigDir)
}

// getSortedConfigFiles returns a sorted list of .toml files in the specified directory.
// Dotfiles (starting with '.') and non-.toml files are ignored.
// Files are sorted lexically (alphabetically) by filename.
func getSortedConfigFiles(ctx context.Context, dir string) ([]string, error) {
	logger := klogutil.FromContext(ctx)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			logger.V(4).Info("Skipping dotfile", "file_name", name)
			continue
		}
		if !strings.HasSuffix(name, ".toml") {
			logger.V(4).Info("Skipping non-.toml file", "file_name", name)
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files, nil
}

// mergeTOMLFiles reads and deep-merges TOML files in order, with later files
// overriding earlier ones. sources maps each dotted key to the file that last set it.
func mergeTOMLFiles(ctx context.Context, files []string) (map[string]any, map[string]string, error) {
	merged := map[string]any{}
	sources := map[string]string{}
	for _, file := range files {
		klogutil.FromContext(ctx).V(3).Info("Merging config", "file_name", filepath.Base(file))
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read config %s: %w", file, err)
		}
		dropIn := make(map[string]any)
		if _, err = toml.NewDecoder(bytes.NewReader(data)).Decode(&dropIn); err != nil {
			return nil, nil, fmt.Errorf("failed to decode config %s: %w", file, err)
		}
		deepMerge(merged, dropIn, file, sources, "")
	}
	return merged, sources, nil
}

// deepMerge recursively merges src into dst.
// For nested maps, it merges recursively. For other types, src overwrites dst.
// Each overwritten path is recorded in sources as coming from srcFile.
func deepMerge(dst, src map[string]any, srcFile string, sources map[string]string, prefix string) {
	for key, srcVal := range src {
		path := joinPath(prefix, key)
		if dstVal, exists := dst[key]; exists {
			srcMap, srcIsMap := srcVal.(map[string]any)
			dstMap, dstIsMap := dstVal.(map[string]any)
			if srcIsMap && dstIsMap {
				deepMerge(dstMap, srcMap, srcFile, sources, path)
				continue
			}
		}
		dst[key] = srcVal
		recordSources(sources, path, srcVal, srcFile)
	}
}

// recordSources records srcFile as the origin of path and every nested key under val.
func recordSources(sources map[string]string, path string, val any, srcFile string) {
	if path != "" {
		sources[path] = srcFile
	}
	child, ok := val.(map[string]any)
	if !ok {
		return
	}
	for k, v := range child {
		recordSources(sources, joinPath(path, k), v, srcFile)
	}
}

// ReadToml resolves TOML bytes with env overlays (no files). Used by tests
// and by Complete when neither --config nor --config-dir is set.
func ReadToml(configData []byte, opts ...ReadConfigOpt) (*Config, error) {
	settings := applyReadOpts(opts)
	merged := map[string]any{}
	sources := map[string]string{}
	if len(bytes.TrimSpace(configData)) > 0 {
		if _, err := toml.NewDecoder(bytes.NewReader(configData)).Decode(&merged); err != nil {
			return nil, err
		}
		src := "<toml>"
		if settings.dirPath != "" {
			src = settings.dirPath
		}
		recordSources(sources, "", merged, src)
	}
	return resolve(context.Background(), merged, sources, settings)
}

// resolve applies merged TOML, rejects unknown keys, parses extension tables,
// overlays env, and pins non-reloadable options when settings.previous is set.
func resolve(ctx context.Context, merged map[string]any, sources map[string]string, settings loadSettings) (*Config, error) {
	cfg := New()
	if settings.dirPath != "" {
		cfg.configDirPath = settings.dirPath
	}

	if _, ok := merged["token_exchange"]; ok {
		cfg.TokenExchange.present = true
	}

	if err := applyTOML(cfg, merged, sources); err != nil {
		return nil, err
	}
	if err := rejectUnknownKeys(merged, cfg, sources); err != nil {
		return nil, err
	}
	if err := parseExtensions(ctx, cfg, merged, sources); err != nil {
		return nil, err
	}

	if err := walkApply(cfg, func(o option) error { return o.applyEnv() }); err != nil {
		return nil, err
	}

	if settings.previous != nil {
		pinNonReloadable(ctx, settings.previous, cfg)
	}

	dumpConfig(ctx, cfg, settings.previous)
	return cfg, nil
}

// walkApply runs fn on every Option and prefixes errors with the option path.
func walkApply(cfg *Config, fn func(option) error) error {
	var applyErr error
	walkOptions(cfg, func(o option, path string) {
		if applyErr != nil {
			return
		}
		if err := fn(o); err != nil {
			applyErr = fmt.Errorf("%s: %w", path, err)
		}
	})
	return applyErr
}

// applyTOML copies present TOML keys onto matching Options.
func applyTOML(cfg *Config, merged map[string]any, sources map[string]string) error {
	var applyErr error
	walkOptions(cfg, func(o option, path string) {
		if applyErr != nil || path == "" {
			return
		}
		raw, ok := mapLookup(merged, path)
		if !ok {
			return
		}
		src := Source(sources[path])
		if src == "" {
			src = Source("<file>")
		}
		applyErr = o.applyTOML(raw, src, path)
	})
	return applyErr
}

// mapLookup walks a dotted key path through nested maps.
func mapLookup(m map[string]any, dotted string) (any, bool) {
	parts := strings.Split(dotted, ".")
	var cur any = m
	for _, p := range parts {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := mm[p]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// removedKeyReplacements maps dropped TOML keys to their [token_exchange] replacements.
var removedKeyReplacements = map[string]string{
	"token_exchange_strategy":  "token_exchange.strategy",
	"sts_audience":             "token_exchange.audience",
	"sts_scopes":               "token_exchange.scopes",
	"sts_subject_token_type":   "token_exchange.subject_token_type",
	"sts_requested_token_type": "token_exchange.requested_token_type",
	"sts_client_id":            "token_exchange.client_auth.client_id",
	"sts_client_secret":        "token_exchange.client_auth.client_secret",
	"sts_auth_style":           "token_exchange.client_auth.method",
	"sts_client_cert_file":     "token_exchange.client_auth.certificate_file",
	"sts_client_key_file":      "token_exchange.client_auth.private_key_file",
	"sts_federated_token_file": "token_exchange.client_auth.token_file",
}

// rejectUnknownKeys fails the load if merged TOML contains a key that is not
// an Option, wrapping table, or registered extension table.
func rejectUnknownKeys(merged map[string]any, cfg *Config, sources map[string]string) error {
	optionKeys := map[string]bool{}
	containers := map[string]bool{}
	walkOptions(cfg, func(o option, path string) {
		if path == "" {
			return
		}
		optionKeys[path] = true
		if o.isContainer() {
			containers[path] = true
		}
	})
	wrapping := map[string]bool{}
	for _, t := range wrappingTables(cfg) {
		wrapping[t] = true
	}

	for _, key := range flattenKeys(merged, "") {
		if isKnownKey(key, optionKeys, wrapping, containers) {
			continue
		}
		file := sources[key]
		if file == "" {
			file = "config"
		}
		name := key
		if i := strings.LastIndexByte(key, '.'); i >= 0 {
			name = key[i+1:]
		}
		if replacement, ok := removedKeyReplacements[name]; ok {
			return fmt.Errorf("removed config key %q in %s; use %s", key, file, replacement)
		}
		return fmt.Errorf("unknown config key %q in %s", key, file)
	}
	return nil
}

// isKnownKey reports whether key is an Option path, wrapping table, extension
// table, or a nested key under a container Option.
func isKnownKey(key string, optionKeys, wrapping, containers map[string]bool) bool {
	if optionKeys[key] || wrapping[key] {
		return true
	}
	if key == extensionToolsetTable || strings.HasPrefix(key, extensionToolsetTable+".") {
		return true
	}
	if key == extensionProviderTable || strings.HasPrefix(key, extensionProviderTable+".") {
		return true
	}
	for c := range containers {
		if key == c || strings.HasPrefix(key, c+".") {
			return true
		}
	}
	return false
}

// flattenKeys returns every dotted key in m, including intermediate table paths.
func flattenKeys(m map[string]any, prefix string) []string {
	var keys []string
	for k, v := range m {
		path := joinPath(prefix, k)
		keys = append(keys, path)
		if child, ok := v.(map[string]any); ok {
			keys = append(keys, flattenKeys(child, path)...)
		}
	}
	return keys
}

// parseExtensions decodes cluster_provider_configs and toolset_configs via
// registered parsers.
func parseExtensions(ctx context.Context, cfg *Config, merged map[string]any, sources map[string]string) error {
	ctx = withConfigDirPath(ctx, cfg.configDirPath)
	ctx = withRequireTLS(ctx, cfg.RequireTLS.Get())

	var err error
	if raw, ok := merged[extensionProviderTable].(map[string]any); ok {
		cfg.clusterProviderConfigsSource = Source(sources[extensionProviderTable])
		cfg.parsedClusterProviderConfigs, err = providerConfigRegistry.parseMaps(ctx, extensionProviderTable, raw)
		if err != nil {
			return err
		}
	}
	if raw, ok := merged[extensionToolsetTable].(map[string]any); ok {
		cfg.toolsetConfigsSource = Source(sources[extensionToolsetTable])
		cfg.parsedToolsetConfigs, err = toolsetConfigRegistry.parseMaps(ctx, extensionToolsetTable, raw)
		if err != nil {
			return err
		}
	}
	return nil
}

// pinNonReloadable copies previous values for options that cannot change on SIGHUP.
func pinNonReloadable(ctx context.Context, prev, next *Config) {
	prevs := optionsByPath(prev)
	logger := klogutil.FromContext(ctx)
	walkOptions(next, func(o option, path string) {
		p, ok := prevs[path]
		if !ok || o.reloadable() || o.equalValue(p) {
			return
		}
		klogutil.LogWarn(logger,
			"Ignoring change to non-reloadable option; restart the process to apply it",
			klogutil.Field("option", path),
			klogutil.Field("kept", p.Describe()),
			klogutil.Field("ignored", o.Describe()),
		)
		o.keepFrom(p)
	})
	pinClusterProviderConfigs(ctx, prev, next)
}

// pinClusterProviderConfigs keeps previous cluster_provider_configs on reload.
func pinClusterProviderConfigs(ctx context.Context, prev, next *Config) {
	if equalExtendedMaps(prev.parsedClusterProviderConfigs, next.parsedClusterProviderConfigs) {
		next.clusterProviderConfigsSource = prev.clusterProviderConfigsSource
		return
	}
	klogutil.LogWarn(klogutil.FromContext(ctx),
		"Ignoring change to non-reloadable option; restart the process to apply it",
		klogutil.Field("option", extensionProviderTable),
	)
	next.parsedClusterProviderConfigs = prev.parsedClusterProviderConfigs
	next.clusterProviderConfigsSource = prev.clusterProviderConfigsSource
}

// equalExtendedMaps reports whether a and b contain the same parsed extension configs.
func equalExtendedMaps(a, b map[string]api.ExtendedConfig) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// dumpConfig logs every option at V(1), marking values that changed from prev.
func dumpConfig(ctx context.Context, cfg *Config, prev *Config) {
	logger := klogutil.FromContext(ctx).V(1)
	prevs := map[string]option{}
	if prev != nil {
		prevs = optionsByPath(prev)
	}
	walkOptions(cfg, func(o option, path string) {
		if path == "" {
			return
		}
		keys := []any{"option", path, "value", o.Describe()}
		if p, ok := prevs[path]; ok && !o.equalValue(p) {
			keys = append(keys, "changed", true, "previous", p.Describe())
		}
		logger.Info("config option", keys...)
	})
}
