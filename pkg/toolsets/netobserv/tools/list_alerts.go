package tools

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
)

func InitListAlerts() []api.ServerTool {
	name := defaults.ToolsetName() + "_list_alerts"
	return []api.ServerTool{{
		Tool: api.Tool{
			Name: name,
			Description: "Lists Prometheus alerting or recording rules exposed by the NetObserv plugin. " +
				"Requires the plugin to proxy Prometheus (standalone deployment with AlertManager URL configured).",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"type": {
						Type:        "string",
						Description: "Rule type: alert or record.",
						Default:     api.ToRawMessage("alert"),
						Enum:        []any{"alert", "record"},
					},
					"match": {
						Type: "string",
						Description: "Prometheus label matcher without braces (e.g. alertname=NetObserv_* or namespace=openshift-netobserv). " +
							"Translated to match[]={match} on the API.",
					},
				},
			},
			Annotations: readOnlyAnnotations("List NetObserv Alert Rules"),
		},
		Handler: listAlertsHandler,
	}}
}

func listAlertsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	args := params.GetArguments()
	if args == nil {
		args = map[string]any{}
	}
	if _, ok := args["type"]; !ok {
		args["type"] = "alert"
	}
	client := netobservclient.NewNetObserv(params, params.KubernetesClient)
	content, err := client.ExecuteGet(params.Context, NetObservAlertRulesEndpoint, args)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list alert rules: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
