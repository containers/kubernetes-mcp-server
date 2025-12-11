package kiali

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// Config holds Kiali toolset configuration
type Config struct {
	Url                  string `toml:"url"`
	Insecure             bool   `toml:"insecure,omitempty"`
	CertificateAuthority string `toml:"certificate_authority,omitempty"`
}

var _ api.ExtendedConfig = (*Config)(nil)

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("kiali config is nil")
	}
	if c.Url == "" {
		return errors.New("url is required")
	}
	if u, err := url.Parse(c.Url); err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("url must be a valid URL")
	}
	u, _ := url.Parse(c.Url)
	if strings.EqualFold(u.Scheme, "https") && !c.Insecure && strings.TrimSpace(c.CertificateAuthority) == "" {
		return errors.New("certificate_authority is required for https when insecure is false")
	}
	// Validate that certificate_authority is a valid file
	if caValue := strings.TrimSpace(c.CertificateAuthority); caValue != "" {
		if _, err := os.Stat(caValue); err != nil {
			return fmt.Errorf("certificate_authority must be a valid file path: %w", err)
		}
	}
	return nil
}

func kialiToolsetParser(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
	var cfg Config
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}

	// If certificate_authority is provided, resolve it relative to the config directory if it's a relative path
	if cfg.CertificateAuthority != "" {
		configDir := config.ConfigDirPathFromContext(ctx)
		if configDir != "" && !filepath.IsAbs(cfg.CertificateAuthority) {
			cfg.CertificateAuthority = filepath.Join(configDir, cfg.CertificateAuthority)
		}
		// If it's already absolute or configDir is empty, use as-is
	}

	return &cfg, nil
}

func init() {
	config.RegisterToolsetConfig("kiali", kialiToolsetParser)
}
