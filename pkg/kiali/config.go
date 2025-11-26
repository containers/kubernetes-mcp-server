package kiali

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// Config holds Kiali toolset configuration
type Config struct {
	Url                  string `toml:"url"`
	Insecure             bool   `toml:"insecure,omitempty"`
	CertificateAuthority string `toml:"certificate_authority,omitempty"`
}

var _ config.Extended = (*Config)(nil)

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
	return nil
}

// isPEMContent checks if the string appears to be inline PEM content
// (starts with "-----BEGIN" which is the standard PEM header)
func isPEMContent(s string) bool {
	trimmed := strings.TrimSpace(s)
	return strings.HasPrefix(trimmed, "-----BEGIN")
}

func kialiToolsetParser(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (config.Extended, error) {
	var cfg Config
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}

	// If certificate_authority is provided and appears to be a file path (not inline PEM),
	// resolve it relative to the config directory if it's a relative path
	if cfg.CertificateAuthority != "" && !isPEMContent(cfg.CertificateAuthority) {
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
