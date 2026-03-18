package api

import "fmt"

// ConfirmationRule defines a rule for prompting the user before an action.
// Rules are classified as tool-level or kube-level based on which fields are set.
// A rule must not have both tool-level and kube-level fields set.
type ConfirmationRule struct {
	// Tool-level fields
	Tool        string         `toml:"tool,omitempty"`
	Destructive *bool          `toml:"destructive,omitempty"`
	Input       map[string]any `toml:"input,omitempty"`
	// Kube-level fields
	Verb      string `toml:"verb,omitempty"`
	Kind      string `toml:"kind,omitempty"`
	Group     string `toml:"group,omitempty"`
	Version   string `toml:"version,omitempty"`
	Namespace string `toml:"namespace,omitempty"`
	// Common fields
	Message  string `toml:"message"`
	Fallback string `toml:"fallback,omitempty"`
}

// IsToolLevel returns true if the rule targets MCP tool invocations.
func (r *ConfirmationRule) IsToolLevel() bool {
	return r.Tool != "" || r.Destructive != nil
}

// IsKubeLevel returns true if the rule targets Kubernetes API requests.
func (r *ConfirmationRule) IsKubeLevel() bool {
	return r.Verb != "" || r.Kind != "" || r.Group != "" || r.Version != ""
}

// Validate checks that the rule is well-formed.
// A rule must not mix tool-level fields (tool, destructive) with kube-level fields (verb, kind, group, version).
// The namespace field is allowed alongside tool-level fields because it also appears in tool input matching.
func (r *ConfirmationRule) Validate() error {
	if r.IsToolLevel() && r.IsKubeLevel() {
		return fmt.Errorf("confirmation rule mixes tool-level fields (tool, destructive) with kube-level fields (verb, kind, group, version): %q", r.Message)
	}
	return nil
}

// EffectiveFallback returns the rule's fallback if set, otherwise the global default.
func (r *ConfirmationRule) EffectiveFallback(globalDefault string) string {
	if r.Fallback != "" {
		return r.Fallback
	}
	return globalDefault
}

// ConfirmationRulesProvider provides access to confirmation rules and the global fallback.
type ConfirmationRulesProvider interface {
	GetConfirmationRules() []ConfirmationRule
	GetConfirmationFallback() string
}
