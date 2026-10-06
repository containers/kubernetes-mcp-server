package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// withResultBudget reads mcp_max_result_bytes from the live server config.
// A nil server, or a server with no config stored, disables the limit.
func withResultBudget(s *Server, ctx context.Context) (context.Context, int64) {
	if s == nil {
		return ctx, 0
	}
	cfg := s.configuration.Load()
	if cfg == nil {
		return ctx, 0
	}
	limit := cfg.McpMaxResultBytes.Get()
	return api.WithResultBudget(ctx, limit), limit
}

func toolResultWithinLimit(limit int64, content string, structured any) error {
	return rejectOversizedToolResult(limit, &api.ToolCallResult{
		Content:           content,
		StructuredContent: structured,
	})
}

// rejectOversizedToolResult rejects a tool result that is still over the cap
// after aggregators have truncated. A tool error is sent as its error text
// alone, so that text is the measured size. When ContentBlocks is set, those
// bytes replace Content, including image data.
func rejectOversizedToolResult(limit int64, result *api.ToolCallResult) error {
	if limit <= 0 || result == nil {
		return nil
	}
	if result.Error != nil {
		return overResultLimit(limit, len(result.Error.Error()))
	}
	size, err := toolResultSize(result)
	if err != nil {
		return err
	}
	return overResultLimit(limit, size)
}

func toolResultSize(result *api.ToolCallResult) (int, error) {
	size := 0
	if len(result.ContentBlocks) > 0 {
		for _, block := range result.ContentBlocks {
			if block != nil {
				size += len(block.Data)
			}
		}
	} else {
		size = len(result.Content)
	}
	structured := result.StructuredContent
	if structured == nil {
		return size, nil
	}
	prepared := ensureStructuredObject(structured)
	if prepared == nil {
		return size, nil
	}
	raw, err := json.Marshal(prepared)
	if err != nil {
		return 0, fmt.Errorf("result structured content: %w", err)
	}
	return size + len(raw), nil
}

func promptWithinLimit(limit int64, messages []api.PromptMessage) error {
	if limit <= 0 {
		return nil
	}
	size := 0
	for _, msg := range messages {
		size += len(msg.Content.Text)
	}
	return overResultLimit(limit, size)
}

func resourceWithinLimit(limit int64, content *api.ResourceContent) error {
	if limit <= 0 || content == nil {
		return nil
	}
	size := len(content.Blob)
	if content.Text != "" {
		size = len(content.Text)
	}
	return overResultLimit(limit, size)
}

func overResultLimit(limit int64, size int) error {
	// The truncation notice may extend a result by its own length.
	if int64(size) <= limit+int64(len(api.ResultTruncationNotice)) {
		return nil
	}
	return fmt.Errorf("result exceeds mcp_max_result_bytes (%d > %d)", size, limit)
}
