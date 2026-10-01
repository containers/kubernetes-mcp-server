package kiali

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type KialiImpersonationSuite struct{ suite.Suite }

func TestKialiImpersonation(t *testing.T) { suite.Run(t, new(KialiImpersonationSuite)) }

func (s *KialiImpersonationSuite) TestExternalAPICannotUseImpersonatorCredentials() {
	for _, tc := range []struct {
		name, mode, user string
		blocked          bool
	}{
		{"configured impersonation", config.ClusterAuthImpersonation, "", true},
		{"derived identity after mode reload", config.ClusterAuthPassthrough, "alice", true},
		{"ordinary bearer authentication", config.ClusterAuthPassthrough, "", false},
	} {
		s.Run(tc.name, func() {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				s.Equal("Bearer backend-credential", r.Header.Get("Authorization"))
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			cfg, err := config.ReadToml(s.T().Context(), []byte(fmt.Sprintf("[toolset_configs.kiali]\nurl = %q\n", server.URL)))
			s.Require().NoError(err)
			cfg.ClusterAuthMode.SetForTest(tc.mode)
			cfg.RequireTLS.SetForTest(false)
			client, err := NewKiali(cfg, &rest.Config{
				BearerToken: "backend-credential",
				Impersonate: rest.ImpersonationConfig{UserName: tc.user},
			})
			if err == nil {
				_, requestErr := client.ExecuteRequest(s.T().Context(), "/api/test", nil)
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
