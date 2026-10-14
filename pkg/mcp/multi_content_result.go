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

	if structured != nil {
		result.StructuredContent = ensureStructuredObject(structured)
	}

	for _, block := range blocks {
		if block == nil {
			continue
		}

		if block.IsText() {
			result.Content = append(result.Content, &mcp.TextContent{
				Text: block.Text(),
			})
			continue
		}

		if len(block.Data) == 0 {
			return NewTextResult("", fmt.Errorf("image block must have data"))
		}

		result.Content = append(result.Content, &mcp.ImageContent{
			MIMEType: block.MIMEType,
			Data:     block.Data,
		})
	}
	return result
}
