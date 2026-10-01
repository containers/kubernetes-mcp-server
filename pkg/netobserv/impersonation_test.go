package netobserv

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type NetObservImpersonationSuite struct{ suite.Suite }

func TestNetObservImpersonation(t *testing.T) { suite.Run(t, new(NetObservImpersonationSuite)) }

func (s *NetObservImpersonationSuite) TestExternalAPICannotUseImpersonatorCredentials() {
	for _, tc := range []struct {
		name, mode, user string
		blocked          bool
	}{
		{"configured impersonation", config.ClusterAuthImpersonation, "", true},
		{"derived identity after mode reload", config.ClusterAuthPassthrough, "alice", true},
		{"ordinary bearer authentication", config.ClusterAuthPassthrough, "", false},
	} {
		for _, fromFile := range []bool{false, true} {
			s.Run(fmt.Sprintf("%s/file=%v", tc.name, fromFile), func() {
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					s.Equal("Bearer backend-credential", r.Header.Get("Authorization"))
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				cfg, err := config.ReadToml(s.T().Context(), []byte(fmt.Sprintf("[toolset_configs.netobserv]\nurl = %q\n", server.URL)))
				s.Require().NoError(err)
				cfg.ClusterAuthMode.SetForTest(tc.mode)
				cfg.RequireTLS.SetForTest(false)
				restConfig := &rest.Config{
					BearerToken: "backend-credential",
					Impersonate: rest.ImpersonationConfig{UserName: tc.user},
				}
				if fromFile {
					restConfig.BearerToken = ""
					restConfig.BearerTokenFile = filepath.Join(s.T().TempDir(), "token")
					s.Require().NoError(os.WriteFile(restConfig.BearerTokenFile, []byte("backend-credential"), 0600))
				}
				client, err := NewNetObserv(s.T().Context(), cfg, restConfig, nil)
				if err == nil {
					_, requestErr := client.ExecuteGet(s.T().Context(), "/api/test", nil)
					s.NoError(requestErr)
				}
				if tc.blocked {
					s.ErrorContains(err, "impersonation")
					s.Nil(client)
					s.Zero(requests.Load(), "the external API must never receive backend credentials")
				} else {
					s.NoError(err)
					s.Equal(int32(1), requests.Load())
				}
			})
		}
	}
}
