package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestServerPrompt_IsClusterAware(t *testing.T) {
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
			sp := &ServerPrompt{
				ClusterAware: tt.clusterAware,
			}
			assert.Equal(t, tt.want, sp.IsClusterAware())
		})
	}
}

func TestNewPromptCallResult(t *testing.T) {
	tests := []struct {
		name        string
		description string
		messages    []PromptMessage
		err         error
	}{
		{
			name:        "successful result",
			description: "Test description",
			messages: []PromptMessage{
				{
					Role: "user",
					Content: PromptContent{
						Type: "text",
						Text: "Hello",
					},
				},
			},
			err: nil,
		},
		{
			name:        "result with error",
			description: "Error description",
			messages:    nil,
			err:         assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewPromptCallResult(tt.description, tt.messages, tt.err)
			assert.Equal(t, tt.description, result.Description)
			assert.Equal(t, tt.messages, result.Messages)
			assert.Equal(t, tt.err, result.Error)
		})
	}
}
