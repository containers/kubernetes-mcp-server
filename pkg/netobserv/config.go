package netobserv

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
	"k8s.io/klog/v2"
)

// Config holds NetObserv console plugin backend configuration.
type Config struct {
	// Url overrides the plugin base URL. When empty, built from namespace, service, and port.
	Url                  string `toml:"url,omitempty"`
	Namespace            string `toml:"namespace,omitempty"`
	Service              string `toml:"service,omitempty"`
	Port                 int    `toml:"port,omitempty"`
	Insecure             bool   `toml:"insecure,omitempty"`
	CertificateAuthority string `toml:"certificate_authority,omitempty"`
}

var _ api.ExtendedConfig = (*Config)(nil)

// ResolvedURL returns the plugin base URL, applying operator-aligned defaults when url is unset.
func (c *Config) ResolvedURL(isOpenShift bool) string {
	if c == nil {
		return DefaultPluginURL(isOpenShift)
	}
	if u := strings.TrimSpace(c.Url); u != "" {
		return u
	}
	ns := c.Namespace
	if ns == "" {
		ns = DefaultPluginNamespace
	}
	svc := c.Service
	if svc == "" {
		svc = DefaultPluginService
	}
	port := c.Port
	if port == 0 {
		port = DefaultPluginPort
	}
	return BuildPluginURL(ns, svc, port, isOpenShift)
}

func (c *Config) usesSynthesizedURL() bool {
	return c == nil || strings.TrimSpace(c.Url) == ""
}

// applyDefaults sets certificate_authority from the projected service CA on OpenShift when url is synthesized.
func (c *Config) applyDefaults(isOpenShift bool) {
	c.applyDefaultsWithStat(isOpenShift, os.Stat)
}

func (c *Config) applyDefaultsWithStat(isOpenShift bool, stat func(string) (os.FileInfo, error)) {
	if c == nil || !c.usesSynthesizedURL() || !isOpenShift {
		return
	}
	if c.Insecure || strings.TrimSpace(c.CertificateAuthority) != "" {
		return
	}
	if _, err := stat(DefaultPluginServiceCAPath); err == nil {
		c.CertificateAuthority = DefaultPluginServiceCAPath
		return
	}
	klog.Background().Info(
		"NetObserv plugin TLS: service CA not found; set certificate_authority or insecure=true",
		"path", DefaultPluginServiceCAPath,
	)
}

func (c *Config) Validate() error {
	return c.validate(false)
}

func (c *Config) validate(isOpenShift bool) error {
	if c == nil {
		return errors.New("netobserv config is nil")
	}
	resolved := c.ResolvedURL(isOpenShift)
	u, err := url.Parse(resolved)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("url must be a valid URL")
	}
	if strings.EqualFold(u.Scheme, "https") && !c.Insecure && strings.TrimSpace(c.CertificateAuthority) == "" {
		return errors.New("certificate_authority is required for https when insecure is false")
	}
	if caValue := strings.TrimSpace(c.CertificateAuthority); caValue != "" {
		if _, err := os.Stat(caValue); err != nil {
			return fmt.Errorf("certificate_authority must be a valid file path: %w", err)
		}
	}
	return nil
}

func netobservToolsetParser(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
	var cfg Config
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}

	if cfg.CertificateAuthority != "" {
		configDir := config.ConfigDirPathFromContext(ctx)
		if configDir != "" && !filepath.IsAbs(cfg.CertificateAuthority) {
			cfg.CertificateAuthority = filepath.Join(configDir, cfg.CertificateAuthority)
		}
	}

	requireTLS := config.RequireTLSFromContext(ctx)
	// Config is validated without a live cluster; assume non-OpenShift (HTTP synthesized URL).
	const configLoadOpenShift = false
	if requireTLS {
		if err := config.ValidateURLRequiresTLS(cfg.ResolvedURL(configLoadOpenShift), "NetObserv URL"); err != nil {
			return nil, err
		}
		if cfg.Insecure {
			return nil, errors.New("require_tls is enabled but NetObserv insecure=true disables certificate verification")
		}
	}

	cfg.applyDefaults(configLoadOpenShift)

	if err := cfg.validate(configLoadOpenShift); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func init() {
	config.RegisterToolsetConfig("netobserv", netobservToolsetParser)
}
