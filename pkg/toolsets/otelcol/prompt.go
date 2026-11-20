package otelcol

// ServerPrompt provides instructions for LLMs using the OpenTelemetry Collector toolset.
const ServerPrompt = `
## OpenTelemetry Collector Configuration Workflow

When generating or modifying collector configurations:
1. List available components first - never guess component names
2. Check the component schema to understand configuration options
3. Validate configurations before presenting to the user

If the user specifies a version, use it consistently across all tool calls.

## OTTL in transform and filter processors

Schema validation does not execute OTTL. When a configuration contains OTTL statements or conditions, check these points before presenting it:
- error_mode: the default is "ignore" from collector 0.153 and "propagate" before that. With "ignore", a statement that fails for a record is skipped for that record and logged, and the record still passes through; in the filter processor a failing condition counts as not matching, so the record is kept. With "propagate", a failure fails the whole batch and nothing from that batch is exported. With "silent", the failure is not logged. A statement or condition group can set its own error_mode, which overrides the top-level one.
- Converters can fail at run time: Double returns an error for a string that cannot be parsed. Int does too from collector 0.160; before that it returns nil without an error, so error_mode never sees the failure. Len returns an error for a nil or unsupported value, and ParseJSON returns an error for invalid JSON. Guard them with a where clause.
- A string is never equal to a number: the string "200" is not equal to the integer 200. Integers and doubles compare numerically.
- The context of a statement group decides which paths are valid. A path that is not available in that context, or an unknown function name, is rejected when the collector starts, even though the configuration passes schema validation. Paths of parent contexts, such as resource.attributes in a log or span statement, are valid.
- Regular expressions use Go RE2 syntax: matching is case-sensitive and unanchored unless written otherwise, and quantifiers are greedy.
- The filter processor drops the records that match any of its conditions. A metric condition drops the whole metric with all its data points; a datapoint condition drops single data points.
- ParseJSON keeps JSON types: numbers become doubles, strings stay strings and objects become maps. Merging a parsed map into attributes does not flatten nested objects.`
