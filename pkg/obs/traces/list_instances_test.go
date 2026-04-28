package traces

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListInstancesHandler_Success(t *testing.T) {
	fakeClient := newMockK8sClient(
		newTempoStack("ns1", "stack1", []string{"tenant-a", "tenant-b"}),
		newTempoStack("ns2", "stack2", []string{"tenant-c"}),
	)

	toolset := &Toolset{}
	output, err := toolset.ListInstancesHandler(ToolParams{
		context:       t.Context(),
		dynamicClient: fakeClient,
		config:        &Config{UseRoute: false},
	})
	require.NoError(t, err)
	require.Len(t, output.Instances, 2)

	inst := output.Instances[0]
	require.Equal(t, "ns1", inst.Namespace)
	require.Equal(t, "stack1", inst.Name)
	require.Equal(t, []string{"tenant-a", "tenant-b"}, inst.Tenants)
	require.Equal(t, "Ready", inst.Status)

	inst2 := output.Instances[1]
	require.Equal(t, "ns2", inst2.Namespace)
	require.Equal(t, "stack2", inst2.Name)
}
