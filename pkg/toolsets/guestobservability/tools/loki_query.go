package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
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

Guest telemetry follows a label contract in which namespace and vm_name identify the KubeVirt VM, while os and source classify the guest operating system and telemetry source.

Telemetry streams may have incomplete contract labels. A LogQL label matcher only returns streams containing a matching value for that label. Consequently, filtering by namespace, vm_name, os, or source excludes streams where that label is absent.

For Windows guest-log investigations, available classification labels such as os="windows" and source="windows_eventlog" can narrow the search. Specific event signatures such as provider names, event IDs, or diagnostic message text can be used in the LogQL content filter; for example, a Windows storage-reset investigation may be identified by StorPort or Event ID 129 content.

When an identity label such as namespace or vm_name is absent from telemetry, discovering that stream requires a bounded query that omits the missing identity matcher while retaining available classification labels and the specific event signature. A result without namespace cannot establish namespace attribution, and a result without vm_name cannot establish the affected VM.

Missing os or source labels represent classification uncertainty but do not by themselves invalidate identity when namespace and vm_name remain present.

Query results may include guest telemetry contract warnings identifying missing identity or classification labels and whether VM attribution is reliable.`,
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

	if value, ok := arguments["limit"].(int); ok {
		request.Limit = value
	}

	if value, ok := arguments["direction"].(string); ok {
		request.Direction = value
	}

	if value, ok := arguments["step"].(string); ok {
		request.Step = value
	}

	client, err := guestobservability.NewLoki(
		params,
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

	content = addGuestTelemetryContractWarnings(content)

	return api.NewToolCallResult(content, nil), nil
}

type guestTelemetryContractWarning struct {
	Missing          []string `json:"missing"`
	IdentityReliable bool     `json:"identityReliable"`
	Message          string   `json:"message"`
}

func addGuestTelemetryContractWarnings(content string) string {
	response, results, ok := guestTelemetryResults(content)
	if !ok {
		return content
	}

	warnings := guestTelemetryContractWarnings(results)
	if len(warnings) == 0 {
		return content
	}

	response["guestTelemetryContractWarnings"] = warnings

	enriched, err := json.Marshal(response)
	if err != nil {
		return content
	}

	return string(enriched)
}

func guestTelemetryResults(
	content string,
) (map[string]any, []any, bool) {
	var response map[string]any

	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, nil, false
	}

	data, ok := response["data"].(map[string]any)
	if !ok {
		return nil, nil, false
	}

	results, ok := data["result"].([]any)
	if !ok || len(results) == 0 {
		return nil, nil, false
	}

	return response, results, true
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
			newGuestTelemetryContractWarning(stream, missing),
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
	stream map[string]any,
	missing []string,
) guestTelemetryContractWarning {
	identityReliable :=
		hasNonEmptyStringLabel(stream, "namespace") &&
			hasNonEmptyStringLabel(stream, "vm_name")

	return guestTelemetryContractWarning{
		Missing:          missing,
		IdentityReliable: identityReliable,
		Message: guestTelemetryContractWarningMessage(
			missing,
		),
	}
}

func hasNonEmptyStringLabel(
	stream map[string]any,
	label string,
) bool {
	value, ok := stream[label].(string)

	return ok && value != ""
}

func guestTelemetryContractWarningMessage(
	missing []string,
) string {
	missingNamespace := containsString(
		missing,
		"namespace",
	)

	missingVMName := containsString(
		missing,
		"vm_name",
	)

	switch {
	case missingNamespace && missingVMName:
		return "Guest telemetry is missing namespace and vm_name. " +
			"The affected VM and namespace are UNKNOWN. " +
			"Do not name, rank, suggest, or speculate about candidate " +
			"VMs, and do not associate the event with a namespace " +
			"unless the telemetry contains an explicit correlation field."

	case missingVMName:
		return "Guest telemetry is missing vm_name. " +
			"The affected VM is UNKNOWN. " +
			"Do not name, rank, suggest, or speculate about candidate " +
			`VMs, including a "most likely" or "strongest candidate" VM. ` +
			"Do not infer VM identity from VM names, unrelated Kubernetes " +
			"workload metadata, or elimination among known VMs unless " +
			"the telemetry contains an explicit correlation field."

	case missingNamespace:
		return "Guest telemetry is missing namespace. " +
			"vm_name alone does not identify a namespaced VM. " +
			"Do not claim that the event belongs to the user's requested " +
			"namespace unless the telemetry contains an explicit " +
			"correlation field."

	default:
		return fmt.Sprintf(
			"Guest telemetry is missing classification label(s): %s. "+
				"Preserve this classification uncertainty rather than "+
				"inventing the missing value.",
			strings.Join(missing, ", "),
		)
	}
}

func containsString(
	values []string,
	target string,
) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}
