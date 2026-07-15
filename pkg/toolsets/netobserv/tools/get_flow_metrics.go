package tools

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
)

func InitGetFlowMetrics() []api.ServerTool {
	props := flowQueryProperties()
	props["dataSource"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Metrics backend: auto (prefer Prometheus, fallback to Loki), prom, or loki.",
		Default:     api.ToRawMessage(DefaultDataSource),
		Enum:        []any{"auto", "prom", "loki"},
	}
	props["type"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Metric type to aggregate.",
		Default:     api.ToRawMessage(DefaultMetricType),
		Enum: []any{
			"Flows", "Bytes", "Packets", "PktDropBytes", "PktDropPackets",
			"DnsLatencyMs", "DnsFlows", "TimeFlowRttNs",
		},
	}
	props["function"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Aggregation function.",
		Default:     api.ToRawMessage(DefaultMetricFunction),
		Enum:        []any{"count", "sum", "avg", "min", "max", "p90", "p99", "rate"},
	}
	props["aggregateBy"] = &jsonschema.Schema{
		Type:        "string",
		Description: aggregateByParameterDescription,
	}
	props["groups"] = &jsonschema.Schema{
		Type:        "string",
		Description: groupsParameterDescription,
	}
	props["rateInterval"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Prometheus rate interval (e.g. 1m, 5m).",
		Default:     api.ToRawMessage(DefaultRateInterval),
	}
	props["step"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Query resolution step (e.g. 30s, 1m).",
		Default:     api.ToRawMessage(DefaultStep),
	}

	name := defaults.ToolsetName() + "_get_flow_metrics"
	return []api.ServerTool{{
		Tool: api.Tool{
			Name:        name,
			Description: "Returns aggregated NetObserv flow metrics as topology or time-series data. Use for throughput, TLS/DNS/drop breakdowns, and namespace or workload traffic analysis; see aggregateBy and groups for grouping options.",
			InputSchema: toolInputSchema(props, []string{"aggregateBy"}),
			Annotations: readOnlyAnnotations("Get NetObserv Flow Metrics"),
		},
		Handler: getFlowMetricsHandler,
	}}
}

func getFlowMetricsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	client := netobservclient.NewNetObserv(params.Context, params, params.KubernetesClient, params.FilteringProvider)
	content, err := client.ExecuteGet(params.Context, NetObservFlowMetricsEndpoint, params.GetArguments())
	return jsonAPIResult(content, wrapAPIError("get flow metrics", err))
}
