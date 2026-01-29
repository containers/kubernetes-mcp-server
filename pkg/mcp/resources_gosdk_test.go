package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func TestToMcpAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations *api.ResourceAnnotations
		wantNil     bool
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			wantNil:     true,
		},
		{
			name: "with all fields",
			annotations: &api.ResourceAnnotations{
				Audience:     []string{"user", "assistant"},
				LastModified: "2025-01-15T10:00:00Z",
				Priority:     0.8,
			},
			wantNil: false,
		},
		{
			name: "with empty audience",
			annotations: &api.ResourceAnnotations{
				Audience:     []string{},
				LastModified: "2025-01-15T10:00:00Z",
				Priority:     0.5,
			},
			wantNil: false,
		},
		{
			name: "with nil audience",
			annotations: &api.ResourceAnnotations{
				Audience:     nil,
				LastModified: "",
				Priority:     0,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toMcpAnnotations(tt.annotations)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			assert.Equal(t, tt.annotations.LastModified, got.LastModified)
			assert.Equal(t, tt.annotations.Priority, got.Priority)
			assert.Len(t, got.Audience, len(tt.annotations.Audience))
		})
	}
}

func TestToMcpReadResourceResult(t *testing.T) {
	tests := []struct {
		name   string
		result *api.ResourceCallResult
		want   int // expected number of contents
	}{
		{
			name:   "nil result",
			result: nil,
			want:   0,
		},
		{
			name: "empty contents",
			result: &api.ResourceCallResult{
				Contents: []*api.ResourceContents{},
			},
			want: 0,
		},
		{
			name: "single text content",
			result: &api.ResourceCallResult{
				Contents: []*api.ResourceContents{
					{
						URI:      "k8s://pods/default/my-pod",
						MIMEType: "application/json",
						Text:     `{"kind":"Pod"}`,
					},
				},
			},
			want: 1,
		},
		{
			name: "single binary content",
			result: &api.ResourceCallResult{
				Contents: []*api.ResourceContents{
					{
						URI:      "k8s://secrets/default/my-secret",
						MIMEType: "application/octet-stream",
						Blob:     []byte{0x01, 0x02, 0x03},
					},
				},
			},
			want: 1,
		},
		{
			name: "multiple contents",
			result: &api.ResourceCallResult{
				Contents: []*api.ResourceContents{
					{
						URI:      "k8s://pods/default/pod1",
						MIMEType: "application/json",
						Text:     `{"name":"pod1"}`,
					},
					{
						URI:      "k8s://pods/default/pod2",
						MIMEType: "application/json",
						Text:     `{"name":"pod2"}`,
					},
				},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toMcpReadResourceResult(tt.result)

			require.NotNil(t, got)
			assert.Len(t, got.Contents, tt.want)

			if tt.result != nil && len(tt.result.Contents) > 0 {
				for i, content := range tt.result.Contents {
					assert.Equal(t, content.URI, got.Contents[i].URI)
					assert.Equal(t, content.MIMEType, got.Contents[i].MIMEType)
					assert.Equal(t, content.Text, got.Contents[i].Text)
					assert.Equal(t, content.Blob, got.Contents[i].Blob)
				}
			}
		})
	}
}

func TestServerResourceToGoSdkResource_Conversion(t *testing.T) {
	serverResource := api.ServerResource{
		Resource: api.Resource{
			Name:        "test-resource",
			Description: "Test resource description",
			Title:       "Test Resource",
			URI:         "k8s://pods/default/test-pod",
			MIMEType:    "application/json",
			Size:        1024,
			Annotations: &api.ResourceAnnotations{
				Audience:     []string{"user"},
				LastModified: "2025-01-15T10:00:00Z",
				Priority:     0.9,
			},
		},
		Handler: func(params api.ResourceHandlerParams) (*api.ResourceCallResult, error) {
			return api.NewResourceTextResult(params.URI, "application/json", `{"kind":"Pod"}`), nil
		},
	}

	mockServer := &Server{}

	mcpResource, handler, err := ServerResourceToGoSdkResource(mockServer, serverResource)

	require.NoError(t, err)
	require.NotNil(t, mcpResource)
	require.NotNil(t, handler)

	assert.Equal(t, "test-resource", mcpResource.Name)
	assert.Equal(t, "Test resource description", mcpResource.Description)
	assert.Equal(t, "Test Resource", mcpResource.Title)
	assert.Equal(t, "k8s://pods/default/test-pod", mcpResource.URI)
	assert.Equal(t, "application/json", mcpResource.MIMEType)
	assert.Equal(t, int64(1024), mcpResource.Size)

	require.NotNil(t, mcpResource.Annotations)
	assert.Len(t, mcpResource.Annotations.Audience, 1)
	assert.Equal(t, "2025-01-15T10:00:00Z", mcpResource.Annotations.LastModified)
	assert.Equal(t, 0.9, mcpResource.Annotations.Priority)
}

func TestServerResourceToGoSdkResource_NilAnnotations(t *testing.T) {
	serverResource := api.ServerResource{
		Resource: api.Resource{
			Name:        "simple-resource",
			Description: "Resource without annotations",
			URI:         "k8s://configmaps/default/config",
			MIMEType:    "application/json",
			Annotations: nil,
		},
		Handler: func(params api.ResourceHandlerParams) (*api.ResourceCallResult, error) {
			return api.NewResourceTextResult(params.URI, "application/json", `{}`), nil
		},
	}

	mockServer := &Server{}

	mcpResource, handler, err := ServerResourceToGoSdkResource(mockServer, serverResource)

	require.NoError(t, err)
	require.NotNil(t, mcpResource)
	require.NotNil(t, handler)

	assert.Equal(t, "simple-resource", mcpResource.Name)
	assert.Nil(t, mcpResource.Annotations)
}

func TestServerResourceTemplateToGoSdkResourceTemplate_Conversion(t *testing.T) {
	serverTemplate := api.ServerResourceTemplate{
		ResourceTemplate: api.ResourceTemplate{
			Name:        "test-template",
			Description: "Test template description",
			Title:       "Test Template",
			URITemplate: "k8s://pods/{namespace}/{name}",
			MIMEType:    "application/json",
			Annotations: &api.ResourceAnnotations{
				Audience:     []string{"user", "assistant"},
				LastModified: "2025-01-15T12:00:00Z",
				Priority:     0.7,
			},
		},
		Handler: func(params api.ResourceHandlerParams) (*api.ResourceCallResult, error) {
			return api.NewResourceTextResult(params.URI, "application/json", `{"kind":"Pod"}`), nil
		},
	}

	mockServer := &Server{}

	mcpTemplate, handler, err := ServerResourceTemplateToGoSdkResourceTemplate(mockServer, serverTemplate)

	require.NoError(t, err)
	require.NotNil(t, mcpTemplate)
	require.NotNil(t, handler)

	assert.Equal(t, "test-template", mcpTemplate.Name)
	assert.Equal(t, "Test template description", mcpTemplate.Description)
	assert.Equal(t, "Test Template", mcpTemplate.Title)
	assert.Equal(t, "k8s://pods/{namespace}/{name}", mcpTemplate.URITemplate)
	assert.Equal(t, "application/json", mcpTemplate.MIMEType)

	require.NotNil(t, mcpTemplate.Annotations)
	assert.Len(t, mcpTemplate.Annotations.Audience, 2)
	assert.Equal(t, "2025-01-15T12:00:00Z", mcpTemplate.Annotations.LastModified)
	assert.Equal(t, 0.7, mcpTemplate.Annotations.Priority)
}

func TestServerResourceTemplateToGoSdkResourceTemplate_NilAnnotations(t *testing.T) {
	serverTemplate := api.ServerResourceTemplate{
		ResourceTemplate: api.ResourceTemplate{
			Name:        "simple-template",
			Description: "Template without annotations",
			URITemplate: "k8s://services/{namespace}/{name}",
			MIMEType:    "application/json",
			Annotations: nil,
		},
		Handler: func(params api.ResourceHandlerParams) (*api.ResourceCallResult, error) {
			return api.NewResourceTextResult(params.URI, "application/json", `{}`), nil
		},
	}

	mockServer := &Server{}

	mcpTemplate, handler, err := ServerResourceTemplateToGoSdkResourceTemplate(mockServer, serverTemplate)

	require.NoError(t, err)
	require.NotNil(t, mcpTemplate)
	require.NotNil(t, handler)

	assert.Equal(t, "simple-template", mcpTemplate.Name)
	assert.Nil(t, mcpTemplate.Annotations)
}
