package api

import "fmt"

// ConfirmationRule defines a rule for prompting the user before an action.
// Rules are classified as tool-level or kube-level based on which fields are set.
// A rule must not have both tool-level and kube-level fields set.
type ConfirmationRule struct {
	// Tool-level fields
	Tool        string `toml:"tool,omitempty"`
	Destructive *bool  `toml:"destructive,omitempty"`
	// Kube-level fields
	Verb      string `toml:"verb,omitempty"`
	Kind      string `toml:"kind,omitempty"`
	Group     string `toml:"group,omitempty"`
	Version   string `toml:"version,omitempty"`
	Name      string `toml:"name,omitempty"`
	Namespace string `toml:"namespace,omitempty"`
	// Common fields
	Message string `toml:"message"`
}

// IsToolLevel returns true if the rule targets MCP tool invocations.
func (r *ConfirmationRule) IsToolLevel() bool {
	return r.Tool != "" || r.Destructive != nil
}

// IsKubeLevel returns true if the rule targets Kubernetes API requests.
func (r *ConfirmationRule) IsKubeLevel() bool {
	return r.Verb != "" || r.Kind != "" || r.Group != "" || r.Version != "" || r.Name != "" || r.Namespace != ""
}

// Validate checks that the rule is well-formed.
// A rule must be either tool-level or kube-level (not both, and not neither).
// Tool-level rules must not contain kube-level-only fields and vice versa.
func (r *ConfirmationRule) Validate() error {
	if r.IsToolLevel() && r.IsKubeLevel() {
		return fmt.Errorf("confirmation rule mixes tool-level fields (tool, destructive) with kube-level fields (verb, kind, group, version, name, namespace): %q", r.Message)
	}
	if !r.IsToolLevel() && !r.IsKubeLevel() {
		return fmt.Errorf("confirmation rule must set at least one tool-level field (tool, destructive) or kube-level field (verb, kind, group, version, name, namespace): %q", r.Message)
	}
	return nil
}

// ConfirmationRulesProvider provides access to confirmation rules and the global fallback.
type ConfirmationRulesProvider interface {
	GetConfirmationRules() []ConfirmationRule
	GetConfirmationFallback() string
}
