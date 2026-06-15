package mcp

import (
	"errors"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/mcpapps"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type TextResultSuite struct {
	suite.Suite
}

func (s *TextResultSuite) TestNewTextResult() {
	s.Run("returns text content for successful result", func() {
		result := NewTextResult("pod list output", nil)
		s.False(result.IsError)
		s.Require().Len(result.Content, 1)
		tc, ok := result.Content[0].(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("pod list output", tc.Text)
		s.Nil(result.StructuredContent)
	})
	s.Run("returns error result when error is provided", func() {
		err := errors.New("connection refused")
		result := NewTextResult("", err)
		s.True(result.IsError)
		s.Require().Len(result.Content, 1)
		tc, ok := result.Content[0].(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("connection refused", tc.Text)
	})
	s.Run("does not set structured content", func() {
		result := NewTextResult("output", nil)
		s.Nil(result.StructuredContent)
	})
}

func (s *TextResultSuite) TestEnsureStructuredObject() {
	s.Run("returns nil for nil input", func() {
		result := ensureStructuredObject(nil)
		s.Nil(result)
	})
	s.Run("wraps slice in items object", func() {
		items := []string{"a", "b"}
		result := ensureStructuredObject(items)
		wrapped, ok := result.(map[string]any)
		s.Require().True(ok)
		s.Equal(items, wrapped["items"])
	})
	s.Run("passes map through unchanged", func() {
		m := map[string]any{"key": "value"}
		result := ensureStructuredObject(m)
		s.Equal(m, result)
	})
	s.Run("passes string through unchanged", func() {
		result := ensureStructuredObject("hello")
		s.Equal("hello", result)
	})
	s.Run("passes nil slice through unchanged", func() {
		var nilSlice []map[string]any
		result := ensureStructuredObject(nilSlice)
		s.Nil(result)
	})
}

func (s *TextResultSuite) TestNewStructuredResult() {
	s.Run("returns text and structured content for successful result", func() {
		structured := map[string]any{"pods": []string{"pod-1", "pod-2"}}
		result := NewStructuredResult(`{"pods":["pod-1","pod-2"]}`, structured, nil)
		s.False(result.IsError)
		s.Require().Len(result.Content, 1)
		tc, ok := result.Content[0].(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal(`{"pods":["pod-1","pod-2"]}`, tc.Text)
		s.Equal(structured, result.StructuredContent)
	})
	s.Run("wraps slice in object for MCP spec compliance", func() {
		items := []map[string]any{{"name": "ns-1"}, {"name": "ns-2"}}
		result := NewStructuredResult("text", items, nil)
		s.False(result.IsError)
		wrapped, ok := result.StructuredContent.(map[string]any)
		s.Require().True(ok, "expected map[string]any wrapper")
		s.Equal(items, wrapped["items"])
	})
	s.Run("does not wrap map structured content", func() {
		structured := map[string]any{"key": "value"}
		result := NewStructuredResult("text", structured, nil)
		s.Equal(structured, result.StructuredContent)
	})
	s.Run("omits structured content for typed nil slice", func() {
		var items []map[string]any // typed nil
		result := NewStructuredResult("text", items, nil)
		s.Nil(result.StructuredContent, "typed nil slice should not produce {\"items\": null}")
	})
	s.Run("omits structured content when nil", func() {
		result := NewStructuredResult("text output", nil, nil)
		s.False(result.IsError)
		s.Require().Len(result.Content, 1)
		tc, ok := result.Content[0].(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("text output", tc.Text)
		s.Nil(result.StructuredContent)
	})
	s.Run("returns error result and ignores structured content", func() {
		err := errors.New("metrics unavailable")
		structured := map[string]any{"should": "be ignored"}
		result := NewStructuredResult("", structured, err)
		s.True(result.IsError)
		s.Require().Len(result.Content, 1)
		tc, ok := result.Content[0].(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent")
		s.Equal("metrics unavailable", tc.Text)
		s.Nil(result.StructuredContent)
	})
}

func TestTextResult(t *testing.T) {
	suite.Run(t, new(TextResultSuite))
}

type RegisterMCPAppResourcesSuite struct {
	suite.Suite
}

func (s *RegisterMCPAppResourcesSuite) newServer() *Server {
	return &Server{
		server: mcp.NewServer(
			&mcp.Implementation{Name: "test"},
			&mcp.ServerOptions{
				Capabilities: &mcp.ServerCapabilities{
					Resources: &mcp.ResourceCapabilities{},
				},
			},
		),
	}
}

func (s *RegisterMCPAppResourcesSuite) TestTracksRegisteredURIs() {
	srv := s.newServer()
	srv.registerMCPAppResources([]string{"pods_list", "nodes_top"})
	s.ElementsMatch(
		[]string{
			mcpapps.ToolResourceURI("pods_list"),
			mcpapps.ToolResourceURI("nodes_top"),
		},
		srv.registeredAppURIs,
	)
}

func (s *RegisterMCPAppResourcesSuite) TestRemovesStaleResources() {
	srv := s.newServer()
	// First registration: two tools
	srv.registerMCPAppResources([]string{"pods_list", "nodes_top"})
	s.Len(srv.registeredAppURIs, 2)
	// Second registration: only one tool remains — stale nodes_top should be cleaned up
	srv.registerMCPAppResources([]string{"pods_list"})
	s.Equal(
		[]string{mcpapps.ToolResourceURI("pods_list")},
		srv.registeredAppURIs,
	)
}

func (s *RegisterMCPAppResourcesSuite) TestUpdatesTrackingOnReRegister() {
	srv := s.newServer()
	srv.registerMCPAppResources([]string{"pods_list"})
	srv.registerMCPAppResources([]string{"pods_list", "namespaces_list"})
	s.ElementsMatch(
		[]string{
			mcpapps.ToolResourceURI("pods_list"),
			mcpapps.ToolResourceURI("namespaces_list"),
		},
		srv.registeredAppURIs,
	)
}

func (s *RegisterMCPAppResourcesSuite) TestEmptyListClearsAll() {
	srv := s.newServer()
	srv.registerMCPAppResources([]string{"pods_list"})
	s.Len(srv.registeredAppURIs, 1)
	srv.registerMCPAppResources([]string{})
	s.Empty(srv.registeredAppURIs)
}

func TestRegisterMCPAppResources(t *testing.T) {
	suite.Run(t, new(RegisterMCPAppResourcesSuite))
}
