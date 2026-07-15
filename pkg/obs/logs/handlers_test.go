package logs

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
)

type mockKubernetesClient struct {
	api.KubernetesClient
	restConfig    *rest.Config
	dynamicClient *dynamicfake.FakeDynamicClient
}

func (m *mockKubernetesClient) RESTConfig() *rest.Config {
	return m.restConfig
}

func (m *mockKubernetesClient) DynamicClient() dynamic.Interface {
	return m.dynamicClient
}

type mockBaseConfig struct {
	api.BaseConfig
	config *Config
}

func (m *mockBaseConfig) GetToolsetConfig(name string) (api.ExtendedConfig, bool) {
	if name == ToolsetName && m.config != nil {
		return m.config, true
	}
	return nil, false
}

type mockToolCallRequest struct {
	arguments map[string]any
}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return m.arguments
}

func newTestParams(t *testing.T, cfg *Config, dynamicClient *dynamicfake.FakeDynamicClient, args map[string]any) api.ToolHandlerParams {
	t.Helper()
	return api.ToolHandlerParams{
		Context:          t.Context(),
		KubernetesClient: &mockKubernetesClient{restConfig: &rest.Config{}, dynamicClient: dynamicClient},
		BaseConfig:       &mockBaseConfig{config: cfg},
		ToolCallRequest:  &mockToolCallRequest{arguments: args},
	}
}

func handlerParams(t *testing.T, lokiURL string, args map[string]any) api.ToolHandlerParams {
	t.Helper()
	return newTestParams(t, &Config{LokiURL: lokiURL}, nil, args)
}

func lokiServer(t *testing.T, responses map[string]mockResponse) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for pattern, resp := range responses {
			if pathMatchesLoki(r.URL.Path, pattern) {
				if resp.err != "" {
					http.Error(w, resp.err, http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, resp.body)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type mockResponse struct {
	body string
	err  string
}

func pathMatchesLoki(actual, pattern string) bool {
	switch pattern {
	case "labels":
		return actual == "/loki/api/v1/labels"
	case "label_values":
		return len(actual) > len("/loki/api/v1/label/") && actual != "/loki/api/v1/labels"
	case "query_range":
		return actual == "/loki/api/v1/query_range"
	}
	return false
}

func TestLabelValuesHandlerRequiresLabel(t *testing.T) {
	srv := lokiServer(t, nil)
	result, err := labelValuesHandler(handlerParams(t, srv.URL, map[string]any{}))
	require.NoError(t, err)
	require.Error(t, result.Error)
}

func TestQueryRangeHandlerDefaults(t *testing.T) {
	srv := lokiServer(t, map[string]mockResponse{
		"query_range": {body: `{"status":"success","data":{"resultType":"streams","result":[{"stream":{"namespace":"default"},"values":[["1000000000","line1"]]}]}}`},
	})
	result, err := queryRangeHandler(handlerParams(t, srv.URL, map[string]any{
		"query": `{namespace="default"}`,
	}))
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(QueryRangeOutput)
	require.Equal(t, "streams", output.ResultType)
}
