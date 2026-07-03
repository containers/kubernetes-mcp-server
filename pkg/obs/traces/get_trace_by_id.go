package traces

import (
	"encoding/json"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"

	tempoclient "github.com/rhobs/obs-mcp/pkg/traces/tempo"
)

// getTraceByIDOutput defines the output schema for the tempo_get_trace_by_id tool.
type getTraceByIDOutput struct {
	Trace any `json:"trace" jsonschema:"The trace data with services, scopes and spans"`
}

var getTraceByIDOutputSchema = mustSchema[getTraceByIDOutput]()

func initGetTraceByID() api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name: "tempo_get_trace_by_id",
			Description: `Retrieve a single distributed trace by its trace ID from Tempo.
Returns the full trace with all its spans, including service names, operation names, durations, and attributes.
Use this tool when you already have a specific trace ID, e.g. from search results or logs.`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"tempoNamespace": tempoNamespaceSchema,
					"tempoName":      tempoNameSchema,
					"tenant":         tempoTenantSchema,
					"traceid": {
						Type:        "string",
						Description: `The trace ID to retrieve, e.g. "26dad4a0e2b0dd9a440dd5ff203a24a4".`,
					},
					"start": {
						Type: "string",
						Description: `Optional start of the time range in RFC 3339 format, e.g. "2025-01-01T00:00:00Z".
Narrows the time range to improve query performance.`,
					},
					"end": {
						Type: "string",
						Description: `Optional end of the time range in RFC 3339 format, e.g. "2025-01-02T00:00:00Z".
Narrows the time range to improve query performance.`,
					},
				},
				Required: []string{"tempoNamespace", "tempoName", "traceid"},
			},
			OutputSchema: getTraceByIDOutputSchema,
			Annotations: api.ToolAnnotations{
				Title:           "Get trace by ID",
				ReadOnlyHint:    new(true),
				DestructiveHint: new(false),
				IdempotentHint:  new(true),
				OpenWorldHint:   new(true),
			},
		},
		Handler: getTraceByIDHandler,
	}
}

func getTraceByIDHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	traceid := p.RequiredString("traceid")
	startStr := p.OptionalString("start", "")
	endStr := p.OptionalString("end", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get trace by ID: %w", err)), nil
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

	trace, err := client.QueryV2(params.Context, traceid, tempoclient.QueryV2Options{
		Start: start,
		End:   end,
	})
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	var output getTraceByIDOutput
	if err := json.Unmarshal([]byte(trace), &output); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to unmarshal trace: %w", err)), nil
	}
	if output.Trace == nil {
		return api.NewToolCallResult("", fmt.Errorf("trace %s not found", traceid)), nil
	}
	return api.NewToolCallResultFull(trace, output, nil), nil
}
