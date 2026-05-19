package tools

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"
)

func flowQueryProperties() map[string]*jsonschema.Schema {
	return map[string]*jsonschema.Schema{
		"namespace": {
			Type:        "string",
			Description: "Restrict results to flows where source or destination namespace matches (dev-scoped Loki tenant).",
		},
		"timeRange": {
			Type:        "integer",
			Description: "Lookback window in seconds when startTime is omitted. Default 300.",
			Default:     api.ToRawMessage(DefaultTimeRangeSeconds),
		},
		"startTime": {
			Type:        "integer",
			Description: "Start of time range as Unix epoch seconds. Overrides timeRange when set.",
		},
		"endTime": {
			Type:        "integer",
			Description: "End of time range as Unix epoch seconds. Defaults to now.",
		},
		"limit": {
			Type:        "integer",
			Description: "Maximum number of flow records to return. Default 100.",
			Default:     api.ToRawMessage(DefaultLimit),
		},
		"recordType": {
			Type:        "string",
			Description: "Flow record type filter.",
			Default:     api.ToRawMessage(DefaultRecordType),
			Enum: []any{
				"flowLog",
				"allConnections",
				"newConnection",
				"heartbeat",
				"endConnection",
			},
		},
		"packetLoss": {
			Type:        "string",
			Description: "Packet loss filter.",
			Default:     api.ToRawMessage(DefaultPacketLoss),
			Enum:        []any{"all", "dropped", "hasDrops", "sent"},
		},
		"filters": {
			Type:        "string",
			Description: filtersParameterDescription,
		},
	}
}

func readOnlyAnnotations(title string) api.ToolAnnotations {
	return api.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    ptr.To(true),
		DestructiveHint: ptr.To(false),
		IdempotentHint:  ptr.To(true),
		OpenWorldHint:   ptr.To(true),
	}
}
