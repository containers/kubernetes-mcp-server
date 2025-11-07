package kiali

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func (s *KialiSuite) TestMeshStatus() {
	var capturedURL *url.URL
	s.MockServer.Config().BearerToken = "token-xyz"
	s.MockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURL = &u
		_, _ = w.Write([]byte("graph"))
	}))

	s.Config = test.Must(config.ReadToml([]byte(fmt.Sprintf(`
		[toolset_configs.kiali]
		url = "%s"
	`, s.MockServer.Config().Host))))
	k := NewKiali(s.Config, s.MockServer.Config())

	out, err := k.MeshStatus(s.T().Context())
	s.Require().NoError(err, "Expected no error executing request")
	s.Run("response body is correct", func() {
		s.Equal("graph", out, "Unexpected response body")
	})
	s.Run("path is correct", func() {
		s.Equal("/api/mesh/graph", capturedURL.Path, "Unexpected path")
	})
	s.Run("query parameters are correct", func() {
		s.Equal("false", capturedURL.Query().Get("includeGateways"), "Unexpected includeGateways query parameter")
		s.Equal("false", capturedURL.Query().Get("includeWaypoints"), "Unexpected includeWaypoints query parameter")
	})

}
