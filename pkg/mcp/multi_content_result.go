package mcp

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewMultiContentResult(blocks []*api.ToolContent, structured any, err error) *mcp.CallToolResult {
	if err != nil {
		return NewTextResult("", err)
	}

	result := &mcp.CallToolResult{}
	for _, block := range blocks {
		if block == nil {
			continue
		}

		switch block.Type {
		case api.ToolContentTypeText:
			result.Content = append(result.Content, &mcp.TextContent{
				Text: block.Text,
			})

		case api.ToolContentTypeImage:
			if block.MIMEType == "" || len(block.Data) == 0 {
				return NewTextResult("", fmt.Errorf("image block must have MIME type and data"))
			}
			result.Content = append(result.Content, &mcp.ImageContent{
				MIMEType: block.MIMEType,
				Data:     block.Data,
			})

		default:
			return NewTextResult("", fmt.Errorf("unknown content block type: %s", block.Type))
		}
	}

	return result
}
