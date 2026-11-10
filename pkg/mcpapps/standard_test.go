package mcpapps_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/mcpapps"
)

type StandardAppSuite struct{ suite.Suite }

func (s *StandardAppSuite) TestStyleOverrides() {
	for _, factory := range []struct {
		name string
		new  func(string, string, ...mcpapps.CustomOption) *api.ToolApp
	}{
		{"table", mcpapps.Table}, {"metrics", mcpapps.Metrics}, {"resource", mcpapps.Resource},
	} {
		s.Run(factory.name, func() {
			read := func(options ...mcpapps.CustomOption) string {
				app := factory.new("ui://example/"+factory.name, "Example", options...)
				s.Require().NoError(app.Validate())
				content, err := app.Handler(api.ResourceHandlerParams{Context: s.T().Context()})
				s.Require().NoError(err)
				s.Require().NotNil(content)
				return content.Text
			}
			baseline := read()
			const first = "body { font-family: monospace; }"
			const second = "body { padding: 24px; }"

			s.Run("appends overrides after the existing styles", func() {
				styled := read(mcpapps.WithStyles(first))
				s.Equal(strings.Replace(baseline, "</head>", "<style>"+first+"</style>\n</head>", 1), styled)
			})
			s.Run("preserves option order", func() {
				styled := read(mcpapps.WithStyles(first), mcpapps.WithStyles(second))
				s.Contains(styled, "<style>"+first+"</style>\n<style>"+second+"</style>\n</head>")
			})
			s.Run("empty overrides leave the document unchanged", func() {
				s.Equal(baseline, read(mcpapps.WithStyles("")))
			})
			s.Run("overrides do not change other app instances", func() {
				read(mcpapps.WithStyles(first))
				s.Equal(baseline, read())
			})
		})
	}
}

func (s *StandardAppSuite) TestStyleOverridePreservesProviderBehavior() {
	expectedErr := errors.New("assets unavailable")
	for _, failure := range []bool{false, true} {
		name := "successful read"
		if failure {
			name = "provider error"
		}
		s.Run(name, func() {
			app := mcpapps.Custom("ui://example/custom", "Custom", func(params api.ResourceHandlerParams) (string, error) {
				s.Equal(s.T().Context(), params.Context)
				if failure {
					return "", expectedErr
				}
				return "<!doctype html><html><head></head><body>Example</body></html>", nil
			}, mcpapps.WithStyles("body { padding: 24px; }"), mcpapps.WithDescription("Styled app"))
			s.Equal("Styled app", app.Description)
			first, err := app.Handler(api.ResourceHandlerParams{Context: s.T().Context()})
			if failure {
				s.ErrorIs(err, expectedErr)
				s.Nil(first)
				return
			}
			s.Require().NoError(err)
			second, err := app.Handler(api.ResourceHandlerParams{Context: s.T().Context()})
			s.Require().NoError(err)
			s.Equal(first.Text, second.Text, "repeated reads must not accumulate overrides")
			s.Equal(1, strings.Count(first.Text, "body { padding: 24px; }"))
		})
	}
}

func (s *StandardAppSuite) TestStyleOverridePreservesMissingProviderValidation() {
	app := mcpapps.Custom("ui://example/missing", "Missing provider", nil, mcpapps.WithStyles("body { padding: 24px; }"))
	s.Error(app.Validate())
}

func TestStandardApp(t *testing.T) {
	suite.Run(t, new(StandardAppSuite))
}
