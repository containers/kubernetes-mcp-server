package kubevirt

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	vm_create "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/create"
	vm_start "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/start"
	vm_stop "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/stop"
	vm_troubleshoot "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/troubleshoot"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "kubevirt"
}

func (t *Toolset) GetDescription() string {
	return "KubeVirt virtual machine management tools"
}

func (t *Toolset) GetTools(o internalk8s.Openshift) []api.ServerTool {
	return slices.Concat(
		vm_create.Tools(),
		vm_start.Tools(),
		vm_stop.Tools(),
		vm_troubleshoot.Tools(),
	)
}

func init() {
	toolsets.Register(&Toolset{})
}
