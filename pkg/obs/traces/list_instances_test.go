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

	result, err := listInstancesHandler(newTestParams(t, &Config{UseRoute: false}, fakeClient, nil))
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(listInstancesOutput)
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

func TestListInstancesHandler_TempoMonolithic(t *testing.T) {
	fakeClient := newMockK8sClient(
		newTempoMonolithic("mono-ns", "mono1", []string{}),
	)

	result, err := listInstancesHandler(newTestParams(t, &Config{UseRoute: false}, fakeClient, nil))
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(listInstancesOutput)
	require.Len(t, output.Instances, 1)

	inst := output.Instances[0]
	require.Equal(t, "mono-ns", inst.Namespace)
	require.Equal(t, "mono1", inst.Name)
	require.False(t, inst.Multitenancy)
	require.Empty(t, inst.Tenants)
	require.Equal(t, "Ready", inst.Status)
}

func TestListInstancesHandler_TempoMonolithic_Multitenancy(t *testing.T) {
	fakeClient := newMockK8sClient(
		newTempoMonolithic("mono-ns", "mono-mt", []string{"dev", "prod"}),
	)

	result, err := listInstancesHandler(newTestParams(t, &Config{UseRoute: false}, fakeClient, nil))
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(listInstancesOutput)
	require.Len(t, output.Instances, 1)

	inst := output.Instances[0]
	require.Equal(t, "mono-mt", inst.Name)
	require.True(t, inst.Multitenancy)
	require.Equal(t, []string{"dev", "prod"}, inst.Tenants)
}

func TestListInstancesHandler_MixedInstances(t *testing.T) {
	fakeClient := newMockK8sClient(
		newTempoStack("tracing", "stack1", []string{}),
		newTempoMonolithic("tracing", "mono1", []string{}),
	)

	result, err := listInstancesHandler(newTestParams(t, &Config{UseRoute: false}, fakeClient, nil))
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(listInstancesOutput)
	require.Len(t, output.Instances, 2)

	// TempoStacks are listed first, then TempoMonolithics
	require.Equal(t, "stack1", output.Instances[0].Name)
	require.Equal(t, "mono1", output.Instances[1].Name)
}
