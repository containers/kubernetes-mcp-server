package mcp

import (
	"errors"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNewMultiContentResult(t *testing.T) {
	t.Run("converts text blocks", func(t *testing.T) {
		blocks := []*api.ToolContent{
			api.NewTextToolContent("hello"),
			api.NewTextToolContent("world"),
		}
		result := NewMultiContentResult(blocks, nil, nil)
		if len(result.Content) != 2 {
			t.Fatalf("expected 2 content blocks, got %d", len(result.Content))
		}

		text1, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("expected TextContent, got %T", result.Content[0])
		}
		if text1.Text != "hello" {
			t.Fatalf("expected 'hello', got %q", text1.Text)
		}
	})

	t.Run("converts image blocks", func(t *testing.T) {
		pngData := []byte{0x89, 0x50, 0x4e, 0x47}
		blocks := []*api.ToolContent{
			api.NewImageToolContent(pngData, "image/png"),
		}
		result := NewMultiContentResult(blocks, nil, nil)
		if len(result.Content) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(result.Content))
		}

		img, ok := result.Content[0].(*mcp.ImageContent)
		if !ok {
			t.Fatalf("expected ImageContent, got %T", result.Content[0])
		}
		if img.MIMEType != "image/png" {
			t.Fatalf("expected MIME type 'image/png', got %q", img.MIMEType)
		}
		if len(img.Data) != len(pngData) {
			t.Fatalf("expected %d bytes, got %d", len(pngData), len(img.Data))
		}
	})

	t.Run("preserves block order", func(t *testing.T) {
		blocks := []*api.ToolContent{
			api.NewTextToolContent("description"),
			api.NewImageToolContent([]byte{1, 2, 3}, "image/png"),
		}
		result := NewMultiContentResult(blocks, nil, nil)
		if len(result.Content) != 2 {
			t.Fatalf("expected 2 blocks, got %d", len(result.Content))
		}

		_, isText := result.Content[0].(*mcp.TextContent)
		_, isImage := result.Content[1].(*mcp.ImageContent)
		if !isText || !isImage {
			t.Fatal("expected text then image, got different types")
		}
	})

	t.Run("returns error on invalid image (missing MIME type)", func(t *testing.T) {
		block := &api.ToolContent{Type: api.ToolContentTypeImage, Data: []byte{1, 2, 3}}
		result := NewMultiContentResult([]*api.ToolContent{block}, nil, nil)
		if !result.IsError {
			t.Fatal("expected error result")
		}
	})

	t.Run("returns error on invalid image (empty data)", func(t *testing.T) {
		block := &api.ToolContent{Type: api.ToolContentTypeImage, MIMEType: "image/png"}
		result := NewMultiContentResult([]*api.ToolContent{block}, nil, nil)
		if !result.IsError {
			t.Fatal("expected error result")
		}
	})

	t.Run("forwards existing error unchanged", func(t *testing.T) {
		originalErr := errors.New("test error")
		result := NewMultiContentResult(nil, nil, originalErr)
		if !result.IsError {
			t.Fatal("expected error result")
		}
	})

	t.Run("handles nil blocks gracefully", func(t *testing.T) {
		blocks := []*api.ToolContent{
			api.NewTextToolContent("text"),
			nil,
			api.NewImageToolContent([]byte{1, 2}, "image/png"),
		}
		result := NewMultiContentResult(blocks, nil, nil)
		if len(result.Content) != 2 {
			t.Fatalf("expected 2 non-nil blocks, got %d", len(result.Content))
		}
	})
}
