package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ClusterStateWatcherTestSuite struct {
	suite.Suite
}

func (s *ClusterStateWatcherTestSuite) TestHasClientCredentials() {
	testCases := map[string]struct {
		restConfig *rest.Config
		expected   bool
	}{
		"no credentials":    {&rest.Config{Host: "https://example.com"}, false},
		"bearer token":      {&rest.Config{BearerToken: "token"}, true},
		"bearer token file": {&rest.Config{BearerTokenFile: "/var/run/token"}, true},
		"basic auth":        {&rest.Config{Username: "user", Password: "password"}, true},
		"client cert data":  {&rest.Config{TLSClientConfig: rest.TLSClientConfig{CertData: []byte("cert")}}, true},
		"client cert file":  {&rest.Config{TLSClientConfig: rest.TLSClientConfig{CertFile: "/etc/cert"}}, true},
		"exec provider":     {&rest.Config{ExecProvider: &clientcmdapi.ExecConfig{Command: "aws"}}, true},
		"auth provider":     {&rest.Config{AuthProvider: &clientcmdapi.AuthProviderConfig{Name: "oidc"}}, true},
		"server CA only":    {&rest.Config{TLSClientConfig: rest.TLSClientConfig{CAData: []byte("ca")}}, false},
	}
	for name, tc := range testCases {
		s.Run(name, func() {
			s.Equal(tc.expected, hasClientCredentials(tc.restConfig))
		})
	}
}

func TestClusterStateWatcher(t *testing.T) {
	suite.Run(t, new(ClusterStateWatcherTestSuite))
}
