package kubernetes_test

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type MultiClusterImpersonationSuite struct{ suite.Suite }

func TestMultiClusterImpersonation(t *testing.T) {
	suite.Run(t, new(MultiClusterImpersonationSuite))
}

func (s *MultiClusterImpersonationSuite) TestConcurrentUsersKeepTargetCredentialsAndIdentity() {
	received := make(map[string]func() []*http.Request)
	raw := clientcmdapi.NewConfig()
	versions := map[string]string{"cluster-a": "v1.35.1", "cluster-b": "v1.35.2"}
	for target, version := range versions {
		recorder, requests := test.RecordRequests()
		received[target] = requests
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/version" {
				http.NotFound(w, r)
				return
			}
			recorder.ServeHTTP(w, r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"major":"1","minor":"35","gitVersion":%q}`, version)
		}))
		s.T().Cleanup(server.Close)
		raw.Clusters[target] = &clientcmdapi.Cluster{
			Server: server.URL,
			CertificateAuthorityData: pem.EncodeToMemory(&pem.Block{
				Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
			}),
		}
		raw.AuthInfos[target] = &clientcmdapi.AuthInfo{Token: target + "-backend-credential"}
		raw.Contexts[target] = &clientcmdapi.Context{Cluster: target, AuthInfo: target}
	}
	raw.CurrentContext = "cluster-a"
	cfg := config.BaseDefault()
	cfg.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), raw))
	cfg.ClusterProviderStrategy.SetForTest(config.ClusterProviderKubeConfig)
	cfg.ClusterAuthMode.SetForTest(config.ClusterAuthImpersonation)
	ctx, cancel := context.WithTimeout(s.T().Context(), 15*time.Second)
	defer cancel()
	provider, err := kubernetes.NewProvider(ctx, cfg)
	s.Require().NoError(err)
	defer provider.Close()

	const callsPerIdentity = 5
	results := make(chan error, len(versions)*2*callsPerIdentity)
	for target, expectedVersion := range versions {
		for _, user := range []string{"alice", "bob"} {
			for range callsPerIdentity {
				go func() {
					requestCtx := context.WithValue(ctx, kubernetes.OAuthAuthorizationHeader, "Bearer caller-token-never-forwarded")
					requestCtx = kubernetes.WithImpersonationIdentity(requestCtx, kubernetes.ImpersonationIdentity{
						UserName: user, Groups: []string{user + "-group"},
					})
					client, err := provider.GetDerivedKubernetes(requestCtx, target)
					if err != nil {
						results <- err
						return
					}
					version, err := client.DiscoveryClient().ServerVersion()
					if err == nil && version.GitVersion != expectedVersion {
						err = fmt.Errorf("%s received version %q from the wrong target", target, version.GitVersion)
					}
					results <- err
				}()
			}
		}
	}
	for range cap(results) {
		select {
		case err := <-results:
			s.Require().NoError(err)
		case <-ctx.Done():
			s.FailNow("concurrent impersonation requests did not finish", ctx.Err().Error())
		}
	}
	for target, requests := range received {
		counts := make(map[string]int)
		recorded := requests()
		s.Len(recorded, 2*callsPerIdentity)
		for _, request := range recorded {
			user := request.Header.Get("Impersonate-User")
			s.Contains([]string{"alice", "bob"}, user)
			s.Equal([]string{user + "-group"}, request.Header.Values("Impersonate-Group"))
			s.Equal("Bearer "+target+"-backend-credential", request.Header.Get("Authorization"))
			s.NotContains(fmt.Sprint(request.Header), "caller-token-never-forwarded")
			counts[user]++
		}
		for _, user := range []string{"alice", "bob"} {
			s.Equal(callsPerIdentity, counts[user], "each target must receive both distinct callers")
		}
	}
}
