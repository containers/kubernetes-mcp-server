package kubernetes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/restmapper"
)

type mockRoundTripper struct {
	mu        sync.Mutex
	called    *bool
	onRequest func(w http.ResponseWriter, r *http.Request)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	*m.called = true
	rec := httptest.NewRecorder()
	m.onRequest(rec, req)
	return rec.Result(), nil
}

type mockDeniedResourcesProvider struct {
	resources []api.GroupVersionKind
}

func (m *mockDeniedResourcesProvider) GetDeniedResources() []api.GroupVersionKind {
	return m.resources
}

type AccessControlRoundTripperTestSuite struct {
	suite.Suite
	mockServer *test.MockServer
	restMapper *restmapper.DeferredDiscoveryRESTMapper
}

func (s *AccessControlRoundTripperTestSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())

	clientSet, err := kubernetes.NewForConfig(s.mockServer.Config())
	s.Require().NoError(err, "Expected no error creating clientset")

	s.restMapper = restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(clientSet.Discovery()))
}

func (s *AccessControlRoundTripperTestSuite) TearDownTest() {
	s.mockServer.Close()
}

func (s *AccessControlRoundTripperTestSuite) TestRoundTripForNonAPIResources() {
	delegateCalled := false
	mockDelegate := &mockRoundTripper{
		called: &delegateCalled,
		onRequest: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	rt := &AccessControlRoundTripper{
		delegate:                mockDelegate,
		deniedResourcesProvider: nil,
		restMapperProvider:      func() meta.RESTMapper { return s.restMapper },
	}

	testCases := []string{"healthz", "readyz", "livez", "metrics", "version"}
	for _, testCase := range testCases {
		s.Run("/"+testCase+" check endpoint bypasses access control", func() {
			delegateCalled = false
			resp, err := rt.RoundTrip(httptest.NewRequest("GET", "/"+testCase, nil))
			s.NoError(err)
			s.NotNil(resp)
			s.Equal(http.StatusOK, resp.StatusCode)
			s.Truef(delegateCalled, "Expected delegate to be called for /%s", testCase)
		})
	}
}

func (s *AccessControlRoundTripperTestSuite) TestRoundTripWithNilRestMapper() {
	// This test covers the nil restMapper branch (issue #688 fix).
	// When restMapperProvider returns nil, requests should fail with an error
	// to prevent bypassing access control.

	delegateCalled := false
	mockDelegate := &mockRoundTripper{
		called: &delegateCalled,
		onRequest: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	rt := &AccessControlRoundTripper{
		delegate:                mockDelegate,
		deniedResourcesProvider: nil,
		restMapperProvider:      func() meta.RESTMapper { return nil }, // nil restMapper
	}

	s.Run("resource API call fails when restMapper is nil", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/api/v1/namespaces/default/pods", nil)
		resp, err := rt.RoundTrip(req)
		s.Error(err, "Expected error when restMapper is nil")
		s.Nil(resp, "Expected no response when restMapper is nil")
		s.Contains(err.Error(), "restMapper not initialized")
		s.False(delegateCalled, "Expected delegate not to be called when restMapper is nil")
	})

	s.Run("non-namespaced resource API call fails when restMapper is nil", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/api/v1/nodes", nil)
		resp, err := rt.RoundTrip(req)
		s.Error(err)
		s.Nil(resp)
		s.False(delegateCalled, "Expected delegate not to be called for non-namespaced resource")
	})

	s.Run("apps group resource API call fails when restMapper is nil", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/apis/apps/v1/namespaces/default/deployments", nil)
		resp, err := rt.RoundTrip(req)
		s.Error(err)
		s.Nil(resp)
		s.False(delegateCalled, "Expected delegate not to be called for apps group resource")
	})
}

func (s *AccessControlRoundTripperTestSuite) TestRoundTripForDiscoveryRequests() {
	delegateCalled := false
	mockDelegate := &mockRoundTripper{
		called: &delegateCalled,
		onRequest: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	rt := &AccessControlRoundTripper{
		delegate:                mockDelegate,
		deniedResourcesProvider: nil,
		restMapperProvider:      func() meta.RESTMapper { return s.restMapper },
	}

	testCases := []string{"/api", "/apis", "/api/v1", "/api/v1/", "/apis/apps", "/apis/apps/v1", "/apis/batch/v1"}
	for _, testCase := range testCases {
		s.Run("API Discovery endpoint "+testCase+" bypasses access control", func() {
			delegateCalled = false
			resp, err := rt.RoundTrip(httptest.NewRequest("GET", testCase, nil))
			s.NoError(err)
			s.NotNil(resp)
			s.Equal(http.StatusOK, resp.StatusCode)
			s.True(delegateCalled, "Expected delegate to be called for /api")
		})
	}
}

func (s *AccessControlRoundTripperTestSuite) TestRoundTripForPrefixedDiscoveryRequests() {
	delegateCalled := false
	mockDelegate := &mockRoundTripper{
		called: &delegateCalled,
		onRequest: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	rt := &AccessControlRoundTripper{
		delegate:                mockDelegate,
		deniedResourcesProvider: nil,
		restMapperProvider: func() meta.RESTMapper {
			s.Fail("restMapper should not be consulted for discovery requests behind a path prefix")
			return nil
		},
		apiPathPrefix: "/api/v1/kube/clusters/test-cluster",
	}

	testCases := []string{
		"/api/v1/kube/clusters/test-cluster/api",
		"/api/v1/kube/clusters/test-cluster/apis",
		"/api/v1/kube/clusters/test-cluster/api/v1",
		"/api/v1/kube/clusters/test-cluster/apis/apps/v1",
		"/api/v1/kube/clusters/test-cluster/openapi/v2",
		"/api/v1/kube/clusters/test-cluster/version",
	}

	for _, testCase := range testCases {
		s.Run("Prefixed discovery endpoint "+testCase+" bypasses access control", func() {
			delegateCalled = false
			resp, err := rt.RoundTrip(httptest.NewRequest("GET", testCase, nil))
			s.NoError(err)
			s.NotNil(resp)
			s.Equal(http.StatusOK, resp.StatusCode)
			s.True(delegateCalled, "Expected delegate to be called for prefixed discovery request")
		})
	}
}

func (s *AccessControlRoundTripperTestSuite) TestRoundTripForAllowedAPIResources() {
	delegateCalled := false
	mockDelegate := &mockRoundTripper{
		called: &delegateCalled,
		onRequest: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	rt := &AccessControlRoundTripper{
		delegate:                mockDelegate,
		deniedResourcesProvider: nil, // nil config allows all resources
		restMapperProvider:      func() meta.RESTMapper { return s.restMapper },
	}

	s.Run("List all pods is allowed", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/api/v1/pods", nil)
		resp, err := rt.RoundTrip(req)
		s.NoError(err)
		s.NotNil(resp)
		s.Equal(http.StatusOK, resp.StatusCode)
		s.True(delegateCalled, "Expected delegate to be called for listing pods")
	})

	s.Run("List pods in namespace is allowed", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/api/v1/namespaces/default/pods", nil)
		resp, err := rt.RoundTrip(req)
		s.NoError(err)
		s.NotNil(resp)
		s.True(delegateCalled, "Expected delegate to be called for namespaced pods list")
	})

	s.Run("Get specific pod is allowed", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/api/v1/namespaces/default/pods/my-pod", nil)
		resp, err := rt.RoundTrip(req)
		s.NoError(err)
		s.NotNil(resp)
		s.True(delegateCalled, "Expected delegate to be called for getting specific pod")
	})

	s.Run("Resource path with trailing slash is allowed", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/api/v1/pods/", nil)
		resp, err := rt.RoundTrip(req)
		s.NoError(err)
		s.NotNil(resp)
		s.True(delegateCalled, "Expected delegate to be called for path with trailing slash")
	})

	s.Run("List Deployments is allowed", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/apis/apps/v1/deployments", nil)
		resp, err := rt.RoundTrip(req)
		s.NoError(err)
		s.NotNil(resp)
		s.True(delegateCalled, "Expected delegate to be called for listing deployments")
	})

	s.Run("List Deployments in namespace is allowed", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/apis/apps/v1/namespaces/default/deployments", nil)
		resp, err := rt.RoundTrip(req)
		s.NoError(err)
		s.NotNil(resp)
		s.True(delegateCalled, "Expected delegate to be called for namespaced deployments list")
	})

	s.Run("List pods in namespace is allowed when the API server has a path prefix", func() {
		delegateCalled = false
		originalAPIPathPrefix := rt.apiPathPrefix
		defer func() {
			rt.apiPathPrefix = originalAPIPathPrefix
		}()
		rt.apiPathPrefix = "/api/v1/kube/clusters/test-cluster"
		req := httptest.NewRequest("GET", "/api/v1/kube/clusters/test-cluster/api/v1/namespaces/default/pods", nil)
		resp, err := rt.RoundTrip(req)
		s.NoError(err)
		s.NotNil(resp)
		s.True(delegateCalled, "Expected delegate to be called for namespaced pods list behind a path prefix")
	})
}

func (s *AccessControlRoundTripperTestSuite) TestValidationDisabledBypassesValidators() {
	s.mockServer.Handle(newOpenAPISchemaHandler())
	clientSet, err := kubernetes.NewForConfig(s.mockServer.Config())
	s.Require().NoError(err)

	delegateCalled := false
	mockDelegate := &mockRoundTripper{
		called: &delegateCalled,
		onRequest: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	s.Run("validation disabled allows request with unknown fields", func() {
		delegateCalled = false
		rt := NewAccessControlRoundTripper(s.T().Context(), AccessControlRoundTripperConfig{
			Delegate:           mockDelegate,
			RestMapperProvider: func() meta.RESTMapper { return s.restMapper },
			ValidationEnabled:  false,
			DiscoveryProvider:  func() discovery.DiscoveryInterface { return clientSet.Discovery() },
			AuthClientProvider: func() authv1client.AuthorizationV1Interface { return clientSet.AuthorizationV1() },
		})
		req := httptest.NewRequest("POST", "/api/v1/namespaces/default/pods", strings.NewReader(`{"apiVersion":"v1","kind":"Pod","specTypo":"bad"}`))
		resp, err := rt.RoundTrip(req)
		s.NoError(err)
		s.NotNil(resp)
		s.True(delegateCalled, "Expected delegate to be called when validation is disabled")
	})

	s.Run("validation enabled rejects request with unknown fields", func() {
		delegateCalled = false
		rt := NewAccessControlRoundTripper(s.T().Context(), AccessControlRoundTripperConfig{
			Delegate:           mockDelegate,
			RestMapperProvider: func() meta.RESTMapper { return s.restMapper },
			ValidationEnabled:  true,
			DiscoveryProvider:  func() discovery.DiscoveryInterface { return clientSet.Discovery() },
			AuthClientProvider: func() authv1client.AuthorizationV1Interface { return clientSet.AuthorizationV1() },
		})
		req := httptest.NewRequest("POST", "/api/v1/namespaces/default/pods", strings.NewReader(`{"apiVersion":"v1","kind":"Pod","specTypo":"bad"}`))
		resp, err := rt.RoundTrip(req)
		s.Error(err, "Expected validation error for unknown field")
		s.Nil(resp)
		s.False(delegateCalled, "Expected delegate not to be called when validation rejects request")
		var ve *api.ValidationError
		s.ErrorAs(err, &ve)
		s.Equal(api.ErrorCodeInvalidField, ve.Code)
	})
}

func (s *AccessControlRoundTripperTestSuite) TestRoundTripForDeniedAPIResources() {
	delegateCalled := false
	mockDelegate := &mockRoundTripper{
		called: &delegateCalled,
		onRequest: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	rt := &AccessControlRoundTripper{
		delegate:                mockDelegate,
		deniedResourcesProvider: config.Default(),
		restMapperProvider:      func() meta.RESTMapper { return s.restMapper },
	}

	s.Run("Specific resource kind is denied", func() {
		s.Require().NoError(toml.Unmarshal([]byte(`
			denied_resources = [ { version = "v1", kind = "Pod" } ]
		`), rt.deniedResourcesProvider), "Expected to parse denied resources config")

		s.Run("List pods is denied", func() {
			delegateCalled = false
			req := httptest.NewRequest("GET", "/api/v1/pods", nil)
			resp, err := rt.RoundTrip(req)
			s.Error(err)
			s.Nil(resp)
			s.False(delegateCalled, "Expected delegate not to be called for denied resource")
			s.Contains(err.Error(), "resource not allowed")
			s.Contains(err.Error(), "Pod")
		})

		s.Run("Get specific pod is denied", func() {
			delegateCalled = false
			req := httptest.NewRequest("GET", "/api/v1/namespaces/default/pods/my-pod", nil)
			resp, err := rt.RoundTrip(req)
			s.Error(err)
			s.Nil(resp)
			s.False(delegateCalled)
			s.Contains(err.Error(), "resource not allowed")
		})

		s.Run("List pods behind an API path prefix is denied", func() {
			delegateCalled = false
			originalAPIPathPrefix := rt.apiPathPrefix
			defer func() {
				rt.apiPathPrefix = originalAPIPathPrefix
			}()
			rt.apiPathPrefix = "/api/v1/kube/clusters/test-cluster"

			req := httptest.NewRequest("GET", "/api/v1/kube/clusters/test-cluster/api/v1/namespaces/default/pods", nil)
			resp, err := rt.RoundTrip(req)
			s.Error(err)
			s.Nil(resp)
			s.False(delegateCalled, "Expected delegate not to be called for denied resource behind a path prefix")
			s.Contains(err.Error(), "resource not allowed")
			s.Contains(err.Error(), "Pod")
		})
	})

	s.Run("Entire group/version is denied", func() {
		s.Require().NoError(toml.Unmarshal([]byte(`
			denied_resources = [ { version = "v1", kind = "" } ]
		`), rt.deniedResourcesProvider), "Expected to v1 denied resources config")

		s.Run("Pods in core/v1 are denied", func() {
			delegateCalled = false
			req := httptest.NewRequest("GET", "/api/v1/pods", nil)
			resp, err := rt.RoundTrip(req)
			s.Error(err)
			s.Nil(resp)
			s.False(delegateCalled)
		})

	})

	s.Run("RESTMapper error for unknown resource", func() {
		rt.deniedResourcesProvider = nil
		delegateCalled = false
		req := httptest.NewRequest("GET", "/api/v1/unknownresources", nil)
		resp, err := rt.RoundTrip(req)
		s.Error(err)
		s.Nil(resp)
		s.False(delegateCalled, "Expected delegate not to be called when RESTMapper fails")
		s.Contains(err.Error(), "RESOURCE_NOT_FOUND")
		s.Contains(err.Error(), "does not exist in the cluster")
	})
}

type StripAPIPathPrefixTestSuite struct {
	suite.Suite
}

func (s *StripAPIPathPrefixTestSuite) TestStripAPIPathPrefix() {
	s.Run("returns original path when prefix is empty", func() {
		s.Equal("/api/v1/pods", stripAPIPathPrefix("/api/v1/pods", ""))
	})

	s.Run("returns original path when prefix is root", func() {
		s.Equal("/api/v1/pods", stripAPIPathPrefix("/api/v1/pods", "/"))
	})

	s.Run("returns slash when path matches prefix exactly", func() {
		s.Equal("/", stripAPIPathPrefix("/api/v1/kube/clusters/test-cluster", "/api/v1/kube/clusters/test-cluster"))
	})

	s.Run("removes the configured API path prefix", func() {
		s.Equal(
			"/api/v1/namespaces/default/pods",
			stripAPIPathPrefix(
				"/api/v1/kube/clusters/test-cluster/api/v1/namespaces/default/pods",
				"/api/v1/kube/clusters/test-cluster",
			),
		)
	})

	s.Run("removes prefix with trailing slash", func() {
		s.Equal(
			"/api/v1/pods",
			stripAPIPathPrefix(
				"/api/v1/kube/clusters/test-cluster/api/v1/pods",
				"/api/v1/kube/clusters/test-cluster/",
			),
		)
	})

	s.Run("does not trim partial prefix matches", func() {
		s.Equal(
			"/api/v1/kube/clusters/test-cluster/api/v1/pods",
			stripAPIPathPrefix(
				"/api/v1/kube/clusters/test-cluster/api/v1/pods",
				"/api/v1/kube/clusters/test",
			),
		)
	})
}

func TestAccessControlRoundTripper(t *testing.T) {
	suite.Run(t, new(AccessControlRoundTripperTestSuite))
}

func TestStripAPIPathPrefix(t *testing.T) {
	suite.Run(t, new(StripAPIPathPrefixTestSuite))
}

// SubresourceOnlyAPIGroupSuite tests the AccessControlRoundTripper behavior
// with API groups that only expose subresource entries (all with empty Kind).
// This reproduces the problem where subresources.kubevirt.io/v1 is rejected
// with RESOURCE_NOT_FOUND because the REST mapper cannot build GVR→GVK
// mappings from entries that have no Kind set.
type SubresourceOnlyAPIGroupSuite struct {
	suite.Suite
	mockServer       *test.MockServer
	discoveryHandler *test.DiscoveryClientHandler
	restMapper       *restmapper.DeferredDiscoveryRESTMapper
	discoveryClient  discovery.DiscoveryInterface
}

func (s *SubresourceOnlyAPIGroupSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
	// Register subresources.kubevirt.io/v1 with subresource-only entries.
	// All entries have zero-value Kind ("") and nil Verbs — this matches
	// what KubeVirt actually advertises for its subresource API group.
	// The REST mapper will see these during discovery but cannot build
	// GVR→GVK mappings, so KindFor() returns NoMatchError.
	s.discoveryHandler = test.NewDiscoveryClientHandler(metav1.APIResourceList{
		GroupVersion: "subresources.kubevirt.io/v1",
		APIResources: []metav1.APIResource{
			{Name: "virtualmachineinstances/pause", Namespaced: true},
			{Name: "virtualmachineinstances/unpause", Namespaced: true},
			{Name: "virtualmachineinstances/guestosinfo", Namespaced: true},
			{Name: "virtualmachineinstances/userlist", Namespaced: true},
			{Name: "virtualmachineinstances/filesystemlist", Namespaced: true},
			{Name: "virtualmachines/start", Namespaced: true},
			{Name: "virtualmachines/stop", Namespaced: true},
			{Name: "virtualmachines/restart", Namespaced: true},
		},
	})
	s.mockServer.Handle(s.discoveryHandler)

	clientSet, err := kubernetes.NewForConfig(s.mockServer.Config())
	s.Require().NoError(err, "Expected no error creating clientset")

	s.discoveryClient = clientSet.Discovery()
	s.restMapper = restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(s.discoveryClient))
}

func (s *SubresourceOnlyAPIGroupSuite) TearDownTest() {
	s.mockServer.Close()
}

func (s *SubresourceOnlyAPIGroupSuite) TestRoundTripForSubresourceOnlyAPIGroups() {
	delegateCalled := false
	mockDelegate := &mockRoundTripper{
		called: &delegateCalled,
		onRequest: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	rt := NewAccessControlRoundTripper(s.T().Context(), AccessControlRoundTripperConfig{
		Delegate:             mockDelegate,
		RestMapperProvider:   func() meta.RESTMapper { return s.restMapper },
		RawDiscoveryProvider: func() discovery.DiscoveryInterface { return s.discoveryClient },
	})

	s.Run("subresource-only API group passes through for pause action", func() {
		delegateCalled = false
		req := httptest.NewRequest("PUT", "/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/pause", nil)
		resp, err := rt.RoundTrip(req)
		s.NoError(err, "Expected request to subresources.kubevirt.io to pass through")
		s.NotNil(resp)
		s.True(delegateCalled, "Expected delegate to be called for subresource-only API group")
	})

	s.Run("subresource-only API group passes through for guest agent query", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/guestosinfo", nil)
		resp, err := rt.RoundTrip(req)
		s.NoError(err, "Expected request to subresources.kubevirt.io guest agent subresource to pass through")
		s.NotNil(resp)
		s.True(delegateCalled, "Expected delegate to be called for guest agent subresource")
	})

	s.Run("non-existent API group is still rejected", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/apis/totally.fake.io/v1/namespaces/default/things", nil)
		resp, err := rt.RoundTrip(req)
		s.Error(err, "Expected error for non-existent API group")
		s.Nil(resp)
		s.False(delegateCalled, "Expected delegate not to be called for non-existent API group")
		var ve *api.ValidationError
		s.ErrorAs(err, &ve)
		s.Equal(api.ErrorCodeResourceNotFound, ve.Code)
	})

	s.Run("non-existent resource in discoverable group is still rejected", func() {
		delegateCalled = false
		req := httptest.NewRequest("GET", "/apis/subresources.kubevirt.io/v1/namespaces/default/totallynotreal/foo", nil)
		resp, err := rt.RoundTrip(req)
		s.Error(err, "Expected error for non-existent resource in discoverable group")
		s.Nil(resp)
		s.False(delegateCalled, "Expected delegate not to be called for non-existent resource in discoverable group")
		var ve *api.ValidationError
		s.ErrorAs(err, &ve)
		s.Equal(api.ErrorCodeResourceNotFound, ve.Code)
	})

	s.Run("non-existent subresource on valid resource is still rejected", func() {
		delegateCalled = false
		req := httptest.NewRequest("PUT", "/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/bogusaction", nil)
		resp, err := rt.RoundTrip(req)
		s.Error(err, "Expected error for non-existent subresource")
		s.Nil(resp)
		s.False(delegateCalled, "Expected delegate not to be called for non-existent subresource")
		var ve *api.ValidationError
		s.ErrorAs(err, &ve)
		s.Equal(api.ErrorCodeResourceNotFound, ve.Code)
	})

	s.Run("denied group/version is blocked even for discoverable subresource-only API groups", func() {
		delegateCalled = false
		deniedRT := NewAccessControlRoundTripper(s.T().Context(), AccessControlRoundTripperConfig{
			Delegate:                mockDelegate,
			DeniedResourcesProvider: &mockDeniedResourcesProvider{resources: []api.GroupVersionKind{{Group: "subresources.kubevirt.io", Version: "v1"}}},
			RestMapperProvider:      func() meta.RESTMapper { return s.restMapper },
			RawDiscoveryProvider:    func() discovery.DiscoveryInterface { return s.discoveryClient },
		})
		req := httptest.NewRequest("PUT", "/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/pause", nil)
		resp, err := deniedRT.RoundTrip(req)
		s.Error(err, "Expected error for denied group/version in discovery fallback path")
		s.Nil(resp)
		s.False(delegateCalled, "Expected delegate not to be called for denied group/version")
		s.Contains(err.Error(), "resource not allowed")
	})
}

func TestSubresourceOnlyAPIGroup(t *testing.T) {
	suite.Run(t, new(SubresourceOnlyAPIGroupSuite))
}
