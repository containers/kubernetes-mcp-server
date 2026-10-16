package api

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// Example is one example call of a tool: rendered into its description in
// one fixed form, so every tool on the server shows its examples the same
// way (docs/specs/tools-for-models.md).
type Example struct {
	// Summary says what the call does, in one line ("last hour of CPU for a deployment").
	Summary string `json:"summary"`
	// Arguments are the arguments of the call, as the model would send them.
	Arguments map[string]any `json:"arguments,omitempty"`
}

// Describe is the description the model sees: the author's, followed by the
// examples as `Example: <summary>: <arguments as JSON>`.
func (t Tool) Describe() string {
	description := strings.TrimSpace(t.Description)
	for _, example := range t.Examples {
		arguments, err := json.Marshal(example.Arguments)
		if err != nil || example.Arguments == nil {
			arguments = []byte("{}")
		}
		description += "\n\nExample: " + strings.TrimSpace(example.Summary) + ": " + string(arguments)
	}
	return description
}

// StandardArguments are the argument names with a server-wide meaning and a
// shared description, so a tool that declares one without a description is
// not told to write one (the dynamic engine fills the description in).
var StandardArguments = map[string]string{
	"namespace":     "Kubernetes namespace. Omit it for the caller's default namespace.",
	"name":          "Name of the resource.",
	"labelSelector": "Kubernetes label selector that narrows the result, e.g. 'app=web' or 'tier in (frontend,backend)'.",
	"fieldSelector": "Kubernetes field selector that narrows the result, e.g. 'status.phase=Running'.",
	"start":         "Start of the time range: RFC 3339 ('2025-01-01T00:00:00Z'), Unix seconds, NOW, or NOW minus a duration ('NOW-1h').",
	"end":           "End of the time range, in the same forms as start. Defaults to now.",
	"duration":      "Lookback from now when start and end are omitted: a number with a unit, e.g. '30s', '15m', '2h', '1d', '1w'.",
	"time":          "A point in time, in the same forms as start. Defaults to now.",
}

// enumeratesInProse matches a description that spells out a fixed set of
// quoted values ("one of 'a', 'b'", "either 'x' or 'y'") instead of
// declaring an enum.
var enumeratesInProse = regexp.MustCompile(`(?i)\b(one of|either)\b[^.]*['"][^'"]+['"]`)

// camelCase is the spelling of every argument name: a lower-case word, then
// capitalized words, digits allowed, no separators.
var camelCase = regexp.MustCompile(`^[a-z][a-z0-9]*([A-Z][a-z0-9]*)*$`)

// LintTool returns the advice of docs/specs/tools-for-models.md a tool does
// not follow. None of it makes the tool invalid; the built-in tools are
// tested to carry none, and the dynamic engine reports it in the resource's
// status.
func LintTool(tool Tool) []string {
	var lints []string
	if len(strings.Fields(tool.Description)) < 4 {
		lints = append(lints, "description is shorter than a sentence: say what question the tool answers and when to prefer it")
	}
	var names []string
	if tool.InputSchema != nil {
		for name := range tool.InputSchema.Properties {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		property := tool.InputSchema.Properties[name]
		if property == nil {
			continue
		}
		if !camelCase.MatchString(name) {
			lints = append(lints, "argument "+name+" is not camelCase")
		}
		if property.Description == "" {
			if _, standard := StandardArguments[name]; !standard {
				lints = append(lints, "argument "+name+" has no description")
			}
		}
		if property.Type == "string" && len(property.Enum) == 0 && enumeratesInProse.MatchString(property.Description) {
			lints = append(lints, "argument "+name+" spells out its values in prose; declare them as enum")
		}
	}
	if len(tool.Examples) == 0 {
		lints = append(lints, "no examples: one example call shows the model how to fill the arguments")
	}
	return lints
}
