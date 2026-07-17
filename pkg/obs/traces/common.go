package traces

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/rhobs/obs-mcp/pkg/auth"
	"github.com/rhobs/obs-mcp/pkg/instrumentation"
	"github.com/rhobs/obs-mcp/pkg/metrics/prometheus"
	"github.com/rhobs/obs-mcp/pkg/traces/discovery"
	tempoclient "github.com/rhobs/obs-mcp/pkg/traces/tempo"
)

var tempoStackGVK = schema.GroupVersionKind{
	Group:   "tempo.grafana.com",
	Version: "v1alpha1",
	Kind:    "TempoStack",
}

var (
	tempoNamespaceSchema = &jsonschema.Schema{
		Type:        "string",
		Description: "The Kubernetes namespace where the Tempo instance is deployed. Use tempo_list_instances to discover available namespaces.",
	}
	tempoNameSchema = &jsonschema.Schema{
		Type:        "string",
		Description: "The name of the Tempo instance to query. Use tempo_list_instances to discover available instance names.",
	}
	tempoTenantSchema = &jsonschema.Schema{
		Type:        "string",
		Description: "The tenant to query. This parameter is required for multi-tenant instances. Use tempo_list_instances to discover available tenants for each instance.",
	}
)

func hasTempoStackCRD(p api.FilteringProvider) func() bool {
	return func() bool {
		return p.AnyTargetHasGVKs(context.TODO(), []schema.GroupVersionKind{
			tempoStackGVK,
		})
	}
}

// getTempoClient returns a Tempo client based on the config and tempoNamespace, tempoName and tenant parameters.
// When a static TempoURL is configured, it is used directly without discovery.
// Otherwise, the Tempo instance is resolved via Kubernetes discovery using the provided parameters.
func getTempoClient(params api.ToolHandlerParams) (tempoclient.Loader, error) {
	cfg := getToolsetConfig(params)

	url, err := resolveTempoURL(params)
	if err != nil {
		return nil, err
	}

	tls := strings.HasPrefix(url, "https://")
	rt, err := auth.BuildRoundTripper(params.Context, params.RESTConfig(), cfg.GetAuthMode(), tls, cfg.Insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create round tripper: %w", err)
	}

	rt = instrumentation.RoundTripper(rt, cfg.ClientMetrics, "tempo")

	httpClient := &http.Client{
		Timeout:   tempoclient.RequestTimeout,
		Transport: rt,
	}
	return tempoclient.NewTempoLoader(httpClient, url), nil
}

func resolveTempoURL(params api.ToolHandlerParams) (string, error) {
	cfg := getToolsetConfig(params)
	if cfg != nil && cfg.TempoURL != "" {
		return cfg.TempoURL, nil
	}

	p := api.WrapParams(params)
	namespace := p.RequiredString("tempoNamespace")
	name := p.RequiredString("tempoName")
	tenant := p.OptionalString("tenant", "")
	if namespace == "" && name == "" {
		return "", fmt.Errorf("tempo URL not configured; set tempo_url/--traces.tempo-url/TEMPO_URL or provide tempoNamespace and tempoName")
	}
	if err := p.Err(); err != nil {
		return "", err
	}
	if namespace == "" {
		return "", errors.New("tempoNamespace parameter must not be empty")
	}
	if name == "" {
		return "", errors.New("tempoName parameter must not be empty")
	}

	instances, err := discovery.ListInstances(params.Context, params.DynamicClient(), cfg.UseRoute)
	if err != nil {
		return "", err
	}

	// Make sure this Tempo instance exists in cluster. Otherwise, an attacker could potentially trick the MCP tool to connect to non-Tempo services.
	instance, err := findInstanceByName(instances, namespace, name)
	if err != nil {
		return "", err
	}

	if instance.Multitenancy {
		if tenant == "" {
			return "", errors.New("tenant parameter must not be empty for multi-tenant instance")
		}
		if !slices.Contains(instance.Tenants, tenant) {
			return "", fmt.Errorf("tenant '%s' does not exist for instance '%s' in namespace '%s'", tenant, name, namespace)
		}
	}

	return instance.GetURL(tenant), nil
}

func findInstanceByName(instances []discovery.TempoInstance, namespace, name string) (discovery.TempoInstance, error) {
	for _, instance := range instances {
		if instance.Namespace == namespace && instance.Name == name {
			return instance, nil
		}
	}

	return discovery.TempoInstance{}, fmt.Errorf("instance '%s' in namespace '%s' not found", name, namespace)
}

func parseTime(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}

	ts, err := prometheus.ParseTimestamp(s)
	if err != nil {
		return 0, err
	}
	return ts.Unix(), nil
}
