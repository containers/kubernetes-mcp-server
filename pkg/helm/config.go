package helm

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

const DefaultTimeout = 5 * time.Minute

// Config holds Helm toolset configuration
type Config struct {
	AllowedRegistries []string `toml:"allowed_registries,omitempty"`
	StorageDriver     string   `toml:"storage_driver,omitempty"`
	// Equivalent to the helm CLI --timeout flag. Defaults to 5m when unset.
	Timeout string `toml:"timeout,omitempty"`
	// Equivalent to the helm CLI --wait flag on `helm install`. Defaults to true when unset
	Wait *bool `toml:"wait,omitempty"`
	// Equivalent to the helm CLI --wait flag on `helm uninstall`. Defaults to true when unset.
	UninstallWait *bool `toml:"uninstall_wait,omitempty"`
}

var _ api.ExtendedConfig = (*Config)(nil)

func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("helm config is nil")
	}
	for i, entry := range c.AllowedRegistries {
		u, err := url.Parse(entry)
		if err != nil || u.Scheme == "" {
			return fmt.Errorf("allowed_registries entry %q must be a valid URL with scheme and host", entry)
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "oci" && scheme != "https" {
			return fmt.Errorf("allowed_registries entry %q must use oci:// or https:// scheme", entry)
		}
		if u.Host == "" {
			return fmt.Errorf("allowed_registries entry %q must be a valid URL with scheme and host", entry)
		}
		// Normalize to lowercase scheme + host and strip trailing slashes
		// so runtime comparison against the normalized chart reference is case-insensitive.
		c.AllowedRegistries[i] = strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
	}
	if c.StorageDriver != "" {
		// Normalize to lowercase
		c.StorageDriver = strings.ToLower(c.StorageDriver)
		if c.StorageDriver != "secret" && c.StorageDriver != "configmap" {
			return fmt.Errorf("unsupported Helm storage driver %q: must be \"secret\" or \"configmap\"", c.StorageDriver)
		}
	}
	if c.Timeout != "" {
		d, err := time.ParseDuration(c.Timeout)
		if err != nil {
			return fmt.Errorf("invalid Helm timeout %q: %v", c.Timeout, err)
		}
		if d <= 0 {
			return fmt.Errorf("invalid Helm timeout %q: must be positive", c.Timeout)
		}
	}

	return nil
}

// TimeoutOrDefault returns the timeout to wait for resource readiness during
// blocking helm operations, or DefaultTimeout when unset or invalid.
func (c *Config) TimeoutOrDefault() time.Duration {
	if c == nil || c.Timeout == "" {
		return DefaultTimeout
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil || d <= 0 {
		return DefaultTimeout
	}
	return d
}

// WaitOrDefault returns whether install waits for resource readiness, or true
// when unset.
func (c *Config) WaitOrDefault() bool {
	if c == nil || c.Wait == nil {
		return true
	}
	return *c.Wait
}

// UninstallWaitOrDefault returns whether uninstall waits for the release's
// resources to be deleted, or true when unset.
func (c *Config) UninstallWaitOrDefault() bool {
	if c == nil || c.UninstallWait == nil {
		return true
	}
	return *c.UninstallWait
}

func helmToolsetParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
	var cfg Config
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func init() {
	config.RegisterToolsetConfig("helm", helmToolsetParser)
}
