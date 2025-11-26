package kiali

import (
	"context"
	"fmt"
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	name := config.Default().ToolsetKialiName
	if name != "" {
		return name
	}
	return "kiali"
}

func (t *Toolset) GetDescription() string {
	name := config.Default().ToolsetKialiName
	if name == "" {
		name = "kiali"
	}
	return fmt.Sprintf("Most common tools for managing %s, check the [%s documentation](https://github.com/containers/kubernetes-mcp-server/blob/main/docs/KIALI.md) for more details.", name, name)
}

func (t *Toolset) GetTools(o internalk8s.Openshift) []api.ServerTool {
	isOpenshift := o.IsOpenShift(context.Background())
	return slices.Concat(
		initGetMeshGraph(isOpenshift),
		initManageIstioConfig(isOpenshift),
		initGetResourceDetails(isOpenshift),
		initGetMetrics(isOpenshift),
		initLogs(isOpenshift),
		initGetTraces(isOpenshift),
	)
}

func init() {
	toolsets.Register(&Toolset{})
}
