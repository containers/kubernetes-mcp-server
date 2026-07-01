package tools

import (
	"encoding/json"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func jsonAPIResult(content string, err error) (*api.ToolCallResult, error) {
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	var structured any
	if jsonErr := json.Unmarshal([]byte(content), &structured); jsonErr != nil {
		return api.NewToolCallResult(content, nil), nil
	}
	return api.NewToolCallResultStructured(structured, nil), nil
}

func textAPIResult(content string, err error) (*api.ToolCallResult, error) {
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func wrapAPIError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to %s: %w", operation, err)
}
