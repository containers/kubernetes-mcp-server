package tools

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
)

func InitListAlertSilences() []api.ServerTool {
	name := defaults.ToolsetName() + "_list_alert_silences"
	return []api.ServerTool{{
		Tool: api.Tool{
			Name: name,
			Description: "Lists Alertmanager silences for NetObserv alerts. " +
				"Uses the console plugin proxy when available; on OpenShift (or when configured with alertmanager_url) falls back to cluster Alertmanager directly.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"filter": {
						Type:        "string",
						Description: "Alertmanager silence filter (e.g. alertname=MyAlert).",
					},
				},
			},
			Annotations: readOnlyAnnotations("List NetObserv Alert Silences"),
		},
		Handler: listAlertSilencesHandler,
	}}
}

func listAlertSilencesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	client := netobservclient.NewNetObserv(params, params.KubernetesClient)
	content, err := client.ExecuteGetAlertSilences(params.Context, params.GetArguments())
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list alert silences: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
