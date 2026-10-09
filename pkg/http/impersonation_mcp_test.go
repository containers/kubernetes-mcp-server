package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type ImpersonationMCPSuite struct{ BaseHttpSuite }

func TestImpersonationMCP(t *testing.T) { suite.Run(t, new(ImpersonationMCPSuite)) }

func (s *ImpersonationMCPSuite) SetupTest() {
	s.BaseHttpSuite.SetupTest()
	s.Config = config.BaseDefault()
	kubeconfig := s.MockServer.TLSKubeconfig(s.T())
	kubeconfig.AuthInfos["fake"].Token = "backend-credential"
	s.Config.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), kubeconfig))
	s.Config.ClusterAuthMode.SetForTest(config.ClusterAuthImpersonation)
	s.Config.ImpersonationTrustedProxies.SetForTest([]string{"127.0.0.1/32"})
	s.Config.RequireOAuth.SetForTest(false)
}

type impersonationTestTransport struct{ headers atomic.Value }

func (t *impersonationTestTransport) setIdentity(user string, groups ...string) {
	h := make(http.Header)
	h.Set("Authorization", "Bearer frontend-credential")
	h.Set("Impersonate-User", user)
	for _, group := range groups {
		h.Add("Impersonate-Group", group)
	}
	t.headers.Store(h)
}

func (t *impersonationTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	for k, values := range t.headers.Load().(http.Header) {
		r.Header[k] = append([]string(nil), values...)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func (s *ImpersonationMCPSuite) connect(user string, groups ...string) (*mcp.ClientSession, *impersonationTestTransport) {
	transport := &impersonationTestTransport{}
	transport.setIdentity(user, groups...)
	client := mcp.NewClient(&mcp.Implementation{Name: "impersonation-test", Version: "1"}, nil)
	session, err := client.Connect(s.T().Context(), &mcp.StreamableClientTransport{
		Endpoint:   fmt.Sprintf("http://127.0.0.1:%s/mcp", s.Config.Port.Get()),
		HTTPClient: &http.Client{Transport: transport},
	}, nil)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = session.Close() })
	return session, transport
}

func (s *ImpersonationMCPSuite) TestCurrentIdentityAndConcurrentUsers() {
	for _, stateless := range []bool{false, true} {
		s.Run(fmt.Sprintf("stateless=%v", stateless), func() {
			s.Config.Stateless.SetForTest(stateless)
			recorder, received := test.RecordRequests()
			s.MockServer.Handle(recorder)
			s.MockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/pods") {
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"apiVersion":"v1","kind":"PodList","items":[]}`)
			}))
			s.StartServer()
			alice, aliceTransport := s.connect("alice", "readers")
			bob, _ := s.connect("bob", "developers")
			call := func(session *mcp.ClientSession, namespace string) error {
				result, err := session.CallTool(s.T().Context(), &mcp.CallToolParams{
					Name: "resources_list", Arguments: map[string]any{"apiVersion": "v1", "kind": "Pod", "namespace": namespace},
				})
				if err == nil && result.IsError {
					return fmt.Errorf("tool returned an error: %v", result.Content[0].(*mcp.TextContent).Text)
				}
				return err
			}
			s.Require().NoError(call(alice, "alice-before"))
			aliceTransport.setIdentity("alice", "restricted-readers")
			errors := make(chan error, 10)
			for range 5 {
				go func() { errors <- call(alice, "alice-after") }()
				go func() { errors <- call(bob, "bob") }()
			}
			for range 10 {
				s.Require().NoError(<-errors)
			}
			observed := make(map[string][]http.Header)
			for _, request := range received() {
				observed[request.URL.Path] = append(observed[request.URL.Path], request.Header)
			}
			for namespace, expected := range map[string]struct{ user, group string }{
				"alice-before": {"alice", "readers"},
				"alice-after":  {"alice", "restricted-readers"},
				"bob":          {"bob", "developers"},
			} {
				requests := observed["/api/v1/namespaces/"+namespace+"/pods"]
				s.NotEmpty(requests)
				for _, headers := range requests {
					s.Equal(expected.user, headers.Get("Impersonate-User"))
					s.Equal([]string{expected.group}, headers.Values("Impersonate-Group"))
					s.Equal("Bearer backend-credential", headers.Get("Authorization"))
				}
			}
			s.Require().NoError(alice.Close())
			s.Require().NoError(bob.Close())
			s.stopRunningServer()
			s.MockServer.ResetHandlers()
			s.MockServer.Handle(test.NewDiscoveryClientHandler())
		})
	}
}

func (s *ImpersonationMCPSuite) TestStatefulSessionRejectsOtherUser() {
	s.Config.Stateless.SetForTest(false)
	s.StartServer()
	alice, _ := s.connect("alice", "readers")
	s.Require().NotEmpty(alice.ID())
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		s.Run(method, func() {
			req, err := http.NewRequestWithContext(s.T().Context(), method,
				fmt.Sprintf("http://127.0.0.1:%s/mcp", s.Config.Port.Get()),
				strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
			s.Require().NoError(err)
			req.Header.Set("Authorization", "Bearer another-valid-frontend-token")
			req.Header.Set("Impersonate-User", "bob")
			req.Header.Set("Mcp-Session-Id", alice.ID())
			req.Header.Set("Mcp-Protocol-Version", "2025-03-26")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			resp, err := http.DefaultClient.Do(req)
			s.Require().NoError(err)
			defer func() { _ = resp.Body.Close() }()
			s.Equal(http.StatusForbidden, resp.StatusCode)
		})
	}
	_, err := alice.ListTools(s.T().Context(), &mcp.ListToolsParams{})
	s.NoError(err, "rejected DELETE must leave the owner's session usable")
}
