package traces

import (
	"encoding/json"
	"fmt"

	"github.com/rhobs/obs-mcp/pkg/tools"
	tempoclient "github.com/rhobs/obs-mcp/pkg/traces/tempo"
)

// SearchTagsOutput defines the output schema for the tempo_search_tags tool.
type SearchTagsOutput struct {
	Scopes []any `json:"scopes" jsonschema:"List of tag scopes with their tag names"`
}

var SearchTagsTool = tools.ToolDef[SearchTagsOutput]{
	Name: "tempo_search_tags",
	Description: `List available tag names (attribute keys) in Tempo, grouped by scope.
Use this tool to discover which attributes are available for building TraceQL queries with tempo_search_traces.
For example, this tool may reveal tag names like "service.name" (in the "resource" scope) or "http.response.status_code" (in the "span" scope).
To use these in TraceQL queries, prefix them with their scope, e.g. "resource.service.name" or "span.http.response.status_code".`,
	Title: "Search tags",
	Params: []tools.ParamDef{
		tempoNamespaceParameter,
		tempoNameParameter,
		tempoTenantParameter,
		{
			Name: "scope",
			Type: tools.ParamTypeString,
			Description: `Filter tags to a specific scope. One of:
"resource" (service-level attributes like service.name),
"span" (individual span attributes like http.response.status_code),
"intrinsic" (built-in fields like duration, status, name).
If omitted, tags from all scopes are returned.`,
		},
		{
			Name: "query",
			Type: tools.ParamTypeString,
			Description: `Optional TraceQL query to filter which traces are considered when listing tags,
e.g. '{ resource.service.name="payment-service" }' to only show tags present in traces from the 'payment-service' service.`,
		},
		{
			Name:        "start",
			Type:        tools.ParamTypeString,
			Description: `Optional start of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing tags.`,
		},
		{
			Name:        "end",
			Type:        tools.ParamTypeString,
			Description: `Optional end of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing tags.`,
		},
		{
			Name:        "limit",
			Type:        tools.ParamTypeNumber,
			Description: "Maximum number of tag names to return per scope.",
		},
		{
			Name:        "maxStaleValues",
			Type:        tools.ParamTypeNumber,
			Description: "Maximum number of consecutive blocks without new tag names before the search stops early. Higher values are more thorough but slower.",
		},
	},
	ReadOnly:    true,
	Destructive: false,
	Idempotent:  true,
	OpenWorld:   true,
}

func (t *Toolset) SearchTagsHandler(params ToolParams) (SearchTagsOutput, error) {
	client, err := t.getTempoClient(params)
	if err != nil {
		return SearchTagsOutput{}, err
	}

	args := params.arguments

	start, err := parseTime(tools.GetString(args, "start", ""))
	if err != nil {
		return SearchTagsOutput{}, fmt.Errorf("invalid start time: %v", err)
	}

	end, err := parseTime(tools.GetString(args, "end", ""))
	if err != nil {
		return SearchTagsOutput{}, fmt.Errorf("invalid end time: %v", err)
	}

	scope := tools.GetString(args, "scope", "")
	query := tools.GetString(args, "query", "")
	limit := tools.GetInt(args, "limit", 0)
	maxStaleValues := tools.GetInt(args, "maxStaleValues", 0)

	opts := tempoclient.SearchTagsV2Options{
		Scope:          scope,
		Query:          query,
		Start:          start,
		End:            end,
		Limit:          limit,
		MaxStaleValues: maxStaleValues,
	}

	result, err := client.SearchTagsV2(params.context, opts)
	if err != nil {
		return SearchTagsOutput{}, err
	}

	var output SearchTagsOutput
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		return SearchTagsOutput{}, fmt.Errorf("failed to unmarshal search tags: %w", err)
	}
	return output, nil
}
