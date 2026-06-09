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
func ServerResourceToGoSdkResource(s *Server, res api.ServerResource) (*mcp.Resource, mcp.ResourceHandler, error) {
	if _, err := url.Parse(res.Resource.URI); err != nil {
		return nil, nil, fmt.Errorf("invalid URI %q: %w", res.Resource.URI, err)
	}
	mcpResource := &mcp.Resource{
		URI:         res.Resource.URI,
		Name:        res.Resource.Name,
		Description: res.Resource.Description,
		MIMEType:    res.Resource.MIMEType,
		Annotations: buildAnnotations(
			res.Resource.Annotations.Audience,
			res.Resource.Annotations.Priority,
			res.Resource.Annotations.LastModified,
		),
		Title: res.Resource.Title,
	}

	handler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		cfg := s.configuration.Load()

		cluster, err := extractClusterNameFromResourceURI(req.Params.URI)
		if err != nil {
			return nil, err
		}

		k, err := s.p.GetDerivedKubernetes(ctx, cluster)
		if err != nil {
			return nil, err
		}

		content, err := res.Handler(api.ResourceHandlerParams{
			Context:          ctx,
			BaseConfig:       cfg,
			KubernetesClient: k,
		})

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
		Annotations: buildAnnotations(
			rt.ResourceTemplate.Annotations.Audience,
			rt.ResourceTemplate.Annotations.Priority,
			rt.ResourceTemplate.Annotations.LastModified,
		),
		Title: rt.ResourceTemplate.Title,
	}
	handler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		content, err := rt.Handler(api.ResourceHandlerParams{Context: ctx, URI: req.Params.URI})
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

// buildAnnotations converts internal annotation fields to Go SDK annotations
// returns nil if all fields are empty
func buildAnnotations(audience []string, priority *float64, lastModified *string) *mcp.Annotations {
	if len(audience) == 0 && priority == nil && lastModified == nil {
		return nil
	}

	annotations := &mcp.Annotations{}

	if len(audience) > 0 {
		annotations.Audience = make([]mcp.Role, len(audience))
		for i, a := range audience {
			annotations.Audience[i] = mcp.Role(a)
		}
	}

	if priority != nil {
		annotations.Priority = *priority
	}

	if lastModified != nil {
		annotations.LastModified = *lastModified
	}

	return annotations
}

// returns the cluster name by parsing a resource URI, who's Host field should be the name of the cluster
// ex: k8s://cluster-name/
func extractClusterNameFromResourceURI(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	if u.Host != "" {
		return u.Host, nil
	}

	return "", errors.New("Resource URI has invalid Host (cluster name)!")
}
