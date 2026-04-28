package traces

import (
	"encoding/json"
	"fmt"

	"github.com/rhobs/obs-mcp/pkg/tools"
	tempoclient "github.com/rhobs/obs-mcp/pkg/traces/tempo"
)

// SearchTagValuesOutput defines the output schema for the tempo_search_tag_values tool.
type SearchTagValuesOutput struct {
	TagValues any `json:"tagValues" jsonschema:"Known values for the specified tag, keyed by type"`
}

var SearchTagValuesTool = tools.ToolDef[SearchTagValuesOutput]{
	Name: "tempo_search_tag_values",
	Description: `List the known values for a specific tag (attribute key) in Tempo.
Use this tool to discover what values exist for a given tag, e.g. to find all service names (values of "resource.service.name") or all HTTP methods (values of "span.http.request.method").
This is useful for building accurate TraceQL queries with tempo_search_traces.`,
	Title: "Search tag values",
	Params: []tools.ParamDef{
		tempoNamespaceParameter,
		tempoNameParameter,
		tempoTenantParameter,
		{
			Name: "tag",
			Type: tools.ParamTypeString,
			Description: `The fully qualified tag name to get values for, including its scope prefix, e.g. "resource.service.name" or "span.http.response.status_code".
Use tempo_search_tags to discover available tag names.`,
			Required: true,
		},
		{
			Name: "query",
			Type: tools.ParamTypeString,
			Description: `Optional TraceQL query to filter which traces are considered when listing values,
e.g. '{ resource.service.name="payment-service" }' to only show tag values from the 'payment-service' service.`,
		},
		{
			Name:        "start",
			Type:        tools.ParamTypeString,
			Description: `Optional start of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing values.`,
		},
		{
			Name:        "end",
			Type:        tools.ParamTypeString,
			Description: `Optional end of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing values.`,
		},
		{
			Name:        "limit",
			Type:        tools.ParamTypeNumber,
			Description: "Maximum number of tag values to return.",
		},
		{
			Name:        "maxStaleValues",
			Type:        tools.ParamTypeNumber,
			Description: "Maximum number of consecutive blocks without new values before the search stops early. Higher values are more thorough but slower.",
		},
	},
	ReadOnly:    true,
	Destructive: false,
	Idempotent:  true,
	OpenWorld:   true,
}

func (t *Toolset) SearchTagValuesHandler(params ToolParams) (SearchTagValuesOutput, error) {
	client, err := t.getTempoClient(params)
	if err != nil {
		return SearchTagValuesOutput{}, err
	}

	args := params.arguments

	tag := tools.GetString(args, "tag", "")
	if tag == "" {
		return SearchTagValuesOutput{}, fmt.Errorf("tag parameter must not be empty")
	}

	start, err := parseTime(tools.GetString(args, "start", ""))
	if err != nil {
		return SearchTagValuesOutput{}, fmt.Errorf("invalid start time: %v", err)
	}

	end, err := parseTime(tools.GetString(args, "end", ""))
	if err != nil {
		return SearchTagValuesOutput{}, fmt.Errorf("invalid end time: %v", err)
	}

	query := tools.GetString(args, "query", "")
	limit := tools.GetInt(args, "limit", 0)
	maxStaleValues := tools.GetInt(args, "maxStaleValues", 0)

	opts := tempoclient.SearchTagValuesV2Options{
		Query:          query,
		Start:          start,
		End:            end,
		Limit:          limit,
		MaxStaleValues: maxStaleValues,
	}

	result, err := client.SearchTagValuesV2(params.context, tag, opts)
	if err != nil {
		return SearchTagValuesOutput{}, err
	}

	var output SearchTagValuesOutput
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		return SearchTagValuesOutput{}, fmt.Errorf("failed to unmarshal tag values: %w", err)
	}
	return output, nil
}
