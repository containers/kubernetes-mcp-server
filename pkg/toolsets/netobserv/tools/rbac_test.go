package tools

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RBACSuite struct {
	suite.Suite
}

type stubInspector struct {
	isOpenShift bool
}

func (m *stubInspector) Discovery() api.AggregateDiscovery       { return m }
func (m *stubInspector) Unstructured() api.AggregateUnstructured { return nil }

func (m *stubInspector) ServerResourcesForGroupVersion(ctx context.Context, _ string) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, stubTargetProvider{}, func(context.Context, string) (*metav1.APIResourceList, error) {
		if m.isOpenShift {
			return &metav1.APIResourceList{APIResources: []metav1.APIResource{{Kind: "Project"}}}, nil
		}
		return &metav1.APIResourceList{}, nil
	})
}

type stubTargetProvider struct{}

func (stubTargetProvider) IsMultiTarget() bool { return false }
func (stubTargetProvider) GetTargets(context.Context) ([]string, error) {
	return []string{"default"}, nil
}
func (stubTargetProvider) GetDefaultTarget() string       { return "default" }
func (stubTargetProvider) GetTargetParameterName() string { return "target" }

var _ api.ClusterInspector = (*stubInspector)(nil)
var _ api.AggregateDiscovery = (*stubInspector)(nil)

func (s *RBACSuite) TestFlowsRBAC() {
	s.Run("bounded loki-reader on OpenShift", func() {
		meta := flowsRBAC(&stubInspector{isOpenShift: true})
		s.Require().NoError(meta.Validate())
		s.Require().NotNil(meta.Bounded)
		s.Require().Len(meta.Bounded.Requirements, 1)
		req := meta.Bounded.Requirements[0]
		s.Equal([]string{"get"}, req.Verbs)
		s.Require().NotNil(req.Target.Resource)
		s.Equal("loki.grafana.com", req.Target.Resource.APIGroup)
		s.Equal("network", req.Target.Resource.Resource)
		s.Require().NotNil(req.ResourceName)
		s.Equal("logs", req.ResourceName.Name)
		s.Nil(req.Namespace)
	})

	s.Run("unbounded off OpenShift", func() {
		meta := flowsRBAC(&stubInspector{isOpenShift: false})
		s.Require().NoError(meta.Validate())
		s.Require().NotNil(meta.Unbounded)
		s.NotEmpty(meta.Unbounded.Reason)
	})

	s.Run("unbounded without provider", func() {
		meta := flowsRBAC(nil)
		s.Require().NoError(meta.Validate())
		s.NotNil(meta.Unbounded)
	})
}

func (s *RBACSuite) TestMetricsRBAC() {
	s.Run("bounded metrics-reader on OpenShift", func() {
		meta := metricsRBAC(&stubInspector{isOpenShift: true})
		s.Require().NoError(meta.Validate())
		s.Require().NotNil(meta.Bounded)
		s.Require().Len(meta.Bounded.Requirements, 1)
		req := meta.Bounded.Requirements[0]
		s.Equal([]string{"create"}, req.Verbs)
		s.Require().NotNil(req.Target.Resource)
		s.Equal("metrics.k8s.io", req.Target.Resource.APIGroup)
		s.Equal("pods", req.Target.Resource.Resource)
		s.Nil(req.ResourceName)
	})

	s.Run("unbounded off OpenShift", func() {
		meta := metricsRBAC(&stubInspector{isOpenShift: false})
		s.Require().NoError(meta.Validate())
		s.Require().NotNil(meta.Unbounded)
		s.NotEmpty(meta.Unbounded.Reason)
	})
}

func (s *RBACSuite) TestDetectedBackendRBAC() {
	s.Run("LokiStack is bounded regardless of cluster platform", func() {
		meta := flowsRBAC(nil, netobservclient.EffectiveConfig{Found: true, LokiEnabled: true, LokiMode: "LokiStack"})
		s.Require().NotNil(meta)
		s.NotNil(meta.Bounded)
		s.Nil(meta.Unbounded)
	})

	s.Run("other Loki modes skip RBAC declaration", func() {
		meta := flowsRBAC(&stubInspector{isOpenShift: true}, netobservclient.EffectiveConfig{Found: true, LokiEnabled: true, LokiMode: "Manual"})
		s.Nil(meta)
	})

	s.Run("disabled Loki skips flow RBAC declaration", func() {
		meta := flowsRBAC(&stubInspector{isOpenShift: true}, netobservclient.EffectiveConfig{Found: true, LokiEnabled: false})
		s.Nil(meta)
	})

	s.Run("unknown detection remains fail open", func() {
		meta := flowsRBAC(&stubInspector{isOpenShift: true}, netobservclient.EffectiveConfig{Unknown: true})
		s.Require().NotNil(meta)
		s.NotNil(meta.Unbounded)
	})
}

func (s *RBACSuite) TestInitToolsCarryRBAC() {
	ocp := &stubInspector{isOpenShift: true}
	plain := &stubInspector{isOpenShift: false}

	for name, tools := range map[string][]api.ServerTool{
		"list_flows":   InitListFlows(ocp),
		"export_flows": InitExportFlows(ocp),
	} {
		s.Run(name+" bounded on OpenShift", func() {
			s.Require().Len(tools, 1)
			s.Require().NotNil(tools[0].RBAC)
			s.NotNil(tools[0].RBAC.Bounded)
		})
	}

	s.Run("get_flow_metrics bounded on OpenShift", func() {
		tools := InitGetFlowMetrics(ocp)
		s.Require().Len(tools, 1)
		s.Require().NotNil(tools[0].RBAC.Bounded)
	})

	s.Run("unbounded off OpenShift", func() {
		s.NotNil(InitListFlows(plain)[0].RBAC.Unbounded)
		s.NotNil(InitExportFlows(plain)[0].RBAC.Unbounded)
		s.NotNil(InitGetFlowMetrics(plain)[0].RBAC.Unbounded)
	})
}

func TestRBAC(t *testing.T) {
	suite.Run(t, new(RBACSuite))
}
