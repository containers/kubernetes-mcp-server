package kubernetes

import (
	"net/http"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type KubernetesTestSuite struct {
	suite.Suite
}

func (s *KubernetesTestSuite) TestDiscoveryRequestsHaveDefaultTimeout() {
	called := false
	requestTimeout := time.Duration(0)
	discoveryHandler := test.NewDiscoveryClientHandler()
	transport := &mockRoundTripper{
		called: &called,
		onRequest: func(w http.ResponseWriter, r *http.Request) {
			deadline, ok := r.Context().Deadline()
			s.True(ok, "Expected discovery request context to have a deadline")
			if ok {
				requestTimeout = time.Until(deadline)
			}
			discoveryHandler.ServeHTTP(w, r)
		},
	}

	rawConfig := clientcmdapi.NewConfig()
	k, err := NewKubernetes(
		s.T().Context(),
		&config.StaticConfig{},
		clientcmd.NewDefaultClientConfig(*rawConfig, &clientcmd.ConfigOverrides{}),
		&rest.Config{Host: "https://cluster.example", Transport: transport},
	)
	s.Require().NoError(err)
	s.T().Cleanup(k.close)

	_, err = k.DiscoveryClient().ServerResourcesForGroupVersion("v1")
	s.NoError(err)
	s.True(called, "Expected discovery request to reach the transport")
	s.GreaterOrEqual(requestTimeout, 9*time.Second)
	s.LessOrEqual(requestTimeout, 10*time.Second)
	s.Zero(k.httpClient.Timeout, "Expected the shared Kubernetes client to remain unbounded")
}

func TestKubernetes(t *testing.T) {
	suite.Run(t, new(KubernetesTestSuite))
}
