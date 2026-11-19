package mcp

import (
	"errors"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type MultiContentResultSuite struct {
	suite.Suite
}

func (s *MultiContentResultSuite) TestContentConversion() {
	s.Run("converts text blocks", func() {
		blocks := []*api.ToolContent{
			api.NewTextToolContent("hello"),
			api.NewTextToolContent("world"),
		}
		result := NewMultiContentResult(blocks, nil, nil)
		s.Require().Len(result.Content, 2)

		text, ok := result.Content[0].(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent, got %T", result.Content[0])
		s.Equal("hello", text.Text)
	})

	s.Run("converts image blocks", func() {
		pngData := []byte{0x89, 0x50, 0x4e, 0x47}
		result := NewMultiContentResult([]*api.ToolContent{api.NewImageToolContent(pngData, "image/png")}, nil, nil)
		s.Require().Len(result.Content, 1)

		img, ok := result.Content[0].(*mcp.ImageContent)
		s.Require().True(ok, "expected ImageContent, got %T", result.Content[0])
		s.Equal("image/png", img.MIMEType)
		s.Len(img.Data, len(pngData))
	})

	s.Run("converts a block with an explicit text/ MIME type to text, not an image", func() {
		block := &api.ToolContent{Data: []byte("plain"), MIMEType: "text/plain"}
		result := NewMultiContentResult([]*api.ToolContent{block}, nil, nil)
		s.Require().Len(result.Content, 1)

		text, ok := result.Content[0].(*mcp.TextContent)
		s.Require().True(ok, "expected TextContent, got %T", result.Content[0])
		s.Equal("plain", text.Text)
	})

	s.Run("preserves block order", func() {
		blocks := []*api.ToolContent{
			api.NewTextToolContent("description"),
			api.NewImageToolContent([]byte{1, 2, 3}, "image/png"),
		}
		result := NewMultiContentResult(blocks, nil, nil)
		s.Require().Len(result.Content, 2)
		s.IsType(&mcp.TextContent{}, result.Content[0])
		s.IsType(&mcp.ImageContent{}, result.Content[1])
	})

	s.Run("handles nil blocks gracefully", func() {
		blocks := []*api.ToolContent{
			api.NewTextToolContent("text"),
			nil,
			api.NewImageToolContent([]byte{1, 2}, "image/png"),
		}
		result := NewMultiContentResult(blocks, nil, nil)
		s.Len(result.Content, 2, "expected nil blocks to be skipped")
	})
}

func (s *MultiContentResultSuite) TestErrors() {
	s.Run("returns error on an image block with empty data", func() {
		block := &api.ToolContent{MIMEType: "image/png"}
		s.True(NewMultiContentResult([]*api.ToolContent{block}, nil, nil).IsError)
	})

	s.Run("forwards existing error unchanged", func() {
		s.True(NewMultiContentResult(nil, nil, errors.New("test error")).IsError)
	})
}

func TestMultiContentResultSuite(t *testing.T) {
	suite.Run(t, new(MultiContentResultSuite))
}
