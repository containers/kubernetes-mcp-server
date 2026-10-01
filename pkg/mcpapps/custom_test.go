package mcpapps_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/mcpapps"
)

type CustomAppSuite struct{ suite.Suite }

type contentProviderContextKey string

func (s *CustomAppSuite) TestStaticHTML() {
	app := mcpapps.Custom(
		"ui://example/static",
		"Static example",
		mcpapps.StaticHTML("<!doctype html><title>Static</title>"),
		mcpapps.WithDescription("A static app"),
		mcpapps.WithMetadata(map[string]any{"ui": map[string]any{"prefersBorder": true}}),
	)

	s.Run("declares resource details", func() {
		s.Equal("ui://example/static", app.URI)
		s.Equal("Static example", app.Name)
		s.Equal("A static app", app.Description)
		s.Equal(map[string]any{"ui": map[string]any{"prefersBorder": true}}, app.Meta)
	})
	s.Run("returns embedded HTML", func() {
		content, err := app.Handler(s.resourceHandlerParams())
		s.Require().NoError(err)
		s.Require().NotNil(content)
		s.Equal("<!doctype html><title>Static</title>", content.Text)
	})
}

func (s *CustomAppSuite) TestContentProvider() {
	const ctxKey contentProviderContextKey = "content-provider-test"
	expectedErr := errors.New("assets unavailable")
	calls := 0
	app := mcpapps.Custom("ui://example/provider", "Provider example", func(params api.ResourceHandlerParams) (string, error) {
		calls++
		if params.Value(ctxKey) == "failure" {
			return "", expectedErr
		}
		return "<!doctype html><title>Provider</title>", nil
	})

	s.Run("does not invoke the provider during app construction", func() {
		s.Zero(calls)
	})
	s.Run("receives the resource params", func() {
		params := s.resourceHandlerParams()
		params.Context = context.WithValue(params.Context, ctxKey, "success")
		content, err := app.Handler(params)
		s.Require().NoError(err)
		s.Require().NotNil(content)
		s.Equal("<!doctype html><title>Provider</title>", content.Text)
	})
	s.Run("preserves provider errors", func() {
		params := s.resourceHandlerParams()
		params.Context = context.WithValue(params.Context, ctxKey, "failure")
		content, err := app.Handler(params)
		s.ErrorIs(err, expectedErr)
		s.Nil(content)
	})
}

func (s *CustomAppSuite) TestMissingContentProvider() {
	app := mcpapps.Custom("ui://example/missing", "Missing provider", nil)

	s.Error(app.Validate())
}

func (s *CustomAppSuite) resourceHandlerParams() api.ResourceHandlerParams {
	return api.ResourceHandlerParams{Context: s.T().Context()}
}

func TestCustomApp(t *testing.T) {
	suite.Run(t, new(CustomAppSuite))
}
