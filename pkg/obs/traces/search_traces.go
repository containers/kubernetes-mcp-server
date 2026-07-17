package traces

import (
	"encoding/json"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"

	"github.com/rhobs/obs-mcp/pkg/tools"
	tempoclient "github.com/rhobs/obs-mcp/pkg/traces/tempo"
)

// searchTracesOutput defines the output schema for the tempo_search_traces tool.
type searchTracesOutput struct {
	Traces  []any `json:"traces" jsonschema:"List of matching traces with metadata"`
	Metrics any   `json:"metrics,omitempty" jsonschema:"Query performance metrics"`
}

var searchTracesOutputSchema = tools.MustSchema[searchTracesOutput]()

func initSearchTraces() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name: "tempo_search_traces",
			Description: `Search for distributed traces in Tempo using TraceQL.
Use this tool to find traces matching specific criteria such as service name, HTTP status code, duration, or other span or resource attributes.

IMPORTANT — "slow" or "long" trace requests: Do NOT guess a duration threshold.
First call this tool WITHOUT a duration filter to establish a latency baseline, then use that baseline to set a sensible threshold.
Both steps are required — do NOT skip the second search with the duration filter.
Skip this two-step process only when the user provides an explicit duration (e.g. "find traces slower than 2s").
`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"tempoNamespace": tempoNamespaceSchema,
					"tempoName":      tempoNameSchema,
					"tenant":         tempoTenantSchema,
					"query": {
						Type: "string",
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
- resource.k8s.namespace.name (Kubernetes namespace)
- resource.k8s.deployment.name (Kubernetes deployment)
- resource.k8s.statefulset.name (Kubernetes statefulset)
- resource.k8s.daemonset.name (Kubernetes daemonset)
- resource.k8s.replicaset.name (Kubernetes replicaset)
- resource.k8s.pod.name (Kubernetes pod)
- resource.k8s.container.name (Kubernetes container)
- resource.k8s.job.name (Kubernetes job)
- resource.k8s.cronjob.name (Kubernetes cronjob)
- resource.k8s.node.name (Kubernetes node)
- resource.k8s.cluster.name (Kubernetes cluster)
- span.http.response.status_code (HTTP response code)
- span.http.request.method (HTTP method like GET, POST)
- span.url.full (request URL)
- name (span name / operation name, e.g. "GET /api/users")
- duration (trace duration, e.g. 100ms, 2s)
- status (trace status: ok, error, unset)

Note: older instrumentation may use legacy HTTP attribute names (e.g. span.http.status_code instead of span.http.response.status_code).
If a query returns no results, try tempo_search_tags to check which attributes exist.

IMPORTANT:
- Always wrap filters in curly braces { }.
- Do NOT use SQL, PromQL, or Lucene syntax.
- Do NOT omit the "resource." or "span." prefix from attribute names
- When the user refers to a Kubernetes resource type (deployment, pod, namespace, etc.), use the matching resource.k8s.* attribute, NOT resource.service.name.

Examples:
- { resource.service.name="frontend" }
- { resource.k8s.deployment.name="checkout" && span.http.response.status_code>=500 }
- { status=error && duration>2s }

If unsure which attributes to filter on, use tempo_search_tags to discover available attributes before building a query.
`,
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of traces to return. Defaults to the server-side limit if not specified.",
					},
					"start": {
						Type: "string",
						Description: `Start of the time range in RFC 3339 format, e.g. "2025-01-01T00:00:00Z".
Use "NOW" for current time.
Both start and end should be provided to search the full time range; if omitted, only a small window of recent data is searched.`,
					},
					"end": {
						Type: "string",
						Description: `End of the time range in RFC 3339 format, e.g. "2025-01-01T00:00:00Z".
Use "NOW" for current time.
Both start and end should be provided to search the full time range; if omitted, only a small window of recent data is searched.`,
					},
					"spss": {
						Type:        "integer",
						Description: "Maximum number of matching spans to return per trace.",
					},
				},
				Required: []string{"tempoNamespace", "tempoName", "query"},
			},
			OutputSchema: searchTracesOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Search traces",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: searchTracesHandler,
	}
}

func searchTracesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	query := p.RequiredString("query")
	startStr := p.OptionalString("start", "")
	endStr := p.OptionalString("end", "")
	limit := int(p.OptionalInt64("limit", 0))
	spss := int(p.OptionalInt64("spss", 0))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to search traces: %w", err)), nil
	}
	if query == "" {
		return api.NewToolCallResult("", fmt.Errorf("query parameter must not be empty")), nil
	}

	start, err := parseTime(startStr)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("invalid start time: %w", err)), nil
	}

	end, err := parseTime(endStr)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("invalid end time: %w", err)), nil
	}

	client, err := getTempoClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	results, err := client.Search(params.Context, tempoclient.SearchOptions{
		Query: query,
		Limit: limit,
		Start: start,
		End:   end,
		Spss:  spss,
	})
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	var output searchTracesOutput
	if err := json.Unmarshal([]byte(results), &output); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to unmarshal search results: %w", err)), nil
	}
	return api.NewToolCallResultFull(results, output, nil), nil
}
