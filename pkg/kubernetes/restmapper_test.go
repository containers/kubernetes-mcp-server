package kubernetes

import (
	"context"
	"net/http"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const restMapperTestGroupVersion = "widgets.example.com/v1"

var (
	widgetGK = schema.GroupKind{Group: "widgets.example.com", Kind: "Widget"}
	gadgetGK = schema.GroupKind{Group: "widgets.example.com", Kind: "Gadget"}
	gizmoGK  = schema.GroupKind{Group: "widgets.example.com", Kind: "Gizmo"}
)

type RESTMapperTestSuite struct {
	suite.Suite
}

// newManager starts a mock cluster that serves widgets.example.com/v1 widgets and
// returns a kubeconfig manager for it together with the cluster's discovery handler.
func (s *RESTMapperTestSuite) newManager() (*Manager, *test.DiscoveryClientHandler) {
	mockServer := test.NewMockServer()
	s.T().Cleanup(mockServer.Close)
	discoveryHandler := test.NewDiscoveryClientHandler(metav1.APIResourceList{
		GroupVersion: restMapperTestGroupVersion,
		APIResources: []metav1.APIResource{restMapperTestResource("widgets", "Widget")},
	})
	mockServer.Handle(discoveryHandler)
	mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/apis/widgets.example.com/v1/namespaces/default/gadgets" {
			_, _ = w.Write([]byte(`{"apiVersion":"widgets.example.com/v1","kind":"GadgetList","metadata":{},"items":[]}`))
		}
	}))
	cfg := config.BaseDefault()
	cfg.KubeConfig.SetForTest(mockServer.KubeconfigFile(s.T()))
	manager, err := NewKubeconfigManager(s.T().Context(), cfg, "")
	s.Require().NoError(err, "Expected no error creating manager")
	s.T().Cleanup(manager.Close)
	return manager, discoveryHandler
}

// staleClient returns a client whose REST mapper is loaded before a kind is added to
// widgets.example.com/v1.
func (s *RESTMapperTestSuite) staleClient(ctx context.Context, kind string) *Kubernetes {
	manager, discoveryHandler := s.newManager()
	k, err := manager.Derived(ctx)
	s.Require().NoError(err, "Expected no error deriving client")
	_, err = k.RESTMapper().RESTMapping(widgetGK, "v1")
	s.Require().NoError(err, "Expected the REST mapper to resolve resources discovered when it is loaded")
	addKind(discoveryHandler, kind)
	return k
}

func addKind(discoveryHandler *test.DiscoveryClientHandler, kind string) {
	resources := map[string]string{"Gadget": "gadgets", "Gizmo": "gizmos"}
	discoveryHandler.AddAPIResource(restMapperTestGroupVersion, restMapperTestResource(resources[kind], kind))
}

func restMapperTestResource(name, kind string) metav1.APIResource {
	return metav1.APIResource{Name: name, Kind: kind, Namespaced: true, Verbs: metav1.Verbs{"get", "list"}}
}

func (s *RESTMapperTestSuite) TestResolvesKindAddedToExistingGroupAfterLoad() {
	gadgets := schema.GroupVersionResource{Group: "widgets.example.com", Version: "v1", Resource: "gadgets"}
	lookups := map[string]func(meta.RESTMapper) error{
		"KindFor": func(m meta.RESTMapper) error {
			_, err := m.KindFor(gadgets)
			return err
		},
		"KindsFor": func(m meta.RESTMapper) error {
			_, err := m.KindsFor(gadgets)
			return err
		},
		"ResourceFor": func(m meta.RESTMapper) error {
			_, err := m.ResourceFor(gadgets)
			return err
		},
		"ResourcesFor": func(m meta.RESTMapper) error {
			_, err := m.ResourcesFor(gadgets)
			return err
		},
		"RESTMapping": func(m meta.RESTMapper) error {
			_, err := m.RESTMapping(gadgetGK, "v1")
			return err
		},
		"RESTMappings": func(m meta.RESTMapper) error {
			_, err := m.RESTMappings(gadgetGK, "v1")
			return err
		},
	}
	for name, lookup := range lookups {
		s.Run(name+" resets the REST mapper on NoMatch", func() {
			k := s.staleClient(s.T().Context(), "Gadget")
			s.NoError(lookup(k.RESTMapper()), "Expected the REST mapper to be reset on NoMatch")
		})
	}
	s.Run("access control resolves the kind for a request", func() {
		k := s.staleClient(s.T().Context(), "Gadget")
		_, err := k.DynamicClient().Resource(gadgets).Namespace("default").List(s.T().Context(), metav1.ListOptions{})
		s.NoError(err, "Expected access control to find the kind through the REST mapper")
	})
}

func (s *RESTMapperTestSuite) TestResetsAtMostOncePerInterval() {
	manager, discoveryHandler := s.newManager()
	k, err := manager.Derived(s.T().Context())
	s.Require().NoError(err, "Expected no error deriving client")
	_, err = k.RESTMapper().RESTMapping(widgetGK, "v1")
	s.Require().NoError(err, "Expected the REST mapper to resolve resources discovered when it is loaded")
	addKind(discoveryHandler, "Gadget")
	_, err = k.RESTMapper().RESTMapping(gadgetGK, "v1")
	s.Require().NoError(err, "Expected the first NoMatch to reset the REST mapper")
	addKind(discoveryHandler, "Gizmo")

	s.Run("keeps the REST mapper on a NoMatch within the interval", func() {
		_, err := k.RESTMapper().RESTMapping(gizmoGK, "v1")
		s.True(meta.IsNoMatchError(err), "Expected a NoMatch while the reset is rate limited, got %v", err)
	})
	s.Run("rejects the request in access control within the interval", func() {
		gizmos := schema.GroupVersionResource{Group: "widgets.example.com", Version: "v1", Resource: "gizmos"}
		_, err := k.DynamicClient().Resource(gizmos).Namespace("default").List(s.T().Context(), metav1.ListOptions{})
		var ve *api.ValidationError
		s.Require().ErrorAs(err, &ve, "Expected a validation error while the reset is rate limited")
		s.Equal(api.ErrorCodeResourceNotFound, ve.Code)
	})
	s.Run("explicit invalidation is not rate limited", func() {
		manager.Invalidate()
		_, err := k.RESTMapper().RESTMapping(gizmoGK, "v1")
		s.NoError(err, "Expected the invalidated REST mapper to resolve the kind")
	})
}

func (s *RESTMapperTestSuite) TestPerRequestOAuthClientDoesNotReset() {
	ctx := context.WithValue(s.T().Context(), OAuthAuthorizationHeader, "Bearer test-token")
	k := s.staleClient(ctx, "Gadget")
	_, err := k.RESTMapper().RESTMapping(gadgetGK, "v1")
	s.True(meta.IsNoMatchError(err), "Expected the per-request REST mapper to be kept, got %v", err)
}

func TestRESTMapper(t *testing.T) {
	suite.Run(t, new(RESTMapperTestSuite))
}
