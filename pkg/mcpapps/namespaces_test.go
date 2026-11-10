package mcpapps

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

type NamespacesAppSuite struct{ suite.Suite }

func (s *NamespacesAppSuite) TestNamespacesList() {
	app := NamespacesList()

	s.Run("declares a UI resource", func() {
		s.Equal("ui://kubernetes-mcp-server/namespaces-list", app.URI)
	})
	s.Run("returns an offline HTML application", func() {
		content, err := app.Handler(api.ResourceHandlerParams{Context: s.T().Context()})
		s.Require().NoError(err)
		html := content.Text
		s.Contains(html, "<!doctype html>")
		s.Contains(html, "ui/notifications/tool-result")
		s.Contains(html, `data-app-kind="table"`)
		s.NotContains(html, "http://")
		s.NotContains(html, "https://")
		s.NotContains(html, "src=\"//")
		s.NotContains(html, "href=\"//")
	})
}

func TestNamespacesApp(t *testing.T) {
	suite.Run(t, new(NamespacesAppSuite))
}
