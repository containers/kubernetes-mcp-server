package guestobservability

import (
	"context"
	"crypto/x509"
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

const ToolsetName = "guest-observability"

type Config struct {
	Loki LokiConfig `toml:"loki"`
}

type LokiConfig struct {
	URL                  string `toml:"url"`
	Tenant               string `toml:"tenant,omitempty"`
	Insecure             bool   `toml:"insecure,omitempty"`
	CertificateAuthority string `toml:"certificate_authority,omitempty"`
}

var _ api.ExtendedConfig = (*Config)(nil)

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("guest-observability config is nil")
	}

	return c.Loki.Validate()
}

func (c *LokiConfig) Validate() error {
	if c == nil {
		return errors.New("loki config is nil")
	}

	if strings.TrimSpace(c.URL) == "" {
		return errors.New("loki url is required")
	}

	u, err := url.Parse(c.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("loki url must be a valid URL")
	}

	if caValue := strings.TrimSpace(c.CertificateAuthority); caValue != "" {
		caPEM, err := os.ReadFile(caValue)
		if err != nil {
			return fmt.Errorf(
				"failed to read loki certificate_authority %q: %w",
				caValue,
				err,
			)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return fmt.Errorf(
				"loki certificate_authority %q contains no valid PEM certificates",
				caValue,
			)
		}
	}

	return nil
}

func toolsetConfigParser(
	ctx context.Context,
	primitive toml.Primitive,
	md toml.MetaData,
) (api.ExtendedConfig, error) {
	var cfg Config

	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}

	if cfg.Loki.CertificateAuthority != "" {
		configDir := config.ConfigDirPathFromContext(ctx)

		if configDir != "" &&
			!filepath.IsAbs(cfg.Loki.CertificateAuthority) {
			cfg.Loki.CertificateAuthority = filepath.Join(
				configDir,
				cfg.Loki.CertificateAuthority,
			)
		}
	}

	if config.RequireTLSFromContext(ctx) {
		if err := config.ValidateURLRequiresTLS(
			cfg.Loki.URL,
			"Guest observability Loki URL",
		); err != nil {
			return nil, err
		}

		if cfg.Loki.Insecure {
			return nil, errors.New(
				"require_tls is enabled but guest-observability Loki insecure=true disables certificate verification",
			)
		}
	}

	return &cfg, nil
}

func init() {
	config.RegisterToolsetConfig(
		ToolsetName,
		toolsetConfigParser,
	)
}
