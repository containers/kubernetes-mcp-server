package traces

import (
	"encoding/json"
	"fmt"

	"github.com/rhobs/obs-mcp/pkg/tools"
	tempoclient "github.com/rhobs/obs-mcp/pkg/traces/tempo"
)

// GetTraceByIDOutput defines the output schema for the tempo_get_trace_by_id tool.
type GetTraceByIDOutput struct {
	Trace any `json:"trace" jsonschema:"The trace data with services, scopes and spans"`
}

var GetTraceByIDTool = tools.ToolDef[GetTraceByIDOutput]{
	Name: "tempo_get_trace_by_id",
	Description: `Retrieve a single distributed trace by its trace ID from Tempo.
Returns the full trace with all its spans, including service names, operation names, durations, and attributes.
Use this tool when you already have a specific trace ID, e.g. from search results or logs.`,
	Title: "Get trace by ID",
	Params: []tools.ParamDef{
		tempoNamespaceParameter,
		tempoNameParameter,
		tempoTenantParameter,
		{
			Name:        "traceid",
			Type:        tools.ParamTypeString,
			Description: `The trace ID to retrieve, e.g. "26dad4a0e2b0dd9a440dd5ff203a24a4".`,
			Required:    true,
		},
		{
			Name: "start",
			Type: tools.ParamTypeString,
			Description: `Optional start of the time range in RFC 3339 format, e.g. "2025-01-01T00:00:00Z".
Narrows the time range to improve query performance.`,
		},
		{
			Name: "end",
			Type: tools.ParamTypeString,
			Description: `Optional end of the time range in RFC 3339 format, e.g. "2025-01-02T00:00:00Z".
Narrows the time range to improve query performance.`,
		},
	},
	ReadOnly:    true,
	Destructive: false,
	Idempotent:  true,
	OpenWorld:   true,
}

func (t *Toolset) GetTraceByIDHandler(params ToolParams) (GetTraceByIDOutput, error) {
	client, err := t.getTempoClient(params)
	if err != nil {
		return GetTraceByIDOutput{}, err
	}

	args := params.arguments

	traceid := tools.GetString(args, "traceid", "")
	if traceid == "" {
		return GetTraceByIDOutput{}, fmt.Errorf("traceid parameter must not be empty")
	}

	start, err := parseTime(tools.GetString(args, "start", ""))
	if err != nil {
		return GetTraceByIDOutput{}, fmt.Errorf("invalid start time: %v", err)
	}

	end, err := parseTime(tools.GetString(args, "end", ""))
	if err != nil {
		return GetTraceByIDOutput{}, fmt.Errorf("invalid end time: %v", err)
	}

	opts := tempoclient.QueryV2Options{
		Start: start,
		End:   end,
	}

	trace, err := client.QueryV2(params.context, traceid, opts)
	if err != nil {
		return GetTraceByIDOutput{}, err
	}

	var output GetTraceByIDOutput
	if err := json.Unmarshal([]byte(trace), &output); err != nil {
		return GetTraceByIDOutput{}, fmt.Errorf("failed to unmarshal trace: %w", err)
	}
	return output, nil
}
