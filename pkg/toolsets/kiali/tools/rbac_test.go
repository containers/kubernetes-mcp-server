package tools

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RBACSuite struct {
	suite.Suite
}

// stubInspector reports a fixed discovery result, standing in for
// OpenShift (project.openshift.io present) or plain Kubernetes.
type stubInspector struct {
	isOpenShift bool
}

func (m *stubInspector) Discovery() api.AggregateDiscovery       { return m }
func (m *stubInspector) Unstructured() api.AggregateUnstructured { return nil }
func (m *stubInspector) GetTargets(context.Context) ([]string, error) {
	return []string{""}, nil
}
func (m *stubInspector) GetDefaultTarget() string       { return "" }
func (m *stubInspector) GetTargetParameterName() string { return "" }
func (m *stubInspector) IsMultiTarget() bool            { return false }
func (m *stubInspector) ServerResourcesForGroupVersion(ctx context.Context, _ string) api.Results[*metav1.APIResourceList] {
	return api.NewResults(ctx, m, func(context.Context, string) (*metav1.APIResourceList, error) {
		list := &metav1.APIResourceList{}
		if m.isOpenShift {
			list.APIResources = []metav1.APIResource{{Kind: "Project"}}
		}
		return list, nil
	})
}

func (s *RBACSuite) TestLogsRBAC() {
	meta := LogsRBAC()
	s.Require().NoError(meta.Validate())
	s.Require().NotNil(meta.Bounded)
	s.NotEmpty(meta.Bounded.Requirements)
}

func (s *RBACSuite) TestIstioConfigWriteRBAC() {
	meta := IstioConfigWriteRBAC()
	s.Require().NoError(meta.Validate())
	s.Require().NotNil(meta.Bounded)
	s.GreaterOrEqual(len(meta.Bounded.Requirements), len(istioNetworkingResources))
	for _, req := range meta.Bounded.Requirements {
		s.Contains(req.Verbs, "create")
		s.Contains(req.Verbs, "patch")
		s.Contains(req.Verbs, "delete")
		s.Require().NotNil(req.Namespace)
		s.Equal("namespace", req.Namespace.Argument)
		s.Require().NotNil(req.ResourceName)
		s.Equal("object", req.ResourceName.Argument)
	}
}

func (s *RBACSuite) TestIstioConfigReadRBAC() {
	meta := IstioConfigReadRBAC()
	s.Require().NoError(meta.Validate())
	s.Require().NotNil(meta.Bounded)
	for _, req := range meta.Bounded.Requirements {
		s.Contains(req.Verbs, "get")
		s.Contains(req.Verbs, "list")
		s.Require().NotNil(req.Namespace)
		s.True(req.Namespace.AllNamespaces)
	}
}

func (s *RBACSuite) TestResourcesRBAC() {
	s.Run("includes openshift resources on OpenShift", func() {
		meta := ResourcesRBAC(s.T().Context(), &stubInspector{isOpenShift: true})
		s.Require().NoError(meta.Validate())
		s.Require().NotNil(meta.Bounded)
		var hasDC, hasRoute bool
		for _, req := range meta.Bounded.Requirements {
			if req.Target.Resource == nil {
				continue
			}
			if req.Target.Resource.APIGroup == "apps.openshift.io" && req.Target.Resource.Resource == "deploymentconfigs" {
				hasDC = true
			}
			if req.Target.Resource.APIGroup == "route.openshift.io" && req.Target.Resource.Resource == "routes" {
				hasRoute = true
			}
		}
		s.True(hasDC)
		s.True(hasRoute)
	})

	s.Run("omits openshift resources off OpenShift", func() {
		meta := ResourcesRBAC(s.T().Context(), &stubInspector{isOpenShift: false})
		s.Require().NoError(meta.Validate())
		for _, req := range meta.Bounded.Requirements {
			if req.Target.Resource == nil {
				continue
			}
			s.NotEqual("apps.openshift.io", req.Target.Resource.APIGroup)
			s.NotEqual("route.openshift.io", req.Target.Resource.APIGroup)
		}
	})

	s.Run("omits openshift resources without provider", func() {
		meta := ResourcesRBAC(s.T().Context(), nil)
		s.Require().NoError(meta.Validate())
		for _, req := range meta.Bounded.Requirements {
			if req.Target.Resource == nil {
				continue
			}
			s.NotEqual("apps.openshift.io", req.Target.Resource.APIGroup)
		}
	})
}

func (s *RBACSuite) TestNamespaceAccessRBAC() {
	meta := NamespaceAccessRBAC("namespace")
	s.Require().NoError(meta.Validate())
	s.Require().NotNil(meta.Bounded)
	s.Require().Len(meta.Bounded.Requirements, 2)
	s.Equal("namespaces", meta.Bounded.Requirements[1].Target.Resource.Resource)
	s.Equal("namespace", meta.Bounded.Requirements[1].ResourceName.Argument)
}

func (s *RBACSuite) TestGraphRBAC() {
	ocp := GraphRBAC(s.T().Context(), &stubInspector{isOpenShift: true})
	s.Require().NoError(ocp.Validate())
	s.NotNil(ocp.Bounded)

	plain := GraphRBAC(s.T().Context(), &stubInspector{isOpenShift: false})
	s.Require().NoError(plain.Validate())
	s.Less(len(plain.Bounded.Requirements), len(ocp.Bounded.Requirements))
}

func (s *RBACSuite) TestUnboundedHelpers() {
	s.Require().NoError(MeshStatusRBAC().Validate())
	s.NotNil(MeshStatusRBAC().Unbounded)
	s.Require().NoError(ListClustersRBAC().Validate())
	s.NotNil(ListClustersRBAC().Unbounded)
}

func (s *RBACSuite) TestInitToolsCarryRBAC() {
	ocp := &stubInspector{isOpenShift: true}
	plain := &stubInspector{isOpenShift: false}

	s.Run("bounded tools", func() {
		s.NotNil(InitGetLogs()[0].RBAC.Bounded)
		s.NotNil(InitManageIstioConfig()[0].RBAC.Bounded)
		s.NotNil(InitManageIstioConfigRead()[0].RBAC.Bounded)
		s.NotNil(InitGetMetrics()[0].RBAC.Bounded)
		s.NotNil(InitListTraces()[0].RBAC.Bounded)
		s.NotNil(InitGetPodPerformance()[0].RBAC.Bounded)
		s.NotNil(InitGetTraceDetails()[0].RBAC.Bounded)
		s.NotNil(InitListOrGetResources(s.T().Context(), plain)[0].RBAC.Bounded)
		s.NotNil(InitGetMeshTrafficGraph(s.T().Context(), plain)[0].RBAC.Bounded)
		s.NotNil(InitListOrGetResources(s.T().Context(), ocp)[0].RBAC.Bounded)
		s.NotNil(InitGetMeshTrafficGraph(s.T().Context(), ocp)[0].RBAC.Bounded)
	})

	s.Run("unbounded tools", func() {
		s.NotNil(InitGetMeshStatus()[0].RBAC.Unbounded)
		s.NotNil(InitListMeshClusters()[0].RBAC.Unbounded)
	})
}

func (s *RBACSuite) TestMergeRBACBounded() {
	merged := MergeRBACBounded(LogsRBAC(), NamespaceAccessRBAC("namespace"))
	s.Require().NoError(merged.Validate())
	s.Greater(len(merged.Bounded.Requirements), len(LogsRBAC().Bounded.Requirements))
}

func TestRBAC(t *testing.T) {
	suite.Run(t, new(RBACSuite))
}
