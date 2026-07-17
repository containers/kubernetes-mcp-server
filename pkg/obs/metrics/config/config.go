package config

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	serverconfig "github.com/containers/kubernetes-mcp-server/pkg/config"

	"github.com/rhobs/obs-mcp/pkg/auth"
	"github.com/rhobs/obs-mcp/pkg/metrics/prometheus"
)

const MetricsToolSetName = "observability/metrics"

// Config holds obs-mcp toolset configuration
type Config struct {
	// AuthMode controls where the bearer token is obtained for authenticating
	// against Prometheus and Alertmanager endpoints.
	// Valid values: "header" (default) - read from the request context authorization header,
	//              "kubeconfig" - read from the kubeconfig/REST config.
	AuthMode auth.AuthMode `toml:"auth_mode,omitempty"`
	// PrometheusURL is the URL of the Prometheus/Thanos Querier endpoint.
	// This field is required. Example: "https://thanos-querier-openshift-monitoring.apps.example.com"
	PrometheusURL string `toml:"prometheus_url,omitempty"`

	// AlertmanagerURL is the URL of the Alertmanager endpoint.
	// This field is optional. Example: "https://alertmanager-main-openshift-monitoring.apps.example.com"
	AlertmanagerURL string `toml:"alertmanager_url,omitempty"`

	// Insecure controls whether to skip TLS certificate verification.
	// Default: false (verify certificates)
	Insecure bool `toml:"insecure,omitempty"`

	// Guardrails controls which query safety checks are enabled.
	// Valid values: "all" (default), "none", or comma-separated list of:
	//   - "disallow-explicit-name-label"
	//   - "require-label-matcher"
	//   - "disallow-blanket-regex"
	Guardrails string `toml:"guardrails,omitempty"`

	// MaxMetricCardinality is the maximum allowed series count per metric.
	// When unset, the default of 20000 is used.
	MaxMetricCardinality *uint64 `toml:"max_metric_cardinality,omitempty"`

	// MaxLabelCardinality is the maximum allowed label value count for blanket regex.
	// Only takes effect if disallow-blanket-regex is enabled.
	// When unset, the default of 500 is used.
	// Set to 0 to always disallow blanket regex regardless of cardinality.
	MaxLabelCardinality *uint64 `toml:"max_label_cardinality,omitempty"`

	// RangeQueryFullResponse controls whether range queries return full data points
	// instead of summary statistics.
	// Default: false (return summary statistics)
	RangeQueryFullResponse bool `toml:"range_query_full_response,omitempty"`
}

var _ api.ExtendedConfig = (*Config)(nil)

// Validate checks that the configuration values are valid.
func (c *Config) Validate() error {
	if c.AuthMode != "" && c.AuthMode != auth.AuthModeHeader && c.AuthMode != auth.AuthModeKubeConfig {
		return fmt.Errorf("invalid auth_mode: %q (valid options: %q, %q)", c.AuthMode, auth.AuthModeHeader, auth.AuthModeKubeConfig)
	}

	if _, err := c.GetGuardrails(); err != nil {
		return err
	}

	return nil
}

// GetAuthMode returns the configured token source, defaulting to TokenSourceHeader.
func (c *Config) GetAuthMode() auth.AuthMode {
	if c.AuthMode == "" {
		return auth.AuthModeHeader
	}
	return c.AuthMode
}

// GetGuardrails returns the parsed guardrails configuration with cardinality limits applied.
func (c *Config) GetGuardrails() (*prometheus.Guardrails, error) {
	guardrailsStr := c.Guardrails
	if guardrailsStr == "" {
		guardrailsStr = "all" // default
	}

	guardrails, err := prometheus.ParseGuardrails(guardrailsStr)
	if err != nil {
		return nil, err
	}

	if c.MaxMetricCardinality != nil {
		if guardrails == nil || !guardrails.ForceMaxMetricCardinality {
			return nil, fmt.Errorf(
				"max_metric_cardinality is set but the %q guardrail is not enabled",
				prometheus.GuardrailMaxMetricCardinality)
		}
		if *c.MaxMetricCardinality == 0 {
			return nil, fmt.Errorf(
				"max_metric_cardinality = 0 is not supported to disable the guardrail; use '!%s' in guardrails instead",
				prometheus.GuardrailMaxMetricCardinality)
		}
		guardrails.MaxMetricCardinality = *c.MaxMetricCardinality
	}
	if c.MaxLabelCardinality != nil {
		if guardrails == nil || !guardrails.DisallowBlanketRegex {
			return nil, fmt.Errorf(
				"max_label_cardinality is set but the %q guardrail is not enabled",
				prometheus.GuardrailDisallowBlanketRegex)
		}
		guardrails.MaxLabelCardinality = *c.MaxLabelCardinality
	}

	return guardrails, nil
}

func obsMCPToolsetParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
	var cfg Config
	if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func init() {
	serverconfig.RegisterToolsetConfig(MetricsToolSetName, obsMCPToolsetParser)
}
