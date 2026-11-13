package kubernetes

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type ResourcesListAccessReviewSuite struct {
	suite.Suite
	mockServer *test.MockServer
	core       *Core
	mu         sync.Mutex
	reviews    int
	listPaths  []string
}

func (s *ResourcesListAccessReviewSuite) SetupTest() {
	s.reviews, s.listPaths = 0, nil
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler(metav1.APIResourceList{
		GroupVersion: "authorization.k8s.io/v1",
		APIResources: []metav1.APIResource{{Name: "selfsubjectaccessreviews", Kind: "SelfSubjectAccessReview", Verbs: metav1.Verbs{"create"}}},
	}))
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews":
			s.mu.Lock()
			s.reviews++
			s.mu.Unlock()
			test.WriteObject(w, &authorizationv1.SelfSubjectAccessReview{
				TypeMeta: metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SelfSubjectAccessReview"},
				Status:   authorizationv1.SubjectAccessReviewStatus{Allowed: true},
			})
		case strings.HasSuffix(r.URL.Path, "/pods") && r.Method == http.MethodGet:
			s.mu.Lock()
			s.listPaths = append(s.listPaths, r.URL.Path)
			s.mu.Unlock()
			test.WriteObject(w, &unstructured.UnstructuredList{Object: map[string]interface{}{"apiVersion": "v1", "kind": "PodList"}})
		}
	}))
	k, err := NewKubernetes(
		s.T().Context(),
		config.BaseDefault(),
		clientcmd.NewDefaultClientConfig(*s.mockServer.Kubeconfig(), &clientcmd.ConfigOverrides{}),
		s.mockServer.Config(),
	)
	s.Require().NoError(err)
	s.T().Cleanup(k.close)
	s.core = NewCore(k)
}

func (s *ResourcesListAccessReviewSuite) TearDownTest() {
	s.mockServer.Close()
}

func (s *ResourcesListAccessReviewSuite) TestNamespacedListMakesNoAccessReview() {
	_, err := s.core.ResourcesList(context.Background(), &schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, "team-a", api.ListOptions{})
	s.Require().NoError(err)
	s.Equal(0, s.reviews, "a list in a given namespace needs no SelfSubjectAccessReview")
	s.Equal([]string{"/api/v1/namespaces/team-a/pods"}, s.listPaths)
}

func (s *ResourcesListAccessReviewSuite) TestAllNamespacesListMakesOneAccessReview() {
	_, err := s.core.ResourcesList(context.Background(), &schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, "", api.ListOptions{})
	s.Require().NoError(err)
	s.Equal(1, s.reviews)
	s.Equal([]string{"/api/v1/pods"}, s.listPaths)
}

func TestResourcesListAccessReview(t *testing.T) {
	suite.Run(t, new(ResourcesListAccessReviewSuite))
}
