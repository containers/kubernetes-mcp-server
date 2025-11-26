package kiali

import (
	"context"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOpenShiftTrue struct{}

func (fakeOpenShiftTrue) IsOpenShift(context.Context) bool { return true }

func TestKialiToolsUseOssmPrefixWhenOpenShift(t *testing.T) {
	var o kubernetes.Openshift = fakeOpenShiftTrue{}
	tools := (&Toolset{}).GetTools(o)

	require.NotEmpty(t, tools, "expected kiali toolset to expose tools")
	for _, tool := range tools {
		name := tool.Tool.Name
		assert.Truef(t, strings.HasPrefix(name, "ossm_"), "expected tool name to start with 'ossm_', got %q", name)
		assert.Falsef(t, strings.HasPrefix(name, "kiali_"), "did not expect tool name to start with 'kiali_' when OpenShift, got %q", name)
	}
}


