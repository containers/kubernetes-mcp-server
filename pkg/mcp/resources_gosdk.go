package mcp

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerResourceToGoSdkResource converts an api.ServerResource to MCP SDK types
func ServerResourceToGoSdkResource(s *Server, resource api.ServerResource) (*mcp.Resource, mcp.ResourceHandler, error) {
	goSdkResource := &mcp.Resource{
		Name:        resource.Resource.Name,
		Description: resource.Resource.Description,
		Title:       resource.Resource.Title,
		URI:         resource.Resource.URI,
		MIMEType:    resource.Resource.MIMEType,
		Size:        resource.Resource.Size,
		Annotations: toMcpAnnotations(resource.Resource.Annotations),
	}

	return goSdkResource, newResourceHandler(s, resource.Handler), nil
}

// ServerResourceTemplateToGoSdkResourceTemplate converts an api.ServerResourceTemplate to MCP SDK types
func ServerResourceTemplateToGoSdkResourceTemplate(s *Server, template api.ServerResourceTemplate) (*mcp.ResourceTemplate, mcp.ResourceHandler, error) {
	goSdkTemplate := &mcp.ResourceTemplate{
		Name:        template.ResourceTemplate.Name,
		Description: template.ResourceTemplate.Description,
		Title:       template.ResourceTemplate.Title,
		URITemplate: template.ResourceTemplate.URITemplate,
		MIMEType:    template.ResourceTemplate.MIMEType,
		Annotations: toMcpAnnotations(template.ResourceTemplate.Annotations),
	}

	return goSdkTemplate, newResourceHandler(s, template.Handler), nil
}

// newResourceHandler creates a common resource handler for both resources and resource templates
func newResourceHandler(s *Server, handler api.ResourceHandlerFunc) mcp.ResourceHandler {
	return func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := ""
		if request.Params != nil {
			uri = request.Params.URI
		}

		// Get the Kubernetes client using the default target
		// Resources don't carry cluster information in the URI
		// TODO: revisit this, as things may differ cluster to cluster
		k, err := s.p.GetDerivedKubernetes(ctx, s.p.GetDefaultTarget())
		if err != nil {
			return nil, err
		}

		result, err := handler(api.ResourceHandlerParams{
			Context:                ctx,
			ExtendedConfigProvider: s.configuration,
			KubernetesClient:       k,
			URI:                    uri,
		})
		if err != nil {
			return nil, err
		}

		return toMcpReadResourceResult(result), nil
	}
}

// toMcpReadResourceResult converts an api.ResourceCallResult to MCP SDK ReadResourceResult
func toMcpReadResourceResult(result *api.ResourceCallResult) *mcp.ReadResourceResult {
	if result == nil {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{},
		}
	}

	contents := make([]*mcp.ResourceContents, 0, len(result.Contents))
	for _, c := range result.Contents {
		contents = append(contents, &mcp.ResourceContents{
			URI:      c.URI,
			MIMEType: c.MIMEType,
			Text:     c.Text,
			Blob:     c.Blob,
		})
	}

	return &mcp.ReadResourceResult{
		Contents: contents,
	}
}

// toMcpAnnotations converts api.ResourceAnnotations to MCP SDK Annotations
func toMcpAnnotations(annotations *api.ResourceAnnotations) *mcp.Annotations {
	if annotations == nil {
		return nil
	}

	var roles []mcp.Role
	for _, a := range annotations.Audience {
		roles = append(roles, mcp.Role(a))
	}

	return &mcp.Annotations{
		Audience:     roles,
		LastModified: annotations.LastModified,
		Priority:     annotations.Priority,
	}
}
