package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestServerResource_IsClusterAware(t *testing.T) {
	tests := []struct {
		name         string
		clusterAware *bool
		want         bool
	}{
		{
			name:         "nil defaults to true",
			clusterAware: nil,
			want:         true,
		},
		{
			name:         "explicitly true",
			clusterAware: ptr.To(true),
			want:         true,
		},
		{
			name:         "explicitly false",
			clusterAware: ptr.To(false),
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr := &ServerResource{
				ClusterAware: tt.clusterAware,
			}
			assert.Equal(t, tt.want, sr.IsClusterAware())
		})
	}
}

func TestServerResourceTemplate_IsClusterAware(t *testing.T) {
	tests := []struct {
		name         string
		clusterAware *bool
		want         bool
	}{
		{
			name:         "nil defaults to true",
			clusterAware: nil,
			want:         true,
		},
		{
			name:         "explicitly true",
			clusterAware: ptr.To(true),
			want:         true,
		},
		{
			name:         "explicitly false",
			clusterAware: ptr.To(false),
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srt := &ServerResourceTemplate{
				ClusterAware: tt.clusterAware,
			}
			assert.Equal(t, tt.want, srt.IsClusterAware())
		})
	}
}

func TestNewResourceTextResult(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		mimeType string
		text     string
	}{
		{
			name:     "simple text resource",
			uri:      "file:///test.txt",
			mimeType: "text/plain",
			text:     "Hello, World!",
		},
		{
			name:     "json resource",
			uri:      "k8s://pods/default/my-pod",
			mimeType: "application/json",
			text:     `{"kind":"Pod","metadata":{"name":"my-pod"}}`,
		},
		{
			name:     "empty text",
			uri:      "file:///empty.txt",
			mimeType: "text/plain",
			text:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewResourceTextResult(tt.uri, tt.mimeType, tt.text)
			assert.NotNil(t, result)
			assert.Len(t, result.Contents, 1)
			assert.Equal(t, tt.uri, result.Contents[0].URI)
			assert.Equal(t, tt.mimeType, result.Contents[0].MIMEType)
			assert.Equal(t, tt.text, result.Contents[0].Text)
			assert.Nil(t, result.Contents[0].Blob)
		})
	}
}

func TestNewResourceBinaryResult(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		mimeType string
		blob     []byte
	}{
		{
			name:     "binary image",
			uri:      "file:///image.png",
			mimeType: "image/png",
			blob:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
		},
		{
			name:     "binary data",
			uri:      "k8s://secrets/default/my-secret",
			mimeType: "application/octet-stream",
			blob:     []byte{0x01, 0x02, 0x03, 0x04},
		},
		{
			name:     "empty blob",
			uri:      "file:///empty.bin",
			mimeType: "application/octet-stream",
			blob:     []byte{},
		},
		{
			name:     "nil blob",
			uri:      "file:///nil.bin",
			mimeType: "application/octet-stream",
			blob:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewResourceBinaryResult(tt.uri, tt.mimeType, tt.blob)
			assert.NotNil(t, result)
			assert.Len(t, result.Contents, 1)
			assert.Equal(t, tt.uri, result.Contents[0].URI)
			assert.Equal(t, tt.mimeType, result.Contents[0].MIMEType)
			assert.Equal(t, tt.blob, result.Contents[0].Blob)
			assert.Empty(t, result.Contents[0].Text)
		})
	}
}
