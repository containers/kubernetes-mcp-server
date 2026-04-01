package helm

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// Config holds Helm toolset configuration
type Config struct {
	AllowedRegistries []string `toml:"allowed_registries,omitempty"`
	StorageDriver     string   `toml:"storage_driver,omitempty"`
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

	return nil
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
