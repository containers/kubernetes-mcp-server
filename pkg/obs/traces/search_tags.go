package traces

import (
	"encoding/json"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"

	tempoclient "github.com/rhobs/obs-mcp/pkg/traces/tempo"
)

// searchTagsOutput defines the output schema for the tempo_search_tags tool.
type searchTagsOutput struct {
	Scopes []any `json:"scopes" jsonschema:"List of tag scopes with their tag names"`
}

var searchTagsOutputSchema = mustSchema[searchTagsOutput]()

func initSearchTags() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name: "tempo_search_tags",
			Description: `List available tag names (attribute keys) in Tempo, grouped by scope.
Use this tool to discover which attributes are available for building TraceQL queries with tempo_search_traces.
For example, this tool may reveal tag names like "service.name" (in the "resource" scope) or "http.response.status_code" (in the "span" scope).
To use these in TraceQL queries, prefix them with their scope, e.g. "resource.service.name" or "span.http.response.status_code".`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"tempoNamespace": tempoNamespaceSchema,
					"tempoName":      tempoNameSchema,
					"tenant":         tempoTenantSchema,
					"scope": {
						Type: "string",
						Description: `Filter tags to a specific scope. One of:
"resource" (service-level attributes like service.name),
"span" (individual span attributes like http.response.status_code),
"intrinsic" (built-in fields like duration, status, name).
If omitted, tags from all scopes are returned.`,
					},
					"query": {
						Type: "string",
						Description: `Optional TraceQL query to filter which traces are considered when listing tags,
e.g. '{ resource.service.name="payment-service" }' to only show tags present in traces from the 'payment-service' service.`,
					},
					"start": {
						Type:        "string",
						Description: `Optional start of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing tags.`,
					},
					"end": {
						Type:        "string",
						Description: `Optional end of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing tags.`,
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of tag names to return per scope.",
					},
					"maxStaleValues": {
						Type:        "integer",
						Description: "Maximum number of consecutive blocks without new tag names before the search stops early. Higher values are more thorough but slower.",
					},
				},
				Required: []string{"tempoNamespace", "tempoName"},
			},
			OutputSchema: searchTagsOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Search tags",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: searchTagsHandler,
	}
}

func searchTagsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	scope := p.OptionalString("scope", "")
	query := p.OptionalString("query", "")
	startStr := p.OptionalString("start", "")
	endStr := p.OptionalString("end", "")
	limit := int(p.OptionalInt64("limit", 0))
	maxStaleValues := int(p.OptionalInt64("maxStaleValues", 0))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to search tags: %w", err)), nil
	}

	start, err := parseTime(startStr)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("invalid start time: %v", err)), nil
	}

	end, err := parseTime(endStr)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("invalid end time: %v", err)), nil
	}

	client, err := getTempoClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	result, err := client.SearchTagsV2(params.Context, tempoclient.SearchTagsV2Options{
		Scope:          scope,
		Query:          query,
		Start:          start,
		End:            end,
		Limit:          limit,
		MaxStaleValues: maxStaleValues,
	})
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	var output searchTagsOutput
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to unmarshal search tags: %w", err)), nil
	}
	return api.NewToolCallResultFull(result, output, nil), nil
}
