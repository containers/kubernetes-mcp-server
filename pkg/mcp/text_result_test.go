package mcp

import (
	"errors"
	"testing"

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
