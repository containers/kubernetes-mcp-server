package mcpapps

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

type NamespacesAppSuite struct{ suite.Suite }

func (s *NamespacesAppSuite) TestNamespacesList() {
	app := NamespacesList()

	s.Run("declares a UI resource", func() {
		s.Equal("ui://kubernetes-mcp-server/namespaces-list", app.URI)
	})
	s.Run("returns an offline HTML application", func() {
		html, err := app.Handler(context.Background())
		s.Require().NoError(err)
		s.Contains(html, "<!doctype html>")
		s.Contains(html, "ui/notifications/tool-result")
		s.Contains(html, "sortColumn='Name'")
		s.Contains(html, "params.isError")
		s.Contains(html, "e.source!==parent")
		s.NotContains(html, "http://")
		s.NotContains(html, "https://")
		s.NotContains(html, "src=\"//")
		s.NotContains(html, "href=\"//")
	})
}

func TestNamespacesApp(t *testing.T) {
	suite.Run(t, new(NamespacesAppSuite))
}
