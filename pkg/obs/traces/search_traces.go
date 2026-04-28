package traces

import (
	"encoding/json"
	"fmt"

	"github.com/rhobs/obs-mcp/pkg/tools"
	tempoclient "github.com/rhobs/obs-mcp/pkg/traces/tempo"
)

// SearchTracesOutput defines the output schema for the tempo_search_traces tool.
type SearchTracesOutput struct {
	Traces  []any `json:"traces" jsonschema:"List of matching traces with metadata"`
	Metrics any   `json:"metrics,omitempty" jsonschema:"Query performance metrics"`
}

var SearchTracesTool = tools.ToolDef[SearchTracesOutput]{
	Name: "tempo_search_traces",
	Description: `Search for distributed traces in Tempo using TraceQL.
Use this tool to find traces matching specific criteria such as service name, HTTP status code, duration, or other span or resource attributes.`,
	Title: "Search traces",
	Params: []tools.ParamDef{
		tempoNamespaceParameter,
		tempoNameParameter,
		tempoTenantParameter,
		{
			Name: "query",
			Type: tools.ParamTypeString,
			Description: `A TraceQL query expression. Format:
query: "{ <filters joined by &&> }"

Filters:
- service name:     resource.service.name="<value>" (string, use quotes)
- HTTP status code: span.http.response.status_code=<code> (number, no quotes)
- duration:         duration><value like 100ms, 2s, 5m> (no quotes)
- error status:     status=error (keyword, NO quotes — do NOT write status="error")

IMPORTANT: status values (error, ok, unset) are keywords, NOT strings. Write status=error, NEVER status="error".

Operators: =, !=, >, <, >=, <=

Common attributes:
- resource.service.name (service name)
- span.http.response.status_code (HTTP response code)
- span.http.request.method (HTTP method like GET, POST)
- span.url.full (request URL)
- duration (trace duration, e.g. 100ms, 2s)
- status (trace status: ok, error, unset)

IMPORTANT: Always wrap filters in curly braces { }.
Do NOT use SQL, PromQL, or Lucene syntax.
Do NOT omit the "resource." or "span." prefix from attribute names

If unsure which attributes to filter on, start with {} to return all traces, then use tempo_search_tags to discover available attributes.
`,
			Required: true,
		},
		{
			Name:        "limit",
			Type:        tools.ParamTypeNumber,
			Description: "Maximum number of traces to return. Defaults to the server-side limit if not specified.",
		},
		{
			Name: "start",
			Type: tools.ParamTypeString,
			Description: `Start of the time range in RFC 3339 format, e.g. "2025-01-01T00:00:00Z".
Use "NOW" for current time.
Both start and end should be provided to search the full time range; if omitted, only a small window of recent data is searched.`,
		},
		{
			Name: "end",
			Type: tools.ParamTypeString,
			Description: `End of the time range in RFC 3339 format, e.g. "2025-01-01T00:00:00Z".
Use "NOW" for current time.
Both start and end should be provided to search the full time range; if omitted, only a small window of recent data is searched.`,
		},
		{
			Name:        "spss",
			Type:        tools.ParamTypeNumber,
			Description: "Maximum number of matching spans to return per trace.",
		},
	},
	ReadOnly:    true,
	Destructive: false,
	Idempotent:  true,
	OpenWorld:   true,
}

func (t *Toolset) SearchTracesHandler(params ToolParams) (SearchTracesOutput, error) {
	client, err := t.getTempoClient(params)
	if err != nil {
		return SearchTracesOutput{}, err
	}

	args := params.arguments

	query := tools.GetString(args, "query", "")
	if query == "" {
		return SearchTracesOutput{}, fmt.Errorf("query parameter must not be empty")
	}

	start, err := parseTime(tools.GetString(args, "start", ""))
	if err != nil {
		return SearchTracesOutput{}, fmt.Errorf("invalid start time: %v", err)
	}

	end, err := parseTime(tools.GetString(args, "end", ""))
	if err != nil {
		return SearchTracesOutput{}, fmt.Errorf("invalid end time: %v", err)
	}

	limit := tools.GetInt(args, "limit", 0)
	spss := tools.GetInt(args, "spss", 0)

	opts := tempoclient.SearchOptions{
		Query: query,
		Limit: limit,
		Start: start,
		End:   end,
		Spss:  spss,
	}

	results, err := client.Search(params.context, opts)
	if err != nil {
		return SearchTracesOutput{}, err
	}

	var output SearchTracesOutput
	if err := json.Unmarshal([]byte(results), &output); err != nil {
		return SearchTracesOutput{}, fmt.Errorf("failed to unmarshal search results: %w", err)
	}
	return output, nil
}
