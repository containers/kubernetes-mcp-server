package mcpapps

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type TableAppSuite struct{ suite.Suite }

func (s *TableAppSuite) TestTable() {
	app := Table("ui://example/table", "Example table", WithDescription("Renders rows"))

	s.Run("declares resource details", func() {
		s.Equal("ui://example/table", app.URI)
		s.Equal("Example table", app.Name)
		s.Equal("Renders rows", app.Description)
	})
	s.Run("returns a self-contained application", func() {
		content, err := app.Handler(api.ResourceHandlerParams{Context: s.T().Context()})
		s.Require().NoError(err)
		html := content.Text
		s.Contains(html, "<!doctype html>")
		s.Contains(html, "ui/notifications/tool-result")
		s.NotContains(html, "http://")
		s.NotContains(html, "https://")
	})
}

func (s *TableAppSuite) TestStandardAppsBundleSharedAssets() {
	for _, factory := range []struct {
		kind string
		new  func(string, string, ...CustomOption) *api.ToolApp
	}{
		{"table", Table}, {"metrics", Metrics}, {"resource", Resource},
	} {
		s.Run(factory.kind, func() {
			app := factory.new("ui://example/"+factory.kind, "Example", WithDescription("Shared app"))
			s.Require().NoError(app.Validate())
			content, err := app.Handler(api.ResourceHandlerParams{Context: s.T().Context()})
			s.Require().NoError(err)
			s.Require().NotNil(content)
			s.Contains(content.Text, `data-app-kind="`+factory.kind+`"`)
			s.Contains(content.Text, "--app-background")
			s.NotContains(content.Text, "{{")
			s.NotContains(content.Text, "https://")
			s.NotContains(content.Text, "http://")
		})
	}
}

func TestTableApp(t *testing.T) {
	suite.Run(t, new(TableAppSuite))
}
