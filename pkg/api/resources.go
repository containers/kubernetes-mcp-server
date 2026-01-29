package api

import "context"

type ServerResource struct {
	Resource     Resource
	Handler      ResourceHandlerFunc
	ClusterAware *bool
}

func (sr *ServerResource) IsClusterAware() bool {
	if sr.ClusterAware != nil {
		return *sr.ClusterAware
	}
	return true
}

type ServerResourceTemplate struct {
	ResourceTemplate ResourceTemplate
	Handler          ResourceHandlerFunc
	ClusterAware     *bool
}

func (srt *ServerResourceTemplate) IsClusterAware() bool {
	if srt.ClusterAware != nil {
		return *srt.ClusterAware
	}
	return true
}

type ResourceHandlerFunc func(params ResourceHandlerParams) (*ResourceCallResult, error)

type ResourceHandlerParams struct {
	context.Context
	ExtendedConfigProvider
	KubernetesClient
	URI string
}

type ResourceCallResult struct {
	Contents []*ResourceContents
}

type Resource struct {
	// Optional annotations for the client
	Annotations *ResourceAnnotations
	// A description of what this resource represents.
	//
	// This can be used by clients to improve the LLM's understanding of available
	// resources.
	Description string
	// The MIME type of this resource, if known
	MIMEType string
	// Name of the resource
	Name string
	// The size of the raw resource content, in bytes, if known
	Size int64
	// Human readable title, if not provided the name will be used to display to users
	Title string
	// The URI of this resource
	URI string
}

type ResourceTemplate struct {
	// Optional annotations for the client
	Annotations *ResourceAnnotations
	// A description of what this resource represents.
	//
	// This can be used by clients to improve the LLM's understanding of available
	// resources.
	Description string
	// The MIME type of this resource, if known
	MIMEType string
	// Name of the resource
	Name string
	// The size of the raw resource content, in bytes, if known
	Size int64
	// Human readable title, if not provided the name will be used to display to users
	Title string
	// A URI template (according to RFC 6570) that can be used to construct resource URIs
	URITemplate string
}

func NewResourceTextResult(uri, mimeType, text string) *ResourceCallResult {
	return &ResourceCallResult{
		Contents: []*ResourceContents{{
			URI:      uri,
			MIMEType: mimeType,
			Text:     text,
		}},
	}
}

func NewResourceBinaryResult(uri, mimeType string, blob []byte) *ResourceCallResult {
	return &ResourceCallResult{
		Contents: []*ResourceContents{{
			URI:      uri,
			MIMEType: mimeType,
			Blob:     blob,
		}},
	}
}

type ResourceAnnotations struct {
	// Described who the intended customer of this object or data is

	// It can include multiple entries to indicate content useful for multiple
	// audiences, (e.g. []string{"user", "assistant"}).
	Audience []string `json:"audience,omitempty"`
	// The moment the resource was last modified, as an ISO 8601 formatted string.
	//
	// Examples: last activity timestamp in an open file
	LastModified string `json:"lastModified,omitempty"`
	// Describes how important this data is for operating the server.
	//
	// A value of 1 means "most important", and indicates that the data is
	// effectively required, while 0 means "least important", and indicates
	// that the data is entirely optional.
	Priority float64 `json:"priority,omitempty"`
}

type ResourceContents struct {
	URI      string
	MIMEType string
	Text     string
	Blob     []byte
}
