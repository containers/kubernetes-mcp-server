package mcp

import (
	"net/http"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

// PodsDeleteCleanupSuite covers pods_delete on a Pod created by pods_run, with
// API answers that envtest's RBAC setup cannot easily produce.
type PodsDeleteCleanupSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
	mu         sync.Mutex
	calls      []string
}

func (s *PodsDeleteCleanupSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.calls = nil
	s.mockServer = test.NewMockServer()
	discovery := test.NewDiscoveryClientHandler()
	discovery.APIResourceLists[0].APIResources = append(discovery.APIResourceLists[0].APIResources,
		metav1.APIResource{Name: "services", Kind: "Service", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "delete"}})
	s.mockServer.Handle(discovery)
	s.Cfg.KubeConfig.SetForTest(s.mockServer.KubeconfigFile(s.T()))
}

func (s *PodsDeleteCleanupSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *PodsDeleteCleanupSuite) record(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
}

func (s *PodsDeleteCleanupSuite) handle(podDelete, serviceDelete int) {
	managed := map[string]string{
		"app.kubernetes.io/managed-by": "kubernetes-mcp-server",
		"app.kubernetes.io/name":       "web",
	}
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/namespaces/default/pods/web":
			s.record(r)
			if r.Method == http.MethodDelete && podDelete != http.StatusOK {
				status := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "web", nil).ErrStatus
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(podDelete)
				test.WriteObject(w, &status)
				return
			}
			test.WriteObject(w, &v1.Pod{
				TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", Labels: managed},
			})
		case "/api/v1/namespaces/default/services":
			s.record(r)
			test.WriteObject(w, &v1.ServiceList{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceList"},
				Items:    []v1.Service{{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", Labels: managed}}},
			})
		case "/api/v1/namespaces/default/services/web":
			s.record(r)
			if serviceDelete != http.StatusOK {
				status := apierrors.NewForbidden(schema.GroupResource{Resource: "services"}, "web", nil).ErrStatus
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(serviceDelete)
				test.WriteObject(w, &status)
				return
			}
			test.WriteObject(w, &metav1.Status{Status: metav1.StatusSuccess})
		}
	}))
}

func (s *PodsDeleteCleanupSuite) TestServiceDeleteRefused() {
	s.handle(http.StatusOK, http.StatusForbidden)
	s.InitMcpClient()
	result, err := s.CallTool("pods_delete", map[string]interface{}{"name": "web", "namespace": "default"})
	s.Require().NoError(err)
	s.Run("returns an error", func() {
		s.True(result.IsError, "a refused Service delete must not be reported as success")
	})
	s.Run("says the Pod was deleted and names the Service", func() {
		text := result.Content[0].(*mcp.TextContent).Text
		s.Contains(text, "pod default/web was deleted, but its managed resources were not all deleted")
		s.Contains(text, "service web")
	})
}

func (s *PodsDeleteCleanupSuite) TestPodDeleteRefused() {
	s.handle(http.StatusForbidden, http.StatusOK)
	s.InitMcpClient()
	result, err := s.CallTool("pods_delete", map[string]interface{}{"name": "web", "namespace": "default"})
	s.Require().NoError(err)
	s.Run("returns an error", func() {
		s.True(result.IsError)
	})
	s.Run("leaves the managed Service alone", func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.NotContains(s.calls, "DELETE /api/v1/namespaces/default/services/web")
	})
}

func (s *PodsDeleteCleanupSuite) TestBothDeleted() {
	s.handle(http.StatusOK, http.StatusOK)
	s.InitMcpClient()
	result, err := s.CallTool("pods_delete", map[string]interface{}{"name": "web", "namespace": "default"})
	s.Require().NoError(err)
	s.False(result.IsError, "call tool failed: %v", result.Content)
	s.Equal("Pod deleted successfully", result.Content[0].(*mcp.TextContent).Text)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Contains(s.calls, "DELETE /api/v1/namespaces/default/services/web")
}

func TestPodsDeleteCleanup(t *testing.T) {
	suite.Run(t, new(PodsDeleteCleanupSuite))
}
