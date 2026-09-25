package mcpapps_test

import (
	"context"
	"errors"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/mcpapps"
	"github.com/stretchr/testify/suite"
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
		html, err := app.Handler(context.Background())
		s.Require().NoError(err)
		s.Equal("<!doctype html><title>Static</title>", html)
	})
}

func (s *CustomAppSuite) TestContentProvider() {
	const ctxKey contentProviderContextKey = "content-provider-test"
	expectedErr := errors.New("assets unavailable")
	calls := 0
	app := mcpapps.Custom("ui://example/provider", "Provider example", func(ctx context.Context) (string, error) {
		calls++
		if ctx.Value(ctxKey) == "failure" {
			return "", expectedErr
		}
		return "<!doctype html><title>Provider</title>", nil
	})

	s.Run("does not invoke the provider during app construction", func() {
		s.Zero(calls)
	})
	s.Run("receives the resource context", func() {
		html, err := app.Handler(context.WithValue(context.Background(), ctxKey, "success"))
		s.Require().NoError(err)
		s.Equal("<!doctype html><title>Provider</title>", html)
	})
	s.Run("preserves provider errors", func() {
		_, err := app.Handler(context.WithValue(context.Background(), ctxKey, "failure"))
		s.ErrorIs(err, expectedErr)
	})
}

func (s *CustomAppSuite) TestMissingContentProvider() {
	app := mcpapps.Custom("ui://example/missing", "Missing provider", nil)

	s.Error(app.Validate())
}

func TestCustomApp(t *testing.T) {
	suite.Run(t, new(CustomAppSuite))
}
