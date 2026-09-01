package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/tlsutil"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

const (
	DefaultDropInConfigDir = "conf.d"

	// Environment variable names for TLS configuration.
	EnvTLSMinVersion   = "TLS_MIN_VERSION"
	EnvTLSCipherSuites = "TLS_CIPHER_SUITES"
)

// ToolOverride contains per-tool configuration overrides.
type ToolOverride struct {
	Description string `toml:"description,omitempty"`
}

// TokenExchangeConfig is the TOML configuration for global token exchange.
type TokenExchangeConfig struct {
	Strategy           string                   `toml:"strategy"`
	Audience           string                   `toml:"audience"`
	Scopes             []string                 `toml:"scopes,omitempty"`
	SubjectTokenType   string                   `toml:"subject_token_type,omitempty"`
	RequestedTokenType string                   `toml:"requested_token_type,omitempty"`
	ClientAuth         *TokenExchangeClientAuth `toml:"client_auth,omitempty"`
}

func (c *TokenExchangeConfig) GetStrategy() string           { return c.Strategy }
func (c *TokenExchangeConfig) GetAudience() string           { return c.Audience }
func (c *TokenExchangeConfig) GetScopes() []string           { return c.Scopes }
func (c *TokenExchangeConfig) GetSubjectTokenType() string   { return c.SubjectTokenType }
func (c *TokenExchangeConfig) GetRequestedTokenType() string { return c.RequestedTokenType }
func (c *TokenExchangeConfig) GetClientAuth() api.TokenExchangeClientAuth {
	if c.ClientAuth == nil {
		return nil
	}
	return c.ClientAuth
}

// TokenExchangeClientAuth is the TOML configuration for token exchange client authentication.
type TokenExchangeClientAuth struct {
	Method          api.TokenExchangeClientAuthMethod `toml:"method"`
	ClientID        string                            `toml:"client_id,omitempty"`
	ClientSecret    string                            `toml:"client_secret,omitempty"`
	CertificateFile string                            `toml:"certificate_file,omitempty"`
	PrivateKeyFile  string                            `toml:"private_key_file,omitempty"`
	TokenFile       string                            `toml:"token_file,omitempty"`
}

func (c *TokenExchangeClientAuth) GetMethod() api.TokenExchangeClientAuthMethod { return c.Method }
func (c *TokenExchangeClientAuth) GetClientID() string                          { return c.ClientID }
func (c *TokenExchangeClientAuth) GetClientSecret() string                      { return c.ClientSecret }
func (c *TokenExchangeClientAuth) GetCertificateFile() string                   { return c.CertificateFile }
func (c *TokenExchangeClientAuth) GetPrivateKeyFile() string                    { return c.PrivateKeyFile }
func (c *TokenExchangeClientAuth) GetTokenFile() string                         { return c.TokenFile }

// StaticConfig is the configuration for the server.
// It allows to configure server specific settings and tools to be enabled or disabled.
type StaticConfig struct {
	DeniedResources []api.GroupVersionKind `toml:"denied_resources"`

	LogLevel    int    `toml:"log_level,omitzero"`
	LogFile     string `toml:"log_file,omitempty"`
	Port        string `toml:"port,omitempty"`
	BindAddress string `toml:"bind_address,omitempty"`
	KubeConfig  string `toml:"kubeconfig,omitempty"`
	ListOutput  string `toml:"list_output,omitempty"`
	// Stateless configures the MCP server to operate in stateless mode.
	// When true, the server will not send notifications to clients (e.g., tools/list_changed, prompts/list_changed).
	// This is useful for container deployments, load balancing, and serverless environments where
	// maintaining client state is not desired or possible. However, this disables dynamic tool
	// and prompt updates, requiring clients to manually refresh their tool/prompt lists.
	// Defaults to false (stateful mode with notifications enabled).
	Stateless bool `toml:"stateless,omitempty"`
	// When true, expose only tools annotated with readOnlyHint=true
	ReadOnly bool `toml:"read_only,omitempty"`
	// When true, disable tools annotated with destructiveHint=true
	DisableDestructive bool     `toml:"disable_destructive,omitempty"`
	Toolsets           []string `toml:"toolsets,omitempty"`
	// Tool configuration
	EnabledTools  []string                `toml:"enabled_tools,omitempty"`
	DisabledTools []string                `toml:"disabled_tools,omitempty"`
	ToolOverrides map[string]ToolOverride `toml:"tool_overrides,omitempty"`
	// Prompt configuration
	Prompts []api.Prompt `toml:"prompts,omitempty"`

	// Authorization-related fields
	// RequireOAuth indicates whether the server requires OAuth for authentication.
	RequireOAuth bool `toml:"require_oauth,omitempty"`
	// OAuthAudience is the valid audience for the OAuth tokens, used for offline JWT claim validation.
	OAuthAudience string `toml:"oauth_audience,omitempty"`
	// AuthorizationURL is the URL of the OIDC authorization server.
	// It is used for token validation and for STS token exchange.
	AuthorizationURL string `toml:"authorization_url,omitempty"`
	// SkipJWTVerification allows the server to accept JWTs without cryptographic
	// signature verification when require_oauth is enabled but no authorization_url
	// is configured (offline-only validation). Only use behind a trusted reverse proxy
	// that performs token verification. When false (default), the server refuses to
	// start if require_oauth is true and authorization_url is empty.
	SkipJWTVerification bool `toml:"skip_jwt_verification,omitempty"`
	// DisableDynamicClientRegistration indicates whether dynamic client registration is disabled.
	// If true, the .well-known endpoints will not expose the registration endpoint.
	DisableDynamicClientRegistration bool `toml:"disable_dynamic_client_registration,omitempty"`
	// OAuthScopes are the supported **client** scopes requested during the **client/frontend** OAuth flow.
	OAuthScopes []string `toml:"oauth_scopes,omitempty"`
	// TokenExchange configures global token exchange before tokens are passed to the cluster.
	// A missing block leaves OAuth tokens unchanged.
	TokenExchange *TokenExchangeConfig `toml:"token_exchange,omitempty"`
	// ClusterAuthMode determines how the MCP server authenticates to the cluster.
	// Valid values: "passthrough" (forward Authorization header, with optional exchange), "kubeconfig" (use kubeconfig credentials).
	// If empty, defaults to passthrough: forwards the token when present, falls back to kubeconfig when absent.
	ClusterAuthMode      string `toml:"cluster_auth_mode,omitempty"`
	CertificateAuthority string `toml:"certificate_authority,omitempty"`
	ServerURL            string `toml:"server_url,omitempty"`
	// TrustProxyHeaders allows the server to use X-Forwarded-Host, X-Forwarded-Proto,
	// X-Forwarded-For, and X-Real-IP headers from reverse proxies.
	// Only enable this when the server is behind a trusted reverse proxy.
	// When false (default), the server requires server_url to be set for well-known
	// endpoint metadata and ignores forwarded headers for client IP and scheme detection.
	TrustProxyHeaders bool `toml:"trust_proxy_headers,omitempty"`

	// TLS configuration for the HTTP server
	// TLSCert is the path to the TLS certificate file for HTTPS
	TLSCert string `toml:"tls_cert,omitempty"`
	// TLSKey is the path to the TLS private key file for HTTPS
	TLSKey string `toml:"tls_key,omitempty"`
	// RequireTLS enforces TLS for all server and client connections.
	// When true, the server will refuse to start without TLS certificates,
	// and outbound connections to non-HTTPS endpoints will be rejected.
	RequireTLS bool `toml:"require_tls,omitempty"`
	// TLSMinVersion is the minimum TLS version to accept (e.g., "1.2", "1.3").
	// Defaults to TLS 1.2 if not set. Can be overridden by TLS_MIN_VERSION env var.
	TLSMinVersion string `toml:"tls_min_version,omitempty"`
	// TLSCipherSuites is a list of supported cipher suites for TLS connections.
	// If empty, Go's default cipher suites are used. Can be overridden by TLS_CIPHER_SUITES env var.
	TLSCipherSuites []string `toml:"tls_cipher_suites,omitempty"`

	// HTTP server configuration (timeouts, size limits)
	HTTP HTTPConfig `toml:"http,omitempty"`

	// ClusterProviderStrategy is how the server finds clusters.
	// If set to "kubeconfig", the clusters will be loaded from those in the kubeconfig.
	// If set to "in-cluster", the server will use the in cluster config
	ClusterProviderStrategy string `toml:"cluster_provider_strategy,omitempty"`

	// ClusterProvider-specific configurations
	// This map holds raw TOML primitives that will be parsed by registered provider parsers
	ClusterProviderConfigs map[string]toml.Primitive `toml:"cluster_provider_configs,omitempty"`

	// Toolset-specific configurations
	// This map holds raw TOML primitives that will be parsed by registered toolset parsers
	ToolsetConfigs map[string]toml.Primitive `toml:"toolset_configs,omitempty"`

	// Server instructions to be provided by the MCP server to the MCP client
	// This can be used to provide specific instructions on how the client should use the server
	ServerInstructions string `toml:"server_instructions,omitempty"`

	// Telemetry contains OpenTelemetry configuration options.
	// These can also be configured via OTEL_* environment variables.
	Telemetry TelemetryConfig `toml:"telemetry,omitempty"`

	// ValidationEnabled enables pre-execution validation of tool calls.
	// When enabled, validates resources, schemas, and RBAC before execution.
	// Defaults to false.
	ValidationEnabled bool `toml:"validation_enabled,omitempty"`

	// EnableTargetCompatibilityToolFilters enables filtering of tools based on
	// cluster target compatibility (e.g., hiding OpenShift-specific tools when
	// connected to a non-OpenShift cluster). This feature is experimental, and
	// this option is subject to change or removal in a future release.
	// Defaults to false.
	EnableTargetCompatibilityToolFilters bool `toml:"experimental_enable_target_compatibility_tool_filters,omitempty"`

	// ConfirmationFallback is the global default fallback behavior when a client
	// does not support elicitation. Valid values are "deny" and "allow".
	ConfirmationFallback string `toml:"confirmation_fallback,omitempty"`
	// ConfirmationRules define rules for prompting the user before dangerous actions.
	ConfirmationRules []api.ConfirmationRule `toml:"confirmation_rules,omitempty"`

	// Internal: parsed provider configs (not exposed to TOML package)
	parsedClusterProviderConfigs map[string]api.ExtendedConfig
	// Internal: parsed toolset configs (not exposed to TOML package)
	parsedToolsetConfigs map[string]api.ExtendedConfig

	// Internal: the config.toml directory, to help resolve relative file paths
	configDirPath string
	// Internal: known provider strategies, set via WithProviderStrategies
	providerStrategies []string
	// Internal: known token exchange strategies, set via WithTokenExchangeStrategies
	tokenExchangeStrategies []string
}

var _ api.BaseConfig = (*StaticConfig)(nil)

type ReadConfigOpt func(cfg *StaticConfig)

// WithDirPath returns a ReadConfigOpt that sets the config directory path.
func WithDirPath(path string) ReadConfigOpt {
	return func(cfg *StaticConfig) {
		cfg.configDirPath = path
	}
}

// Read reads the toml file, applies drop-in configs from configDir (if provided),
// and returns the StaticConfig with any opts applied.
// Loading order: defaults → main config file → drop-in files (lexically sorted)
func Read(ctx context.Context, configPath, dropInConfigDir string) (*StaticConfig, error) {
	var configFiles []string
	var configDir string

	logger := klogutil.FromContext(ctx)

	// Main config file
	if configPath != "" {
		logger.V(2).Info("Loading main config", "path", configPath)
		configFiles = append(configFiles, configPath)

		// get and save the absolute dir path to the config file, so that other config parsers can use it
		absPath, err := filepath.Abs(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve absolute path to config file: %w", err)
		}
		configDir = filepath.Dir(absPath)
	}

	// Drop-in config files
	if dropInConfigDir == "" {
		dropInConfigDir = DefaultDropInConfigDir
	}

	// Resolve drop-in config directory path (relative paths are resolved against config directory)
	if configDir != "" && !filepath.IsAbs(dropInConfigDir) {
		dropInConfigDir = filepath.Join(configDir, dropInConfigDir)
	}

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

	// Read and merge all config files
	configData, err := readAndMergeFiles(ctx, configFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to read and merge config files: %w", err)
	}

	return ReadToml(configData, WithDirPath(configDir))
}

// loadDropInConfigs loads and merges config files from a drop-in directory.
// Files are processed in lexical (alphabetical) order.
// Only files with .toml extension are processed; dotfiles are ignored.
func loadDropInConfigs(ctx context.Context, dropInConfigDir string) ([]string, error) {
	logger := klogutil.FromContext(ctx)
	// Check if directory exists
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

	// Get all .toml files in the directory
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
		// Skip directories
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip dotfiles
		if strings.HasPrefix(name, ".") {
			logger.V(4).Info("Skipping dotfile", "file_name", name)
			continue
		}

		// Only process .toml files
		if !strings.HasSuffix(name, ".toml") {
			logger.V(4).Info("Skipping non-.toml file", "file_name", name)
			continue
		}

		files = append(files, filepath.Join(dir, name))
	}

	// Sort lexically
	sort.Strings(files)

	return files, nil
}

// readAndMergeFiles reads and merges multiple TOML config files into a single byte slice.
// Files are merged in the order provided, with later files overriding earlier ones.
func readAndMergeFiles(ctx context.Context, files []string) ([]byte, error) {
	rawConfig := map[string]interface{}{}
	// Merge each file in order using deep merge
	for _, file := range files {
		klogutil.FromContext(ctx).V(3).Info("Merging config", "file_name", filepath.Base(file))
		configData, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read config %s: %w", file, err)
		}

		dropInConfig := make(map[string]interface{})
		if _, err = toml.NewDecoder(bytes.NewReader(configData)).Decode(&dropInConfig); err != nil {
			return nil, fmt.Errorf("failed to decode config %s: %w", file, err)
		}

		deepMerge(rawConfig, dropInConfig)
	}

	bufferedConfig := new(bytes.Buffer)
	if err := toml.NewEncoder(bufferedConfig).Encode(rawConfig); err != nil {
		return nil, fmt.Errorf("failed to encode merged config: %w", err)
	}
	return bufferedConfig.Bytes(), nil
}

// deepMerge recursively merges src into dst.
// For nested maps, it merges recursively. For other types, src overwrites dst.
func deepMerge(dst, src map[string]interface{}) {
	for key, srcVal := range src {
		if dstVal, exists := dst[key]; exists {
			// Both have this key - check if both are maps for recursive merge
			srcMap, srcIsMap := srcVal.(map[string]interface{})
			dstMap, dstIsMap := dstVal.(map[string]interface{})
			if srcIsMap && dstIsMap {
				deepMerge(dstMap, srcMap)
				continue
			}
		}
		// Either key doesn't exist in dst, or values aren't both maps - overwrite
		dst[key] = srcVal
	}
}

// ReadToml reads the toml data, loads and applies drop-in configs from configDir (if provided),
// and returns the StaticConfig with any opts applied.
// Loading order: defaults → main config file → drop-in files (lexically sorted)
func ReadToml(configData []byte, opts ...ReadConfigOpt) (*StaticConfig, error) {
	config := Default()
	md, err := toml.NewDecoder(bytes.NewReader(configData)).Decode(config)
	if err != nil {
		return nil, err
	}
	if err := validateRemovedTokenExchangeKeys(md); err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(config)
	}

	ctx := withConfigDirPath(context.Background(), config.configDirPath)
	ctx = withRequireTLS(ctx, config.RequireTLS)

	config.parsedClusterProviderConfigs, err = providerConfigRegistry.parse(ctx, md, config.ClusterProviderConfigs)
	if err != nil {
		return nil, err
	}

	config.parsedToolsetConfigs, err = toolsetConfigRegistry.parse(ctx, md, config.ToolsetConfigs)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (c *StaticConfig) GetClusterProviderStrategy() string {
	return c.ClusterProviderStrategy
}

func (c *StaticConfig) GetDeniedResources() []api.GroupVersionKind {
	return c.DeniedResources
}

func (c *StaticConfig) GetKubeConfigPath() string {
	return c.KubeConfig
}

func (c *StaticConfig) GetProviderConfig(strategy string) (api.ExtendedConfig, bool) {
	cfg, ok := c.parsedClusterProviderConfigs[strategy]

	return cfg, ok
}

func (c *StaticConfig) GetToolsetConfig(name string) (api.ExtendedConfig, bool) {
	cfg, ok := c.parsedToolsetConfigs[name]
	return cfg, ok
}

func (c *StaticConfig) GetTokenExchangeConfig() api.TokenExchangeConfig {
	if c.TokenExchange == nil {
		return nil
	}
	return c.TokenExchange
}

func (c *StaticConfig) GetCertificateAuthority() string {
	return c.CertificateAuthority
}

func (c *StaticConfig) IsValidationEnabled() bool {
	return c.ValidationEnabled
}

func (c *StaticConfig) IsTargetCompatibilityToolFiltersEnabled() bool {
	return c.EnableTargetCompatibilityToolFilters
}

func (c *StaticConfig) GetConfirmationRules() []api.ConfirmationRule {
	return c.ConfirmationRules
}

func (c *StaticConfig) GetConfirmationFallback() string {
	return c.ConfirmationFallback
}

func (c *StaticConfig) IsRequireTLS() bool {
	return c.RequireTLS
}

// GetTLSMinVersionConfig returns the effective tls_min_version, with TLS_MIN_VERSION
// env var taking precedence over the TOML/CLI value.
func (c *StaticConfig) GetTLSMinVersionConfig() string {
	if envValue := os.Getenv(EnvTLSMinVersion); envValue != "" {
		return envValue
	}
	return c.TLSMinVersion
}

// GetTLSCipherSuitesConfig returns the effective tls_cipher_suites, with
// TLS_CIPHER_SUITES env var taking precedence over the TOML/CLI value.
func (c *StaticConfig) GetTLSCipherSuitesConfig() []string {
	if envValue := os.Getenv(EnvTLSCipherSuites); envValue != "" {
		suites := strings.Split(envValue, ",")
		for i, suite := range suites {
			suites[i] = strings.TrimSpace(suite)
		}
		return suites
	}
	return c.TLSCipherSuites
}

func (c *StaticConfig) IsRequireOAuth() bool {
	return c.RequireOAuth
}

// WithProviderStrategies sets the known cluster-provider strategies for
// validation. Callers that have access to the provider registry should chain
// this before Validate so that cluster_provider_strategy is checked:
//
//	cfg.WithProviderStrategies(kubernetes.GetRegisteredStrategies()).Validate()
func (c *StaticConfig) WithProviderStrategies(strategies []string) *StaticConfig {
	c.providerStrategies = strategies
	return c
}

// WithTokenExchangeStrategies sets the known token exchange strategies for
// validation. Callers that have access to the token exchange registry should
// chain this before Validate so that token_exchange_strategy is checked:
//
//	cfg.WithTokenExchangeStrategies(tokenexchange.GetRegisteredStrategies()).Validate()
func (c *StaticConfig) WithTokenExchangeStrategies(strategies []string) *StaticConfig {
	c.tokenExchangeStrategies = strategies
	return c
}

// Validate validates config-level invariants that must hold at both startup and
// on SIGHUP reload.
func (c *StaticConfig) Validate(ctx context.Context) error {
	// Normalize whitespace-padded fields before any checks use them.
	c.CertificateAuthority = strings.TrimSpace(c.CertificateAuthority)
	c.TLSCert = strings.TrimSpace(c.TLSCert)
	c.TLSKey = strings.TrimSpace(c.TLSKey)
	c.normalizeTokenExchange()
	if output.FromString(c.ListOutput) == nil {
		return fmt.Errorf("invalid output name: %s, valid names are: %s", c.ListOutput, strings.Join(output.Names, ", "))
	}
	if err := toolsets.Validate(c.Toolsets); err != nil {
		return err
	}
	if c.ClusterProviderStrategy != "" && len(c.providerStrategies) > 0 {
		if !slices.Contains(c.providerStrategies, c.ClusterProviderStrategy) {
			return fmt.Errorf("invalid cluster-provider: %s, valid values are: %s", c.ClusterProviderStrategy, strings.Join(c.providerStrategies, ", "))
		}
	}
	if !c.RequireOAuth && (c.OAuthAudience != "" || c.AuthorizationURL != "" || c.ServerURL != "" || c.CertificateAuthority != "") {
		return fmt.Errorf("oauth-audience, authorization-url, server-url and certificate-authority are only valid if require-oauth is enabled. Missing --port may implicitly set require-oauth to false")
	}
	if c.AuthorizationURL != "" {
		u, err := url.Parse(c.AuthorizationURL)
		if err != nil {
			return err
		}
		if u.Scheme != "https" && u.Scheme != "http" {
			return fmt.Errorf("--authorization-url must be a valid URL")
		}
		if u.Scheme == "http" {
			klogutil.LogWarn(
				klogutil.FromContext(ctx),
				"authorization-url is using insecure scheme, this is not recommended production use",
				klogutil.Field("url.scheme", "http"),
			)
		}
	}
	if err := c.validateSkipJWTVerification(ctx); err != nil {
		return err
	}
	if c.CertificateAuthority != "" {
		if _, err := os.Stat(c.CertificateAuthority); err != nil {
			return fmt.Errorf("certificate-authority must be a valid file path: %w", err)
		}
	}
	if (c.TLSCert != "" && c.TLSKey == "") || (c.TLSCert == "" && c.TLSKey != "") {
		return fmt.Errorf("both --tls-cert and --tls-key must be provided together")
	}
	if c.TLSCert != "" {
		if _, err := os.Stat(c.TLSCert); err != nil {
			return fmt.Errorf("tls-cert must be a valid file path: %w", err)
		}
	}
	if c.TLSKey != "" {
		if _, err := os.Stat(c.TLSKey); err != nil {
			return fmt.Errorf("tls-key must be a valid file path: %w", err)
		}
	}
	if err := c.validateTLSSettings(ctx); err != nil {
		return err
	}
	if err := c.ValidateRequireTLS(); err != nil {
		return err
	}
	if err := c.ValidateClusterAuthMode(); err != nil {
		return err
	}
	if err := c.validateTokenExchange(); err != nil {
		return err
	}
	if err := c.validateConfirmation(); err != nil {
		return err
	}
	if err := c.HTTP.Validate(); err != nil {
		return err
	}
	return nil
}

// validateConfirmation validates confirmation-related fields:
//   - confirmation_fallback must be "allow", "deny", or empty
//   - each entry in confirmation_rules must be well-formed
//     (tool-level xor kube-level, with at least one classifying field)
func (c *StaticConfig) validateConfirmation() error {
	if fb := c.ConfirmationFallback; fb != "" && fb != "allow" && fb != "deny" {
		return fmt.Errorf("invalid confirmation_fallback %q: must be \"allow\" or \"deny\"", fb)
	}
	var ruleErrors []error
	for i := range c.ConfirmationRules {
		if ruleErr := c.ConfirmationRules[i].Validate(); ruleErr != nil {
			ruleErrors = append(ruleErrors, fmt.Errorf("confirmation_rules[%d]: %w", i, ruleErr))
		}
	}
	if len(ruleErrors) > 0 {
		return fmt.Errorf("invalid confirmation rules:\n%w", errors.Join(ruleErrors...))
	}
	return nil
}

// validateSkipJWTVerification checks that the user has explicitly opted in to
// skipping JWT signature verification when require_oauth is enabled but no
// authorization_url is configured.
func (c *StaticConfig) validateSkipJWTVerification(ctx context.Context) error {
	if !c.RequireOAuth || c.AuthorizationURL != "" {
		return nil
	}
	if c.SkipJWTVerification {
		klogutil.LogWarn(klogutil.FromContext(ctx),
			"skip_jwt_verification is enabled with no authorization_url: bearer tokens will be forwarded without any local validation. "+
				"The cluster (or a trusted upstream) is the sole authority. Only use this when cluster_auth_mode=passthrough and the cluster validates tokens directly.")
		return nil
	}
	return fmt.Errorf("require_oauth is enabled but authorization_url is not configured: " +
		"JWTs cannot be cryptographically verified without an OIDC provider. " +
		"Set authorization_url to an OIDC issuer, or set skip_jwt_verification=true " +
		"if the server is behind a trusted reverse proxy that verifies tokens")
}

func (c *StaticConfig) validateTokenExchange() error {
	if c.TokenExchange == nil {
		return nil
	}
	if c.AuthorizationURL == "" {
		return fmt.Errorf("token exchange requires authorization_url to discover the token endpoint")
	}
	strategies := c.tokenExchangeStrategies
	if len(strategies) == 0 {
		strategies = tokenexchange.GetRegisteredStrategies()
	}
	if c.TokenExchange.Strategy == "" || !slices.Contains(strategies, c.TokenExchange.Strategy) {
		return fmt.Errorf("invalid token_exchange.strategy %q: valid values are: %s", c.TokenExchange.Strategy, strings.Join(strategies, ", "))
	}
	auth := c.TokenExchange.ClientAuth
	if auth == nil || !authConfigured(auth) {
		return nil
	}
	if auth.Method == "" {
		return fmt.Errorf("token_exchange.client_auth.method is required when client authentication fields are configured")
	}
	if auth.ClientID == "" {
		return fmt.Errorf("token_exchange.client_auth.client_id is required when method is %q", auth.Method)
	}
	switch auth.Method {
	case api.TokenExchangeClientAuthMethodSecretBasic, api.TokenExchangeClientAuthMethodSecretPost:
		if auth.ClientSecret == "" {
			return fmt.Errorf("token_exchange.client_auth.client_secret is required when method is %q", auth.Method)
		}
	case api.TokenExchangeClientAuthMethodPrivateKey:
		if err := validateTokenExchangeFile("certificate_file", auth.CertificateFile); err != nil {
			return err
		}
		if err := validateTokenExchangeFile("private_key_file", auth.PrivateKeyFile); err != nil {
			return err
		}
	case api.TokenExchangeClientAuthMethodJWTFile:
		if err := validateTokenExchangeFile("token_file", auth.TokenFile); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid token_exchange.client_auth.method %q: must be client_secret_basic, client_secret_post, private_key_jwt, or jwt_file", auth.Method)
	}
	return nil
}

// validateTLSSettings validates TLS settings via the getters so env overrides are included,
// and logs once at startup/reload when TLS env vars override TOML.
func (c *StaticConfig) validateTLSSettings(ctx context.Context) error {
	logger := klogutil.FromContext(ctx).V(1)
	if os.Getenv(EnvTLSMinVersion) != "" {
		klogutil.LogInfo(logger, "TLS min version overridden by environment variable",
			klogutil.Field("env", EnvTLSMinVersion),
			klogutil.Field("value", c.GetTLSMinVersionConfig()),
		)
	}
	if os.Getenv(EnvTLSCipherSuites) != "" {
		klogutil.LogInfo(logger, "TLS cipher suites overridden by environment variable",
			klogutil.Field("env", EnvTLSCipherSuites),
		)
	}

	minVersion := c.GetTLSMinVersionConfig()
	if _, err := tlsutil.ParseTLSVersion(minVersion); err != nil {
		return err
	}
	cipherSuites := c.GetTLSCipherSuitesConfig()
	if _, err := tlsutil.ParseTLSCipherSuites(cipherSuites); err != nil {
		return err
	}
	return nil
}

// ValidateRequireTLS validates outbound URL schemes when RequireTLS is enabled.
// Called at startup (root.go Validate) and on config reload (ReloadConfiguration).
func (c *StaticConfig) ValidateRequireTLS() error {
	if !c.RequireTLS {
		return nil
	}
	return ValidateURLsRequireTLS(map[string]string{
		"authorization_url": c.AuthorizationURL,
		"server_url":        c.ServerURL,
	})
}

func (c *StaticConfig) GetClusterAuthMode() string {
	return c.ClusterAuthMode
}

// ResolveClusterAuthMode returns the effective cluster auth mode.
// If explicitly set, returns that value. Otherwise defaults to passthrough,
// which forwards the Authorization header to the cluster when present
// and falls back to kubeconfig credentials when absent.
func (c *StaticConfig) ResolveClusterAuthMode() string {
	if c.ClusterAuthMode != "" {
		return c.ClusterAuthMode
	}
	return api.ClusterAuthPassthrough
}

// ValidateClusterAuthMode validates cluster_auth_mode and its interaction with
// other auth-related settings (require_oauth, token exchange).
func (c *StaticConfig) ValidateClusterAuthMode() error {
	if c.ClusterAuthMode != "" && c.ClusterAuthMode != api.ClusterAuthPassthrough && c.ClusterAuthMode != api.ClusterAuthKubeconfig {
		return fmt.Errorf("invalid cluster_auth_mode %q: must be %q or %q", c.ClusterAuthMode, api.ClusterAuthPassthrough, api.ClusterAuthKubeconfig)
	}
	if c.ClusterAuthMode == api.ClusterAuthKubeconfig && c.RequireOAuth {
		return fmt.Errorf("cluster_auth_mode %q is not compatible with require_oauth=true: all authenticated users would share a single cluster identity, breaking per-user audit trails; use passthrough or token exchange to preserve user identity on the cluster", api.ClusterAuthKubeconfig)
	}
	hasTokenExchange := c.TokenExchange != nil
	if c.ClusterAuthMode == api.ClusterAuthKubeconfig && hasTokenExchange {
		return fmt.Errorf("token_exchange is incompatible with cluster_auth_mode %q (exchanged token would be unused)", api.ClusterAuthKubeconfig)
	}
	if !c.RequireOAuth && hasTokenExchange {
		return fmt.Errorf("token exchange requires require_oauth=true (token exchange depends on OAuth-validated tokens)")
	}
	return nil
}

func (c *StaticConfig) normalizeTokenExchange() {
	if c.TokenExchange == nil {
		return
	}
	c.TokenExchange.Strategy = strings.TrimSpace(c.TokenExchange.Strategy)
	c.TokenExchange.Audience = strings.TrimSpace(c.TokenExchange.Audience)
	c.TokenExchange.SubjectTokenType = strings.TrimSpace(c.TokenExchange.SubjectTokenType)
	c.TokenExchange.RequestedTokenType = strings.TrimSpace(c.TokenExchange.RequestedTokenType)
	if auth := c.TokenExchange.ClientAuth; auth != nil {
		auth.Method = api.TokenExchangeClientAuthMethod(strings.TrimSpace(string(auth.Method)))
		auth.ClientID = strings.TrimSpace(auth.ClientID)
		auth.ClientSecret = strings.TrimSpace(auth.ClientSecret)
		auth.CertificateFile = strings.TrimSpace(auth.CertificateFile)
		auth.PrivateKeyFile = strings.TrimSpace(auth.PrivateKeyFile)
		auth.TokenFile = strings.TrimSpace(auth.TokenFile)
	}
}

func authConfigured(auth *TokenExchangeClientAuth) bool {
	return auth.Method != "" || auth.ClientID != "" || auth.ClientSecret != "" || auth.CertificateFile != "" || auth.PrivateKeyFile != "" || auth.TokenFile != ""
}

func validateTokenExchangeFile(name, path string) error {
	if path == "" {
		return fmt.Errorf("token_exchange.client_auth.%s is required", name)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("token_exchange.client_auth.%s must be a valid file path: %w", name, err)
	}
	return nil
}

func validateRemovedTokenExchangeKeys(md toml.MetaData) error {
	locations := map[string]string{
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
	var removed []string
	for _, key := range md.Undecoded() {
		if replacement, ok := locations[key.String()]; ok {
			removed = append(removed, key.String()+" -> "+replacement)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	sort.Strings(removed)
	return fmt.Errorf("removed token exchange configuration keys: %s", strings.Join(removed, ", "))
}
