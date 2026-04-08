package kubernetes

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestWindowsEULAValidator(t *testing.T) {
	validator := &WindowsEULAValidator{}

	t.Run("allows non-PipelineRun resources", func(t *testing.T) {
		req := &api.HTTPValidationRequest{
			GVK:  &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Verb: "create",
			Body: []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test-pod
spec:
  containers:
  - name: test
    image: nginx`),
		}
		err := validator.Validate(context.Background(), req)
		assert.NoError(t, err)
	})

	t.Run("allows non-windows PipelineRuns", func(t *testing.T) {
		req := &api.HTTPValidationRequest{
			GVK:  &schema.GroupVersionKind{Group: "tekton.dev", Version: "v1", Kind: "PipelineRun"},
			Verb: "create",
			Body: []byte(`apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: other-pipeline
spec:
  pipelineRef:
    resolver: hub
    params:
    - name: name
      value: some-other-pipeline`),
		}
		err := validator.Validate(context.Background(), req)
		assert.NoError(t, err)
	})

	t.Run("allows windows-efi-installer with acceptEula true", func(t *testing.T) {
		req := &api.HTTPValidationRequest{
			GVK:  &schema.GroupVersionKind{Group: "tekton.dev", Version: "v1", Kind: "PipelineRun"},
			Verb: "create",
			Body: []byte(`apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: windows2k22-installer-run-
spec:
  params:
  - name: winImageDownloadURL
    value: https://example.com/win.iso
  - name: acceptEula
    value: "true"
  pipelineRef:
    resolver: hub
    params:
    - name: catalog
      value: kubevirt-tekton-pipelines
    - name: name
      value: windows-efi-installer`),
		}
		err := validator.Validate(context.Background(), req)
		assert.NoError(t, err)
	})

	t.Run("validates server-side apply (PATCH) of windows-efi-installer", func(t *testing.T) {
		req := &api.HTTPValidationRequest{
			GVK:  &schema.GroupVersionKind{Group: "tekton.dev", Version: "v1", Kind: "PipelineRun"},
			Verb: "patch",
			Body: []byte(`apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: windows2k22-installer-run-
spec:
  params:
  - name: acceptEula
    value: "false"
  pipelineRef:
    resolver: hub
    params:
    - name: name
      value: windows-efi-installer`),
		}
		err := validator.Validate(context.Background(), req)
		assert.Error(t, err)
		var validationErr *api.ValidationError
		assert.ErrorAs(t, err, &validationErr)
		assert.Equal(t, api.ErrorCodePermissionDenied, validationErr.Code)
		assert.Contains(t, validationErr.Message, "EULA")
	})

	t.Run("skips non-apply patch bodies without error", func(t *testing.T) {
		req := &api.HTTPValidationRequest{
			GVK:  &schema.GroupVersionKind{Group: "tekton.dev", Version: "v1", Kind: "PipelineRun"},
			Verb: "patch",
			Body: []byte(`[{"op":"replace","path":"/spec/params/0/value","value":"true"}]`),
		}
		err := validator.Validate(context.Background(), req)
		assert.NoError(t, err)
	})

	t.Run("rejects windows-efi-installer without acceptEula when no session", func(t *testing.T) {
		req := &api.HTTPValidationRequest{
			GVK:  &schema.GroupVersionKind{Group: "tekton.dev", Version: "v1", Kind: "PipelineRun"},
			Verb: "create",
			Body: []byte(`apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: windows2k22-installer-run-
spec:
  params:
  - name: winImageDownloadURL
    value: https://example.com/win.iso
  - name: acceptEula
    value: "false"
  pipelineRef:
    resolver: hub
    params:
    - name: catalog
      value: kubevirt-tekton-pipelines
    - name: name
      value: windows-efi-installer`),
		}
		err := validator.Validate(context.Background(), req)
		assert.Error(t, err)
		var validationErr *api.ValidationError
		assert.ErrorAs(t, err, &validationErr)
		assert.Equal(t, api.ErrorCodePermissionDenied, validationErr.Code)
		assert.Contains(t, validationErr.Message, "EULA")
	})

	t.Run("prompts user when acceptEula is false and session available", func(t *testing.T) {
		mockSession := &mockMCPSession{shouldAccept: true}
		ctx := context.WithValue(context.Background(), mcplog.MCPSessionContextKey, mockSession)

		req := &api.HTTPValidationRequest{
			GVK:  &schema.GroupVersionKind{Group: "tekton.dev", Version: "v1", Kind: "PipelineRun"},
			Verb: "create",
			Body: []byte(`apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: windows2k22-installer-run-
spec:
  params:
  - name: winImageDownloadURL
    value: https://example.com/win.iso
  - name: acceptEula
    value: "false"
  pipelineRef:
    resolver: hub
    params:
    - name: catalog
      value: kubevirt-tekton-pipelines
    - name: name
      value: windows-efi-installer`),
		}

		err := validator.Validate(ctx, req)
		assert.NoError(t, err)
		assert.True(t, mockSession.elicitCalled)
		assert.Contains(t, mockSession.lastMessage, "Microsoft EULA")
	})

	t.Run("rejects when user declines EULA", func(t *testing.T) {
		mockSession := &mockMCPSession{shouldAccept: false}
		ctx := context.WithValue(context.Background(), mcplog.MCPSessionContextKey, mockSession)

		req := &api.HTTPValidationRequest{
			GVK:  &schema.GroupVersionKind{Group: "tekton.dev", Version: "v1", Kind: "PipelineRun"},
			Verb: "create",
			Body: []byte(`apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: windows2k22-installer-run-
spec:
  params:
  - name: winImageDownloadURL
    value: https://example.com/win.iso
  - name: acceptEula
    value: "false"
  pipelineRef:
    resolver: hub
    params:
    - name: catalog
      value: kubevirt-tekton-pipelines
    - name: name
      value: windows-efi-installer`),
		}

		err := validator.Validate(ctx, req)
		assert.Error(t, err)
		var validationErr *api.ValidationError
		assert.ErrorAs(t, err, &validationErr)
		assert.Equal(t, api.ErrorCodePermissionDenied, validationErr.Code)
		assert.Contains(t, validationErr.Message, "not accepted")
	})
}

// mockMCPSession is a test double for mcp.ServerSession
// We only need to implement Elicit for these tests
type mockMCPSession struct {
	shouldAccept bool
	elicitCalled bool
	lastMessage  string
}

func (m *mockMCPSession) Elicit(_ context.Context, params *mcp.ElicitParams) (*mcp.ElicitResult, error) {
	m.elicitCalled = true
	m.lastMessage = params.Message
	if m.shouldAccept {
		return &mcp.ElicitResult{Action: api.ElicitActionAccept}, nil
	}
	return &mcp.ElicitResult{Action: api.ElicitActionDecline}, nil
}

// Type assertion check - compile-time verification that mockMCPSession can be used where *mcp.ServerSession is expected
// This won't compile if mockMCPSession doesn't have the methods that windowsEULAValidator actually calls
var _ interface {
	Elicit(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error)
} = (*mockMCPSession)(nil)
