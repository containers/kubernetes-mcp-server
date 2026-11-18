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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	defaultURLDiscoveryNamespace = "istio-system"
	// DiscoveryAllNamespacesWildcard enables cluster-wide Kiali CR listing.
	DiscoveryAllNamespacesWildcard = "**"
)

// Config holds Kiali toolset configuration
type Config struct {
	Url                  string `toml:"url"`
	Insecure             bool   `toml:"insecure,omitempty"`
	CertificateAuthority string `toml:"certificate_authority,omitempty"`
	// AllowedNamespaces limits in-cluster URL autodiscovery (Kiali CR list and
	// well-known kiali.<namespace>.svc DNS probes) to these namespaces. Defaults
	// to istio-system when unset. Use "**" alone for cluster-wide discovery.
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

// normalizeAllowedNamespaces trims, skips empties, and deduplicates preserving
// first-occurrence order. If the only non-empty entry is "**", returns ["**"].
// Mixed "**" + names keeps the namespace names; Validate rejects that config.
func normalizeAllowedNamespaces(raw []string) []string {
	if len(raw) == 0 {
		return []string{defaultURLDiscoveryNamespace}
	}
	seen := sets.New[string]()
	out := make([]string, 0, len(raw))
	hasWildcard := false
	for _, entry := range raw {
		ns := strings.TrimSpace(entry)
		if ns == "" {
			continue
		}
		if ns == DiscoveryAllNamespacesWildcard {
			hasWildcard = true
			continue
		}
		if seen.Has(ns) {
			continue
		}
		seen.Insert(ns)
		out = append(out, ns)
	}
	if hasWildcard && len(out) == 0 {
		return []string{DiscoveryAllNamespacesWildcard}
	}
	if len(out) == 0 {
		return []string{defaultURLDiscoveryNamespace}
	}
	return out
}

func (c *Config) validateAllowedNamespaces() error {
	if len(c.AllowedNamespaces) == 0 {
		return nil
	}
	seen := sets.New[string]()
	out := make([]string, 0, len(c.AllowedNamespaces))
	for _, entry := range c.AllowedNamespaces {
		ns := strings.TrimSpace(entry)
		if ns == "" {
			return errors.New("allowed_namespaces entries must not be empty")
		}
		if ns == DiscoveryAllNamespacesWildcard {
			seen.Insert(ns)
			continue
		}
		if errs := validation.IsDNS1123Label(ns); len(errs) > 0 {
			return fmt.Errorf("allowed_namespaces entry %q is not a valid namespace name: %s", ns, strings.Join(errs, ", "))
		}
		if seen.Has(ns) {
			continue
		}
		seen.Insert(ns)
		out = append(out, ns)
	}
	if seen.Has(DiscoveryAllNamespacesWildcard) && seen.Len() != 1 {
		return fmt.Errorf("allowed_namespaces must contain only %q when enabling cluster-wide discovery", DiscoveryAllNamespacesWildcard)
	}
	if seen.Has(DiscoveryAllNamespacesWildcard) {
		c.AllowedNamespaces = []string{DiscoveryAllNamespacesWildcard}
		return nil
	}
	c.AllowedNamespaces = out
	return nil
}

// URLDiscoveryNamespaces returns namespaces searched for Kiali CRs and
// well-known in-cluster Service URLs when url is empty.
// Soft-normalizes (trim/dedupe); hard checks belong in Validate.
func (c *Config) URLDiscoveryNamespaces() []string {
	if c == nil || len(c.AllowedNamespaces) == 0 {
		return []string{defaultURLDiscoveryNamespace}
	}
	return normalizeAllowedNamespaces(c.AllowedNamespaces)
}

// DiscoverAllNamespaces reports whether discovery should list Kiali CRs cluster-wide.
func DiscoverAllNamespaces(namespaces []string) bool {
	return len(namespaces) == 1 && namespaces[0] == DiscoveryAllNamespacesWildcard
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
