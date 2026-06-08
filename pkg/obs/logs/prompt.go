package logs

const ServerPrompt = `
You have access to Loki log tools.

Recommended workflow:
1. Discover LokiStack instances first with loki_list_instances when Loki Operator is installed.
2. Discover labels with loki_label_names and loki_label_values.
3. Build narrow LogQL queries with explicit label matchers.
4. Use short time windows and small limits first, then expand only if needed.

Avoid broad queries without label matchers because they are expensive and noisy.
`

const (
	lokiLabelNamesPrompt  = `List available Loki label names for a time range. Use this before writing LogQL queries.`
	lokiLabelValuesPrompt = `List possible values for a Loki label key. Use this to build precise label matchers in LogQL.`
	lokiQueryRangePrompt  = `Execute a Loki LogQL range query and return matching log streams and lines.

Use precise label matchers and a short time window first.`
)
