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

Guest telemetry uses a label contract for reliable VM attribution. The namespace and vm_name labels together identify the KubeVirt VM that produced the telemetry. The os and source labels classify the guest operating system and telemetry source.

For Windows guest-log investigations, use os="windows" or source="windows_eventlog" when those classification labels are available.

If namespace or vm_name is missing, the telemetry may still establish that an event occurred, but it cannot reliably identify the affected VM. Do not infer missing VM identity from unrelated Kubernetes workload metadata.

If a namespace-scoped search does not find the specific event being investigated, perform one bounded follow-up query without the namespace matcher while retaining the strongest available classification labels and event identifier. Telemetry returned without namespace must not be attributed to the requested namespace.

If os or source is missing, report the missing classification field rather than inventing it; the remaining labels and log content may still provide useful diagnostic evidence.

Missing contract labels are surfaced explicitly so callers can distinguish reliable attribution from incomplete telemetry.`,
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
	var response map[string]any

	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return content
	}

	data, ok := response["data"].(map[string]any)
	if !ok {
		return content
	}

	results, ok := data["result"].([]any)
	if !ok || len(results) == 0 {
		return content
	}

	warnings := make([]guestTelemetryContractWarning, 0)
	seen := make(map[string]struct{})

	for _, rawResult := range results {
		result, ok := rawResult.(map[string]any)
		if !ok {
			continue
		}

		stream, ok := result["stream"].(map[string]any)
		if !ok {
			continue
		}

		missing := make([]string, 0, 4)

		for _, label := range []string{
			"namespace",
			"vm_name",
			"os",
			"source",
		} {
			if !hasNonEmptyStringLabel(stream, label) {
				missing = append(missing, label)
			}
		}

		if len(missing) == 0 {
			continue
		}

		/*
		   Deduplicate warnings for streams with the same missing-label
		   combination. A Loki result may contain many streams with the
		   same telemetry-contract problem, and repeating the same
		   warning adds noise without adding information.
		*/
		key := strings.Join(missing, ",")

		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}

		identityReliable :=
			hasNonEmptyStringLabel(stream, "namespace") &&
				hasNonEmptyStringLabel(stream, "vm_name")

		warnings = append(
			warnings,
			guestTelemetryContractWarning{
				Missing:          missing,
				IdentityReliable: identityReliable,
				Message: guestTelemetryContractWarningMessage(
					missing,
				),
			},
		)
	}

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
