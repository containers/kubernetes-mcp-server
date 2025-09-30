package prometheus

import (
	"context"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	p8s_api "github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// NewToolset returns a new toolset for Prometheus.
func NewToolset(cfg *config.PrometheusConfig) (api.Toolset, error) {
	if cfg == nil || cfg.URL == "" {
		return &disabledToolset{}, nil
	}

	client, err := p8s_api.NewClient(p8s_api.Config{
		Address: cfg.URL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus client: %w", err)
	}

	return &prometheusToolset{
		api: v1.NewAPI(client),
	}, nil
}

type prometheusToolset struct {
	api v1.API
}

func (t *prometheusToolset) GetName() string {
	return "prometheus"
}

func (t *prometheusToolset) GetDescription() string {
	return "Tools for interacting with Prometheus"
}

func (t *prometheusToolset) GetTools(_ kubernetes.Openshift) []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "prometheus.runQuery",
				Description: "Run a PromQL query.",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"query": {
							Type:        "string",
							Description: "The PromQL query to run.",
						},
					},
					Required: []string{"query"},
				},
				Annotations: api.ToolAnnotations{
					ReadOnlyHint: ptr.To(true),
				},
			},
			Handler: runQueryHandler(t.api),
		},
	}
}

func runQueryHandler(p8sAPI v1.API) api.ToolHandlerFunc {
	return func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		query, _ := params.GetArguments()["query"].(string)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, warnings, err := p8sAPI.Query(ctx, query, time.Now())
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to run query: %w", err)), nil
		}
		if len(warnings) > 0 {
			// Not treating warnings as errors for now
		}

		return api.NewToolCallResult(result.String(), nil), nil
	}
}

type disabledToolset struct{}

func (t *disabledToolset) GetName() string {
	return "prometheus"
}

func (t *disabledToolset) GetDescription() string {
	return "Prometheus toolset is disabled. Please configure it in the server settings."
}

func (t *disabledToolset) GetTools(_ kubernetes.Openshift) []api.ServerTool {
	return nil
}