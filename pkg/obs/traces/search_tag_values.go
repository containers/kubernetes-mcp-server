package traces

import (
	"encoding/json"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"

	tempoclient "github.com/rhobs/obs-mcp/pkg/traces/tempo"
)

// searchTagValuesOutput defines the output schema for the tempo_search_tag_values tool.
type searchTagValuesOutput struct {
	TagValues any `json:"tagValues" jsonschema:"Known values for the specified tag, keyed by type"`
}

var searchTagValuesOutputSchema = mustSchema[searchTagValuesOutput]()

func initSearchTagValues() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name: "tempo_search_tag_values",
			Description: `List the known values for a specific tag (attribute key) in Tempo.
Use this tool to discover what values exist for a given tag, e.g. to find all service names (values of "resource.service.name") or all HTTP methods (values of "span.http.request.method").
This is useful for building accurate TraceQL queries with tempo_search_traces.`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"tempoNamespace": tempoNamespaceSchema,
					"tempoName":      tempoNameSchema,
					"tenant":         tempoTenantSchema,
					"tag": {
						Type: "string",
						Description: `The fully qualified tag name to get values for, including its scope prefix, e.g. "resource.service.name" or "span.http.response.status_code".
Use tempo_search_tags to discover available tag names.`,
					},
					"query": {
						Type: "string",
						Description: `Optional TraceQL query to filter which traces are considered when listing values,
e.g. '{ resource.service.name="payment-service" }' to only show tag values from the 'payment-service' service.`,
					},
					"start": {
						Type:        "string",
						Description: `Optional start of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing values.`,
					},
					"end": {
						Type:        "string",
						Description: `Optional end of the time range (in RFC 3339 format, e.g. "2025-01-01T00:00:00Z") to filter which traces are considered when listing values.`,
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of tag values to return.",
					},
					"maxStaleValues": {
						Type:        "integer",
						Description: "Maximum number of consecutive blocks without new values before the search stops early. Higher values are more thorough but slower.",
					},
				},
				Required: []string{"tempoNamespace", "tempoName", "tag"},
			},
			OutputSchema: searchTagValuesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Search tag values",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: searchTagValuesHandler,
	}
}

func searchTagValuesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	tag := p.RequiredString("tag")
	query := p.OptionalString("query", "")
	startStr := p.OptionalString("start", "")
	endStr := p.OptionalString("end", "")
	limit := int(p.OptionalInt64("limit", 0))
	maxStaleValues := int(p.OptionalInt64("maxStaleValues", 0))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to search tag values: %w", err)), nil
	}

	if tag == "" {
		return api.NewToolCallResult("", fmt.Errorf("tag parameter must not be empty")), nil
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

	result, err := client.SearchTagValuesV2(params.Context, tag, tempoclient.SearchTagValuesV2Options{
		Query:          query,
		Start:          start,
		End:            end,
		Limit:          limit,
		MaxStaleValues: maxStaleValues,
	})
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	var output searchTagValuesOutput
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to unmarshal tag values: %w", err)), nil
	}
	return api.NewToolCallResultFull(result, output, nil), nil
}
