package prompts

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

type RetentionSuite struct {
	suite.Suite
}

func TestRetention(t *testing.T) {
	suite.Run(t, new(RetentionSuite))
}

func (s *RetentionSuite) TestKialiPromptsStopBeforeTheNextRequest() {
	huge := strings.Repeat("x", 8000)
	namespaces := `{"namespaces":[{"name":"ns"}]}`

	s.Run("service troubleshoot skips Istio config once the budget is full", func() {
		paths := s.paths(map[string]string{
			tools.KialiGetLogsEndpoint:               huge,
			tools.KialiManageIstioConfigReadEndpoint: "ISTIO",
		})
		result, err := serviceTroubleshootHandler(s.params(paths.srv, map[string]string{
			"namespace": "ns",
			"service":   "svc",
		}, int64(len(serviceTroubleshootAssistantText)+1500)))
		s.Require().NoError(err)
		text := result.Messages[0].Content.Text
		s.Contains(text, api.ResultTruncationNotice)
		s.NotContains(text, "ISTIO")
		s.NotContains(strings.Join(paths.hits, " "), tools.KialiManageIstioConfigReadEndpoint)
		s.Contains(strings.Join(paths.hits, " "), tools.KialiGetLogsEndpoint)
	})

	s.Run("istio config review skips the service list once the budget is full", func() {
		paths := s.paths(map[string]string{
			tools.KialiManageIstioConfigReadEndpoint: huge,
			tools.KialiListOrGetResourcesEndpoint:    "SERVICES",
		})
		result, err := istioConfigReviewHandler(s.params(paths.srv, map[string]string{
			"namespace": "ns",
		}, int64(len(istioConfigReviewAssistantText)+1500)))
		s.Require().NoError(err)
		text := result.Messages[0].Content.Text
		s.Contains(text, api.ResultTruncationNotice)
		s.NotContains(text, "SERVICES")
		s.NotContains(strings.Join(paths.hits, " "), tools.KialiListOrGetResourcesEndpoint)
	})

	s.Run("trace analysis skips error traces once the budget is full", func() {
		paths := s.paths(map[string]string{
			tools.KialiListTracesEndpoint: huge,
		})
		result, err := traceAnalysisHandler(s.params(paths.srv, map[string]string{
			"namespace": "ns",
			"service":   "svc",
		}, int64(len(traceAnalysisAssistantText)+1500)))
		s.Require().NoError(err)
		text := result.Messages[0].Content.Text
		s.Contains(text, api.ResultTruncationNotice)
		s.Len(paths.bodies, 1)
		s.NotContains(strings.Join(paths.bodies, " "), "errorOnly")
	})

	s.Run("mesh topology skips namespace discovery once the budget is full", func() {
		paths := s.paths(map[string]string{
			tools.KialiGetMeshStatusEndpoint:       huge,
			tools.KialiListOrGetResourcesEndpoint:  namespaces,
			tools.KialiGetMeshTrafficGraphEndpoint: "GRAPH",
		})
		result, err := meshTopologyHandler(s.params(paths.srv, nil, 1500))
		s.Require().NoError(err)
		text := result.Messages[0].Content.Text
		s.Contains(text, api.ResultTruncationNotice)
		s.NotContains(text, "GRAPH")
		joined := strings.Join(paths.hits, " ")
		s.Contains(joined, tools.KialiGetMeshStatusEndpoint)
		s.NotContains(joined, tools.KialiListOrGetResourcesEndpoint)
		s.NotContains(joined, tools.KialiGetMeshTrafficGraphEndpoint)
	})

	s.Run("service troubleshoot still fetches Istio config when the budget is open", func() {
		paths := s.paths(map[string]string{
			tools.KialiGetLogsEndpoint:               "LOGS",
			tools.KialiManageIstioConfigReadEndpoint: "ISTIO",
		})
		result, err := serviceTroubleshootHandler(s.params(paths.srv, map[string]string{
			"namespace": "ns",
			"service":   "svc",
		}, 0))
		s.Require().NoError(err)
		text := result.Messages[0].Content.Text
		s.Contains(text, "LOGS")
		s.Contains(text, "ISTIO")
		s.Contains(text, "## Analysis Instructions")
	})
}

type recordedPaths struct {
	srv    *httptest.Server
	hits   []string
	bodies []string
}

func (s *RetentionSuite) paths(bodies map[string]string) *recordedPaths {
	rec := &recordedPaths{}
	rec.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec.hits = append(rec.hits, r.URL.Path)
		rec.bodies = append(rec.bodies, string(raw))
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	s.T().Cleanup(rec.srv.Close)
	return rec
}

func (s *RetentionSuite) params(srv *httptest.Server, args map[string]string, limit int64) api.PromptHandlerParams {
	cfg, err := config.ReadToml(s.T().Context(), []byte(`
[toolset_configs.kiali]
url = "`+srv.URL+`"
`))
	s.Require().NoError(err)
	ctx := context.Background()
	if limit > 0 {
		ctx = api.WithResultBudget(ctx, int64(limit))
	}
	return api.PromptHandlerParams{
		Context:          ctx,
		Config:           cfg,
		KubernetesClient: kialiPromptClient{},
		Request:          kialiArgs{args: args},
	}
}

type kialiArgs struct {
	args map[string]string
}

func (a kialiArgs) GetArguments() map[string]string {
	if a.args == nil {
		return map[string]string{}
	}
	return a.args
}

type kialiPromptClient struct {
	kubernetes.Interface
}

func (kialiPromptClient) NamespaceOrDefault(ns string) string { return ns }
func (kialiPromptClient) ToRESTConfig() (*rest.Config, error) { return &rest.Config{}, nil }
func (kialiPromptClient) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return nil, errKialiPromptClient
}
func (kialiPromptClient) ToRESTMapper() (meta.RESTMapper, error) { return nil, errKialiPromptClient }
func (kialiPromptClient) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return nil
}
func (kialiPromptClient) RESTConfig() *rest.Config { return &rest.Config{} }
func (kialiPromptClient) RESTMapper() meta.ResettableRESTMapper {
	return nil
}
func (kialiPromptClient) DiscoveryClient() discovery.CachedDiscoveryInterface { return nil }
func (kialiPromptClient) RawDiscoveryClient() discovery.DiscoveryInterface    { return nil }
func (kialiPromptClient) DynamicClient() dynamic.Interface                    { return nil }
func (kialiPromptClient) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return nil
}

var errKialiPromptClient = errString("unused")

type errString string

func (e errString) Error() string { return string(e) }
