package prometheus

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	model "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

// Guardrail name constants for use with ParseGuardrails
const (
	GuardrailDisallowExplicitNameLabel = "disallow-explicit-name-label"
	GuardrailRequireLabelMatcher       = "require-label-matcher"
	GuardrailDisallowBlanketRegex      = "disallow-blanket-regex"
	GuardrailMaxMetricCardinality      = "max-metric-cardinality"

	// GuardrailShortcutTSDB is a shortcut that refers to both TSDB-dependent
	// guardrails (max-metric-cardinality and disallow-blanket-regex). Use
	// "!tsdb" to disable both when the backend does not expose
	// /api/v1/status/tsdb (e.g. Thanos Querier < v0.40.0).
	GuardrailShortcutTSDB = "tsdb"
)

// Default cardinality thresholds
const (
	DefaultMaxMetricCardinality uint64 = 20000
	DefaultMaxLabelCardinality  uint64 = 500
)

// GuardrailViolation is returned when a query violates a specific guardrail rule.
// It carries the guardrail name for structured logging.
type GuardrailViolation struct {
	Guardrail string
	Message   string
}

func (e *GuardrailViolation) Error() string {
	return e.Message
}

// Guardrails provides safety checks for PromQL queries based on configurable rules.
type Guardrails struct {
	// DisallowExplicitNameLabel prevents queries using explicit {__name__="..."} syntax
	DisallowExplicitNameLabel bool
	// RequireLabelMatcher ensures all vector selectors have at least one non-name label matcher
	RequireLabelMatcher bool
	// DisallowBlanketRegex prevents expensive regex patterns like .* or .+ on any label
	DisallowBlanketRegex bool
	// ForceMaxMetricCardinality enables the maximum series count per metric guardrail
	ForceMaxMetricCardinality bool
	// MaxMetricCardinality sets the maximum allowed series count per metric
	// (only enforced when ForceMaxMetricCardinality is true)
	MaxMetricCardinality uint64
	// MaxLabelCardinality sets the maximum allowed label value count for blanket regex
	// (0 = always disallow regex matcher provided DisallowBlanketRegex is true)
	MaxLabelCardinality uint64
}

// DefaultGuardrails returns a Guardrails instance with default numeric thresholds.
// When enableAll is true, all boolean guardrails are also enabled (equivalent to "all").
func DefaultGuardrails(enableAll bool) *Guardrails {
	return &Guardrails{
		DisallowExplicitNameLabel: enableAll,
		RequireLabelMatcher:       enableAll,
		DisallowBlanketRegex:      enableAll,
		ForceMaxMetricCardinality: enableAll,
		MaxMetricCardinality:      DefaultMaxMetricCardinality,
		MaxLabelCardinality:       DefaultMaxLabelCardinality,
	}
}

func ParseGuardrails(value string) (*Guardrails, error) {
	value = strings.TrimSpace(strings.ToLower(value))

	switch value {
	case "none":
		return nil, nil
	case "all", "":
		return DefaultGuardrails(true), nil
	}

	// Determine mode from whether any token carries a "!" prefix, then verify
	// all tokens are consistent (mixing positive and negative is not allowed).
	negative := strings.Contains(value, "!")
	var names []string
	for name := range strings.SplitSeq(value, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if negative != strings.HasPrefix(name, "!") {
			return nil, fmt.Errorf("cannot mix positive and negative guardrail names: " +
				"use either explicit names to enable, or !name to disable from the full set")
		}
		name = strings.TrimPrefix(name, "!")
		names = append(names, name)
	}

	// In negative mode all guardrails start enabled; in positive mode all start disabled.
	defaultValue := negative
	g := DefaultGuardrails(defaultValue)
	for _, name := range names {
		switch name {
		case GuardrailDisallowExplicitNameLabel:
			g.DisallowExplicitNameLabel = !defaultValue
		case GuardrailRequireLabelMatcher:
			g.RequireLabelMatcher = !defaultValue
		case GuardrailDisallowBlanketRegex:
			g.DisallowBlanketRegex = !defaultValue
		case GuardrailMaxMetricCardinality:
			g.ForceMaxMetricCardinality = !defaultValue
		case GuardrailShortcutTSDB:
			if !negative {
				return nil, fmt.Errorf("%q is only valid as a negative shortcut (!tsdb); use individual guardrail names in positive mode", GuardrailShortcutTSDB)
			}
			g.ForceMaxMetricCardinality = false
			g.DisallowBlanketRegex = false
		default:
			return nil, fmt.Errorf("unknown guardrail: %q (valid options: %s, %s, %s, %s)",
				name, GuardrailDisallowExplicitNameLabel, GuardrailRequireLabelMatcher,
				GuardrailDisallowBlanketRegex, GuardrailMaxMetricCardinality)
		}
	}
	return g, nil
}

// IsSafeQuery analyzes a PromQL query string and returns false if it's
// deemed unsafe or too expensive based on the configured rules.
// If client is provided and MaxMetricCardinality is set, it checks TSDB metric cardinality.
// If client is provided and MaxLabelCardinality is set, it checks TSDB label cardinality for blanket regex.
//
// Returns (false, error) if the query is invalid or violates a guardrail rule.
// The error message explains which rule was violated.
// Returns (true, nil) if the query is valid and passes all rules.
//
//nolint:gocyclo // complex validation logic, refactoring would reduce readability
func (g *Guardrails) IsSafeQuery(ctx context.Context, query string, client v1.API) (bool, error) {
	if ((g.DisallowBlanketRegex && g.MaxLabelCardinality > 0) || g.ForceMaxMetricCardinality) && (client == nil || ctx == nil) {
		return false, fmt.Errorf("cannot verify cardinality without TSDB client")
	}

	expr, err := parser.NewParser(parser.Options{}).ParseExpr(query)
	if err != nil {
		return false, fmt.Errorf("failed to parse query: %w", err)
	}

	var unsafeReason error

	parser.Inspect(expr, func(node parser.Node, path []parser.Node) error {
		vs, ok := node.(*parser.VectorSelector)
		if !ok {
			return nil
		}

		// Check for explicit __name__ label query
		if g.DisallowExplicitNameLabel && vs.Name == "" {
			for _, m := range vs.LabelMatchers {
				if m.Name == model.MetricNameLabel {
					unsafeReason = &GuardrailViolation{
						Guardrail: GuardrailDisallowExplicitNameLabel,
						Message:   "query uses explicit __name__ label matcher, which is disallowed",
					}
					return unsafeReason
				}
			}
		}

		// All vector selectors must have at least one non-name label matcher
		if g.RequireLabelMatcher {
			hasNonNameMatcher := false
			for _, m := range vs.LabelMatchers {
				if m.Name != model.MetricNameLabel {
					hasNonNameMatcher = true
					break
				}
			}
			if !hasNonNameMatcher {
				unsafeReason = &GuardrailViolation{
					Guardrail: GuardrailRequireLabelMatcher,
					Message:   fmt.Sprintf("query for metric %q does not have any label matchers, which is required", vs.Name),
				}
				return unsafeReason
			}
		}

		return nil
	})

	if unsafeReason != nil {
		return false, unsafeReason
	}

	// Check metric cardinality
	if g.ForceMaxMetricCardinality {
		metricNames, err := ExtractMetricNames(query)
		if err != nil {
			return false, fmt.Errorf("failed to extract metric names: %w", err)
		}

		if len(metricNames) > 0 {
			tsdbResult, err := client.TSDB(ctx)
			if err != nil {
				return false, fmt.Errorf(
					"cannot enforce max-metric-cardinality guardrail: TSDB stats endpoint is unavailable on this backend "+
						"(Thanos Querier < v0.40.0 does not implement /api/v1/status/tsdb); "+
						"disable this guardrail with --guardrails '!tsdb': %w", err)
			}

			seriesCountByMetric := make(map[string]uint64)
			for _, stat := range tsdbResult.SeriesCountByMetricName {
				seriesCountByMetric[stat.Name] = stat.Value
			}

			for _, metricName := range metricNames {
				if count, exists := seriesCountByMetric[metricName]; exists {
					if count > g.MaxMetricCardinality {
						return false, &GuardrailViolation{
							Guardrail: GuardrailMaxMetricCardinality,
							Message:   fmt.Sprintf("metric %q has cardinality %d, which exceeds maximum allowed %d", metricName, count, g.MaxMetricCardinality),
						}
					}
				}
			}
		}
	}

	// Check blanket regex patterns
	if g.DisallowBlanketRegex {
		blanketRegexLabels, err := ExtractBlanketRegexLabels(query)
		if err != nil {
			return false, fmt.Errorf("failed to extract blanket regex labels: %w", err)
		}

		if len(blanketRegexLabels) > 0 {
			// If MaxLabelCardinality is 0, always disallow blanket regex
			if g.MaxLabelCardinality == 0 {
				return false, &GuardrailViolation{
					Guardrail: GuardrailDisallowBlanketRegex,
					Message:   fmt.Sprintf("query uses blanket regex on label %q, which is disallowed", blanketRegexLabels[0]),
				}
			}

			// Check TSDB label cardinality for blanket regex
			tsdbResult, err := client.TSDB(ctx)
			if err != nil {
				return false, fmt.Errorf(
					"cannot enforce max-label-cardinality guardrail: TSDB stats endpoint is unavailable on this backend "+
						"(Thanos Querier < v0.40.0 does not implement /api/v1/status/tsdb); "+
						"disable this guardrail with --guardrails '!tsdb': %w", err)
			}

			labelValueCountByLabel := make(map[string]uint64)
			for _, stat := range tsdbResult.LabelValueCountByLabelName {
				labelValueCountByLabel[stat.Name] = stat.Value
			}

			for _, labelName := range blanketRegexLabels {
				if count, exists := labelValueCountByLabel[labelName]; exists {
					if count > g.MaxLabelCardinality {
						return false, &GuardrailViolation{
							Guardrail: GuardrailDisallowBlanketRegex,
							Message:   fmt.Sprintf("label %q has cardinality %d, which exceeds maximum allowed %d for blanket regex", labelName, count, g.MaxLabelCardinality),
						}
					}
				}
			}
		}
	}

	return true, nil
}

func ExtractMetricNames(query string) ([]string, error) {
	expr, err := parser.NewParser(parser.Options{}).ParseExpr(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	metricNames := make(map[string]bool)
	parser.Inspect(expr, func(node parser.Node, path []parser.Node) error {
		if vs, ok := node.(*parser.VectorSelector); ok {
			if vs.Name != "" {
				metricNames[vs.Name] = true
			}
			// Also check for __name__ label matchers
			for _, m := range vs.LabelMatchers {
				if m.Name == model.MetricNameLabel && m.Type == labels.MatchEqual {
					metricNames[m.Value] = true
				}
			}
		}
		return nil
	})

	result := make([]string, 0, len(metricNames))
	for name := range metricNames {
		result = append(result, name)
	}
	return result, nil
}

// ExtractBlanketRegexLabels extracts label names that use blanket regex patterns (.* or .+).
func ExtractBlanketRegexLabels(query string) ([]string, error) {
	expr, err := parser.NewParser(parser.Options{}).ParseExpr(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	labelNames := make(map[string]bool)
	parser.Inspect(expr, func(node parser.Node, path []parser.Node) error {
		if vs, ok := node.(*parser.VectorSelector); ok {
			for _, m := range vs.LabelMatchers {
				isRegex := m.Type == labels.MatchRegexp || m.Type == labels.MatchNotRegexp
				if isRegex && (m.Value == ".*" || m.Value == ".+") {
					labelNames[m.Name] = true
				}
			}
		}
		return nil
	})

	result := make([]string, 0, len(labelNames))
	for name := range labelNames {
		result = append(result, name)
	}
	return result, nil
}
