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
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"k8s.io/apimachinery/pkg/util/validation"
)

const defaultURLDiscoveryNamespace = "istio-system"

// Config holds Kiali toolset configuration
type Config struct {
	Url                  string `toml:"url"`
	Insecure             bool   `toml:"insecure,omitempty"`
	CertificateAuthority string `toml:"certificate_authority,omitempty"`
	// AllowedNamespaces limits in-cluster URL autodiscovery (Kiali CR list and
	// well-known Service DNS fallbacks) to these namespaces. Defaults to
	// istio-system when unset.
	AllowedNamespaces []string `toml:"allowed_namespaces,omitempty"`
}

var (
	_ config.ExtendedConfig      = (*Config)(nil)
	_ config.RequireTLSValidator = (*Config)(nil)
)

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("kiali config is nil")
	}
	// URL is optional: when empty, HasKiali may discover an in-cluster Service URL
	// from a Kiali CR and inject it into this config at runtime.
	if strings.TrimSpace(c.Url) == "" {
		if caValue := strings.TrimSpace(c.CertificateAuthority); caValue != "" {
			if _, err := os.Stat(caValue); err != nil {
				return fmt.Errorf("certificate_authority must be a valid file path: %w", err)
			}
		}
		return c.validateAllowedNamespaces()
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
	return c.validateAllowedNamespaces()
}

func (c *Config) validateAllowedNamespaces() error {
	for _, ns := range c.AllowedNamespaces {
		ns = strings.TrimSpace(ns)
		if ns == "" {
			return errors.New("allowed_namespaces entries must not be empty")
		}
		if errs := validation.IsDNS1123Label(ns); len(errs) > 0 {
			return fmt.Errorf("allowed_namespaces entry %q is not a valid namespace name: %s", ns, strings.Join(errs, ", "))
		}
	}
	return nil
}

// URLDiscoveryNamespaces returns namespaces searched for Kiali CRs and
// well-known in-cluster Service URLs when url is empty.
func (c *Config) URLDiscoveryNamespaces() []string {
	if c == nil || len(c.AllowedNamespaces) == 0 {
		return []string{defaultURLDiscoveryNamespace}
	}
	out := make([]string, 0, len(c.AllowedNamespaces))
	seen := make(map[string]struct{}, len(c.AllowedNamespaces))
	for _, ns := range c.AllowedNamespaces {
		if trimmed := strings.TrimSpace(ns); trimmed != "" {
			if _, dup := seen[trimmed]; dup {
				continue
			}
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{defaultURLDiscoveryNamespace}
	}
	return out
}

func (c *Config) ValidateRequireTLS(requireTLS bool) error {
	if !requireTLS {
		return nil
	}
	if c == nil {
		return errors.New("kiali config is nil")
	}
	if err := config.ValidateURLRequiresTLS(c.Url, "Kiali URL"); err != nil {
		return err
	}
	if c.Insecure {
		return errors.New("require_tls is enabled but Kiali insecure=true disables certificate verification")
	}
	return nil
}

func kialiToolsetParser(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (config.ExtendedConfig, error) {
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

	// Validate TLS when require_tls is enabled. Empty URL is allowed here so
	// HasKiali can discover an in-cluster Service URL at filter time.
	if err := cfg.ValidateRequireTLS(config.RequireTLSFromContext(ctx)); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func init() {
	config.RegisterToolsetConfig("kiali", kialiToolsetParser)
}
