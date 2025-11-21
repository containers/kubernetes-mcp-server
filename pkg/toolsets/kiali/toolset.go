package kiali

import (
	"context"
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "kiali"
}

func (t *Toolset) GetDescription() string {
	return "Most common tools for managing Kiali, check the [Kiali integration documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/KIALI_INTEGRATION.md) for more details."
}

func (t *Toolset) GetTools(_ internalk8s.Openshift) []api.ServerTool {
	return slices.Concat(
		initGraph(),
		initMeshStatus(),
		initIstioConfig(),
		initIstioObjectDetails(),
		initIstioObjectPatch(),
		initIstioObjectCreate(),
		initIstioObjectDelete(),
		initValidations(),
		initNamespaces(),
		initServices(),
		initWorkloads(),
		initHealth(),
		initLogs(),
		initTraces(),
	)
}

func (t *Toolset) IsValid(k *internalk8s.Kubernetes) bool {
	// Create a Kiali client
	kialiClient := k.NewKiali()

	// Check if Kiali is actually accessible by making a lightweight API call
	// We'll try to get the mesh status as it's a simple endpoint that doesn't require parameters
	ctx := context.Background()
	_, err := kialiClient.MeshStatus(ctx)

	// If we can successfully call Kiali, it's valid
	return err == nil
}

func init() {
	toolsets.Register(&Toolset{})
}
