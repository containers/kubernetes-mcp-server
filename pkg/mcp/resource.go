package mcp

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// addResource registers a resource with the MCP server.
func addResource(server *mcp.Server, res api.ServerResource) {
	server.AddResource(
		&mcp.Resource{
			URI:         res.Resource.URI,
			Name:        res.Resource.Name,
			Description: res.Resource.Description,
			MIMEType:    res.Resource.MIMEType,
		},
		func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			content, err := res.Handler(ctx)
			if err != nil {
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
		},
	)
}

// addResourceTemplate registers a resource template with the MCP server.
func addResourceTemplate(server *mcp.Server, rt api.ServerResourceTemplate) {
	server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			URITemplate: rt.ResourceTemplate.URITemplate,
			Name:        rt.ResourceTemplate.Name,
			Description: rt.ResourceTemplate.Description,
			MIMEType:    rt.ResourceTemplate.MIMEType,
		},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			content, err := rt.Handler(ctx, req.Params.URI)
			if err != nil {
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
		},
	)
}
