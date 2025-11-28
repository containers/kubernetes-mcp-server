package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"k8s.io/klog/v2"
)

const (
	ClusterProviderKubeConfig = "kubeconfig"
	ClusterProviderInCluster  = "in-cluster"
	ClusterProviderDisabled   = "disabled"
)

// StaticConfig is the configuration for the server.
// It allows to configure server specific settings and tools to be enabled or disabled.
type StaticConfig struct {
	DeniedResources []GroupVersionKind `toml:"denied_resources"`

	LogLevel   int    `toml:"log_level,omitzero"`
	Port       string `toml:"port,omitempty"`
	SSEBaseURL string `toml:"sse_base_url,omitempty"`
	KubeConfig string `toml:"kubeconfig,omitempty"`
	ListOutput string `toml:"list_output,omitempty"`
	// When true, expose only tools annotated with readOnlyHint=true
	ReadOnly bool `toml:"read_only,omitempty"`
	// When true, disable tools annotated with destructiveHint=true
	DisableDestructive bool     `toml:"disable_destructive,omitempty"`
	Toolsets           []string `toml:"toolsets,omitempty"`
	EnabledTools       []string `toml:"enabled_tools,omitempty"`
	DisabledTools      []string `toml:"disabled_tools,omitempty"`

	// Authorization-related fields
	// RequireOAuth indicates whether the server requires OAuth for authentication.
	RequireOAuth bool `toml:"require_oauth,omitempty"`
	// OAuthAudience is the valid audience for the OAuth tokens, used for offline JWT claim validation.
	OAuthAudience string `toml:"oauth_audience,omitempty"`
	// ValidateToken indicates whether the server should validate the token against the Kubernetes API Server using TokenReview.
	ValidateToken bool `toml:"validate_token,omitempty"`
	// AuthorizationURL is the URL of the OIDC authorization server.
	// It is used for token validation and for STS token exchange.
	AuthorizationURL string `toml:"authorization_url,omitempty"`
	// DisableDynamicClientRegistration indicates whether dynamic client registration is disabled.
	// If true, the .well-known endpoints will not expose the registration endpoint.
	DisableDynamicClientRegistration bool `toml:"disable_dynamic_client_registration,omitempty"`
	// OAuthScopes are the supported **client** scopes requested during the **client/frontend** OAuth flow.
	OAuthScopes []string `toml:"oauth_scopes,omitempty"`
	// StsClientId is the OAuth client ID used for backend token exchange
	StsClientId string `toml:"sts_client_id,omitempty"`
	// StsClientSecret is the OAuth client secret used for backend token exchange
	StsClientSecret string `toml:"sts_client_secret,omitempty"`
	// StsAudience is the audience for the STS token exchange.
	StsAudience string `toml:"sts_audience,omitempty"`
	// StsScopes is the scopes for the STS token exchange.
	StsScopes            []string `toml:"sts_scopes,omitempty"`
	CertificateAuthority string   `toml:"certificate_authority,omitempty"`
	ServerURL            string   `toml:"server_url,omitempty"`
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

	// Internal: parsed provider configs (not exposed to TOML package)
	parsedClusterProviderConfigs map[string]Extended
	// Internal: parsed toolset configs (not exposed to TOML package)
	parsedToolsetConfigs map[string]Extended

	// Internal: the config.toml directory, to help resolve relative file paths
	configDirPath string
}

type GroupVersionKind struct {
	Group   string `toml:"group"`
	Version string `toml:"version"`
	Kind    string `toml:"kind,omitempty"`
}

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
func Read(configPath string, configDir string, opts ...ReadConfigOpt) (*StaticConfig, error) {
	// Start with defaults
	cfg := Default()

	// Get the absolute dir path for the main config file
	var dirPath string
	if configPath != "" {
		// get and save the absolute dir path to the config file, so that other config parsers can use it
		absPath, err := filepath.Abs(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve absolute path to config file: %w", err)
		}
		dirPath = filepath.Dir(absPath)

		// Load main config file
		klog.V(2).Infof("Loading main config from: %s", configPath)
		if err := mergeConfigFile(cfg, configPath, append(opts, WithDirPath(dirPath))...); err != nil {
			return nil, fmt.Errorf("failed to load main config file %s: %w", configPath, err)
		}
	}

	// Load drop-in config files if directory is specified
	if configDir != "" {
		if err := loadDropInConfigs(cfg, configDir, append(opts, WithDirPath(dirPath))...); err != nil {
			return nil, fmt.Errorf("failed to load drop-in configs from %s: %w", configDir, err)
		}
	}

	return cfg, nil
}

// mergeConfigFile reads a config file and merges its values into the target config.
// Values present in the file will overwrite existing values in cfg.
// Values not present in the file will remain unchanged in cfg.
func mergeConfigFile(cfg *StaticConfig, filePath string, opts ...ReadConfigOpt) error {
	configData, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	md, err := toml.NewDecoder(bytes.NewReader(configData)).Decode(cfg)
	if err != nil {
		return fmt.Errorf("failed to decode TOML: %w", err)
	}

	for _, opt := range opts {
		opt(cfg)
	}

	ctx := withConfigDirPath(context.Background(), cfg.configDirPath)

	cfg.parsedClusterProviderConfigs, err = providerConfigRegistry.parse(ctx, md, cfg.ClusterProviderConfigs)
	if err != nil {
		return err
	}

	cfg.parsedToolsetConfigs, err = toolsetConfigRegistry.parse(ctx, md, cfg.ToolsetConfigs)
	if err != nil {
		return err
	}

	return nil
}

// loadDropInConfigs loads and merges config files from a drop-in directory.
// Files are processed in lexical (alphabetical) order.
// Only files with .toml extension are processed; dotfiles are ignored.
func loadDropInConfigs(cfg *StaticConfig, dropInDir string, opts ...ReadConfigOpt) error {
	// Check if directory exists
	info, err := os.Stat(dropInDir)
	if err != nil {
		if os.IsNotExist(err) {
			klog.V(2).Infof("Drop-in config directory does not exist, skipping: %s", dropInDir)
			return nil
		}
		return fmt.Errorf("failed to stat drop-in directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("drop-in config path is not a directory: %s", dropInDir)
	}

	// Get all .toml files in the directory
	files, err := getSortedConfigFiles(dropInDir)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		klog.V(2).Infof("No drop-in config files found in: %s", dropInDir)
		return nil
	}

	klog.V(2).Infof("Loading %d drop-in config file(s) from: %s", len(files), dropInDir)

	// Merge each file in order
	for _, file := range files {
		klog.V(3).Infof("  - Merging drop-in config: %s", filepath.Base(file))
		if err := mergeConfigFile(cfg, file, opts...); err != nil {
			return fmt.Errorf("failed to merge drop-in config %s: %w", file, err)
		}
	}

	return nil
}

// getSortedConfigFiles returns a sorted list of .toml files in the specified directory.
// Dotfiles (starting with '.') and non-.toml files are ignored.
// Files are sorted lexically (alphabetically) by filename.
func getSortedConfigFiles(dir string) ([]string, error) {
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
			klog.V(4).Infof("Skipping dotfile: %s", name)
			continue
		}

		// Only process .toml files
		if !strings.HasSuffix(name, ".toml") {
			klog.V(4).Infof("Skipping non-.toml file: %s", name)
			continue
		}

		files = append(files, filepath.Join(dir, name))
	}

	// Sort lexically
	sort.Strings(files)

	return files, nil
}

// ReadToml reads the toml data and returns the StaticConfig, with any opts applied
func ReadToml(configData []byte, opts ...ReadConfigOpt) (*StaticConfig, error) {
	config := Default()
	md, err := toml.NewDecoder(bytes.NewReader(configData)).Decode(config)
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(config)
	}

	ctx := withConfigDirPath(context.Background(), config.configDirPath)

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

func (c *StaticConfig) GetProviderConfig(strategy string) (Extended, bool) {
	config, ok := c.parsedClusterProviderConfigs[strategy]

	return config, ok
}

func (c *StaticConfig) GetToolsetConfig(name string) (Extended, bool) {
	cfg, ok := c.parsedToolsetConfigs[name]
	return cfg, ok
}
