package tools

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	guestobservability "github.com/containers/kubernetes-mcp-server/pkg/guestobservability"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/guestobservability/internal/defaults"
)

func InitLokiQuery() []api.ServerTool {
	name := defaults.ToolsetName() + "_loki_query"

	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name: name,
				Description: `Executes a LogQL range query against the configured Loki backend for virtual machine guest log investigation.

Guest telemetry uses namespace and vm_name as VM identity labels, while os and source classify the guest operating system and telemetry source. A collector label may also be available for orphan guest telemetry when the standard contract labels are absent.

Loki label matchers exclude streams where the matched label is absent. An empty result from a query using namespace, vm_name, os, or source therefore does not prove that matching guest events are absent. When relevant telemetry may have incomplete labels, a bounded follow-up query can use progressively fewer contract-label matchers while preserving filters for the event itself.

A missing label matcher should not be replaced with a log-content filter containing that label's value unless the value is known to occur in the log message. Removing namespace="example" and adding |= "example", for example, does not establish namespace association.

For event-content searches, stable event identifiers and case-insensitive matching are useful when provider or component capitalization is not guaranteed. For Windows guest-log investigations, available classification labels such as os="windows" or source="windows_eventlog" should be retained in the initial query together with the event-content filter. Windows storage reset telemetry commonly contains Event ID 129 and the StorPort provider.

If an event is discovered after identity labels are removed, event detection and VM attribution must be treated separately. Reliable VM identity requires both namespace and vm_name from the telemetry. Missing os or source represents classification uncertainty but does not by itself invalidate identity when namespace and vm_name are present.

Results may include guest telemetry contract warnings identifying missing identity or classification labels and whether VM attribution is reliable.`,
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"query": {
							Type:        "string",
							Description: "LogQL query expression to execute.",
						},
						"start": {
							Type: "string",
							Description: `Optional RFC3339 start timestamp with explicit timezone.
When start and end are omitted, the query defaults to the previous one
hour ending at the server's current time.`,
						},
						"end": {
							Type: "string",
							Description: `Optional RFC3339 end timestamp with explicit timezone.
When start and end are omitted, the query defaults to the previous one
hour ending at the server's current time.`,
						},
						"limit": {
							Type:        "integer",
							Description: "Maximum raw log entries to return. Defaults to 100 and is capped at 1000.",
							Default:     api.ToRawMessage(guestobservability.DefaultLokiLimit),
						},
						"direction": {
							Type:        "string",
							Description: "Log ordering direction: backward returns newest matching entries first; forward returns oldest matching entries first.",
							Default:     api.ToRawMessage(guestobservability.DefaultLokiDirection),
							Enum:        []any{"forward", "backward"},
						},
						"step": {
							Type:        "string",
							Description: "Optional query resolution for metric-style LogQL range queries, for example 30s, 1m, or 5m.",
						},
					},
					Required: []string{"query"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Query Guest Logs with LogQL",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: lokiQueryHandler,
		},
	}
}

func lokiQueryHandler(
	params api.ToolHandlerParams,
) (*api.ToolCallResult, error) {
	arguments := params.GetArguments()

	query, ok := arguments["query"].(string)
	if !ok || query == "" {
		return api.NewToolCallResult(
			"",
			fmt.Errorf("missing required argument query"),
		), nil
	}

	request := guestobservability.LokiQueryRangeRequest{
		Query: query,
	}

	if value, ok := arguments["start"].(string); ok {
		request.Start = value
	}

	if value, ok := arguments["end"].(string); ok {
		request.End = value
	}

	if value, ok := arguments["limit"].(float64); ok {
		request.Limit = int(value)
	}

	if value, ok := arguments["direction"].(string); ok {
		request.Direction = value
	}

	if value, ok := arguments["step"].(string); ok {
		request.Step = value
	}

	var restConfig *rest.Config
	if params.KubernetesClient != nil {
		restConfig = params.RESTConfig()
	}

	client, err := guestobservability.NewLoki(
		params,
		restConfig,
	)
	if err != nil {
		return api.NewToolCallResult(
			"",
			fmt.Errorf(
				"failed to initialize Loki client: %w",
				err,
			),
		), nil
	}

	content, err := client.QueryRange(
		params.Context,
		request,
	)
	if err != nil {
		return api.NewToolCallResult(
			"",
			fmt.Errorf(
				"failed to query Loki: %w",
				err,
			),
		), nil
	}

	return lokiQueryResult(content), nil
}

type guestTelemetryContractWarning struct {
	Missing          []string `json:"missing"`
	IdentityReliable bool     `json:"identityReliable"`
}

func lokiQueryResult(content string) *api.ToolCallResult {
	response, ok := guestTelemetryStructuredResult(content)
	if !ok {
		return api.NewToolCallResult(content, nil)
	}

	return api.NewToolCallResultStructured(response, nil)
}

func guestTelemetryStructuredResult(
	content string,
) (map[string]any, bool) {
	var response map[string]any

	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, false
	}

	addGuestTelemetryContractWarnings(response)

	return response, true
}

func addGuestTelemetryContractWarnings(
	response map[string]any,
) {
	results, ok := guestTelemetryResults(response)
	if !ok {
		return
	}

	warnings := guestTelemetryContractWarnings(results)
	if len(warnings) == 0 {
		return
	}

	response["guestTelemetryContractWarnings"] = warnings
}

func guestTelemetryResults(
	response map[string]any,
) ([]any, bool) {
	data, ok := response["data"].(map[string]any)
	if !ok {
		return nil, false
	}

	results, ok := data["result"].([]any)
	if !ok || len(results) == 0 {
		return nil, false
	}

	return results, true
}

func guestTelemetryContractWarnings(
	results []any,
) []guestTelemetryContractWarning {
	warnings := make([]guestTelemetryContractWarning, 0)
	seen := make(map[string]struct{})

	for _, rawResult := range results {
		stream, ok := guestTelemetryStream(rawResult)
		if !ok {
			continue
		}

		missing := missingGuestTelemetryContractLabels(stream)
		if len(missing) == 0 {
			continue
		}

		key := strings.Join(missing, ",")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		warnings = append(
			warnings,
			newGuestTelemetryContractWarning(missing),
		)
	}

	return warnings
}

func guestTelemetryStream(
	rawResult any,
) (map[string]any, bool) {
	result, ok := rawResult.(map[string]any)
	if !ok {
		return nil, false
	}

	stream, ok := result["stream"].(map[string]any)
	if !ok {
		return nil, false
	}

	return stream, true
}

func missingGuestTelemetryContractLabels(
	stream map[string]any,
) []string {
	labels := []string{
		"namespace",
		"vm_name",
		"os",
		"source",
	}

	missing := make([]string, 0, len(labels))

	for _, label := range labels {
		if !hasNonEmptyStringLabel(stream, label) {
			missing = append(missing, label)
		}
	}

	return missing
}

func newGuestTelemetryContractWarning(
	missing []string,
) guestTelemetryContractWarning {
	identityReliable :=
		!slices.Contains(missing, "namespace") &&
			!slices.Contains(missing, "vm_name")

	return guestTelemetryContractWarning{
		Missing:          missing,
		IdentityReliable: identityReliable,
	}
}

func hasNonEmptyStringLabel(
	stream map[string]any,
	label string,
) bool {
	value, ok := stream[label].(string)

	return ok && value != ""
}
