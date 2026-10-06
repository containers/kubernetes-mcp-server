package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

type ResultLimitSuite struct {
	suite.Suite
}

func TestResultLimit(t *testing.T) {
	suite.Run(t, new(ResultLimitSuite))
}

func (s *ResultLimitSuite) TestToolResultRejectedWhenContentPlusStructuredContentExceedsCap() {
	content := strings.Repeat("a", 80)
	structured := map[string]string{"k": strings.Repeat("b", 80)}
	raw, err := json.Marshal(structured)
	s.Require().NoError(err)
	total := len(content) + len(raw)
	limit := int64(max(len(content), len(raw)))
	s.Greater(total, int(limit)+len(api.ResultTruncationNotice))

	s.NoError(toolResultWithinLimit(limit, content, nil))
	s.NoError(toolResultWithinLimit(limit, "", structured))
	s.NoError(toolResultWithinLimit(int64(total), content, structured))
	s.ErrorContains(toolResultWithinLimit(limit, content, structured), "mcp_max_result_bytes")
	s.NoError(toolResultWithinLimit(10, strings.Repeat("a", 10+len(api.ResultTruncationNotice)), nil))
	s.ErrorContains(toolResultWithinLimit(10, strings.Repeat("a", 11+len(api.ResultTruncationNotice)), nil), "mcp_max_result_bytes")
	s.NoError(toolResultWithinLimit(0, strings.Repeat("a", total), structured))

	var nilSlice []string
	s.NoError(toolResultWithinLimit(int64(len(content)), content, nilSlice))

	s.NoError(rejectOversizedToolResult(1, &api.ToolCallResult{
		Content: strings.Repeat("a", 100),
		Error:   errors.New("tool failed"),
	}))
	s.ErrorContains(rejectOversizedToolResult(1, &api.ToolCallResult{
		Error: errors.New(strings.Repeat("e", len(api.ResultTruncationNotice)+8)),
	}), "mcp_max_result_bytes")

	image := api.NewImageToolContent(bytesOf(len(api.ResultTruncationNotice)+8), "image/png")
	s.NoError(rejectOversizedToolResult(80, &api.ToolCallResult{
		Content:       strings.Repeat("a", 10_000),
		ContentBlocks: []*api.ToolContent{api.NewTextToolContent("ok"), image},
	}))
	s.ErrorContains(rejectOversizedToolResult(1, &api.ToolCallResult{
		Content:       strings.Repeat("a", 10_000),
		ContentBlocks: []*api.ToolContent{image},
	}), "mcp_max_result_bytes")
}

func (s *ResultLimitSuite) TestPromptAndResourceMeasuredSizes() {
	messages := []api.PromptMessage{
		{Content: api.PromptContent{Text: strings.Repeat("a", 40)}},
		{Content: api.PromptContent{Text: strings.Repeat("b", 40)}},
	}
	s.NoError(promptWithinLimit(80, messages))
	s.ErrorContains(promptWithinLimit(int64(80-len(api.ResultTruncationNotice)-1), messages), "mcp_max_result_bytes")
	s.NoError(promptWithinLimit(0, messages))

	s.NoError(resourceWithinLimit(80, &api.ResourceContent{Text: strings.Repeat("a", 80)}))
	s.ErrorContains(resourceWithinLimit(int64(80-len(api.ResultTruncationNotice)-1), &api.ResourceContent{Blob: bytesOf(80)}), "mcp_max_result_bytes")
	s.NoError(resourceWithinLimit(0, &api.ResourceContent{Text: "abcdefgh"}))
}

func (s *ResultLimitSuite) TestNilServerDisablesResourceLimit() {
	_, handler, err := ServerResourceToGoSdkResource(nil, api.ServerResource{
		Resource: api.Resource{URI: "test://example/big", Name: "big", MIMEType: "text/plain"},
		Handler: func(api.ResourceHandlerParams) (*api.ResourceContent, error) {
			return &api.ResourceContent{Text: strings.Repeat("a", 100)}, nil
		},
	})
	s.Require().NoError(err)
	result, err := handler(context.Background(), nil)
	s.Require().NoError(err)
	s.Require().Len(result.Contents, 1)
	s.Len(result.Contents[0].Text, 100)

	_, templateHandler, err := ServerResourceTemplateToGoSdkResourceTemplate(nil, api.ServerResourceTemplate{
		ResourceTemplate: api.ResourceTemplate{URITemplate: "test://example/{name}", Name: "big"},
		Handler: func(api.ResourceHandlerParams) (*api.ResourceContent, error) {
			return &api.ResourceContent{Blob: bytesOf(80)}, nil
		},
	})
	s.Require().NoError(err)
	templateResult, err := templateHandler(context.Background(), &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "test://example/item"},
	})
	s.Require().NoError(err)
	s.Require().Len(templateResult.Contents, 1)
	s.Len(templateResult.Contents[0].Blob, 80)
}

func bytesOf(n int) []byte {
	return []byte(strings.Repeat("b", n))
}
