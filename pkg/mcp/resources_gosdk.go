package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yosida95/uritemplate/v3"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// ServerResourceToGoSdkResource converts an api.ServerResource to MCP SDK types.
// It validates the URI upfront so callers can surface a wrapped error instead of
// letting the SDK panic during registration on hot reload.
func ServerResourceToGoSdkResource(_ *Server, res api.ServerResource) (*mcp.Resource, mcp.ResourceHandler, error) {
	if _, err := url.Parse(res.Resource.URI); err != nil {
		return nil, nil, fmt.Errorf("invalid URI %q: %w", res.Resource.URI, err)
	}
	mcpResource := &mcp.Resource{
		URI:         res.Resource.URI,
		Name:        res.Resource.Name,
		Description: res.Resource.Description,
		MIMEType:    res.Resource.MIMEType,
	}
	handler := func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		content, err := res.Handler(ctx)
		if err != nil {
			return nil, err
		}
		if content == nil {
			return nil, errors.New("resource handler returned nil content")
		}
		if err := validateResourceContent(content); err != nil {
			return nil, err
		}
		mimeType := res.Resource.MIMEType
		if content.MIMEType != "" {
			mimeType = content.MIMEType
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      res.Resource.URI,
				MIMEType: mimeType,
				Text:     content.Text,
				Blob:     content.Blob,
			}},
		}, nil
	}
	return mcpResource, handler, nil
}

// ServerResourceTemplateToGoSdkResourceTemplate converts an api.ServerResourceTemplate to MCP SDK types.
// It validates the URITemplate upfront so callers can surface a wrapped error instead of letting
// the SDK panic during registration on hot reload.
func ServerResourceTemplateToGoSdkResourceTemplate(_ *Server, rt api.ServerResourceTemplate) (*mcp.ResourceTemplate, mcp.ResourceHandler, error) {
	if _, err := uritemplate.New(rt.ResourceTemplate.URITemplate); err != nil {
		return nil, nil, fmt.Errorf("invalid URITemplate %q: %w", rt.ResourceTemplate.URITemplate, err)
	}
	mcpTemplate := &mcp.ResourceTemplate{
		URITemplate: rt.ResourceTemplate.URITemplate,
		Name:        rt.ResourceTemplate.Name,
		Description: rt.ResourceTemplate.Description,
		MIMEType:    rt.ResourceTemplate.MIMEType,
	}
	handler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		content, err := rt.Handler(ctx, req.Params.URI)
		if err != nil {
			return nil, err
		}
		if content == nil {
			return nil, errors.New("resource template handler returned nil content")
		}
		if err := validateResourceContent(content); err != nil {
			return nil, err
		}
		mimeType := rt.ResourceTemplate.MIMEType
		if content.MIMEType != "" {
			mimeType = content.MIMEType
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: mimeType,
				Text:     content.Text,
				Blob:     content.Blob,
			}},
		}, nil
	}
	return mcpTemplate, handler, nil
}

// validateResourceContent enforces the api.ResourceContent invariant:
// exactly one of Text or Blob must be set.
func validateResourceContent(content *api.ResourceContent) error {
	hasText := content.Text != ""
	hasBlob := len(content.Blob) > 0
	if !hasText && !hasBlob {
		return errors.New("resource content must have either Text or Blob set, both are empty")
	}
	if hasText && hasBlob {
		return errors.New("resource content must have only one of Text or Blob set, both are set")
	}
	return nil
}
