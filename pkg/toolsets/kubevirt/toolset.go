package kubevirt

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	vm_create "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/create"
	vm_lifecycle "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/lifecycle"
	vm_snapshot_create "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/snapshot/create"
	vm_snapshot_delete "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/snapshot/delete"
	vm_snapshot_list "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/snapshot/list"
	vm_snapshot_restore "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/snapshot/restore"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "kubevirt"
}

func (t *Toolset) GetDescription() string {
	return "KubeVirt virtual machine management tools"
}

func (t *Toolset) GetTools(_ api.Openshift) []api.ServerTool {
	return slices.Concat(
		vm_create.Tools(),
		vm_lifecycle.Tools(),
		vm_snapshot_create.Tools(),
		vm_snapshot_delete.Tools(),
		vm_snapshot_list.Tools(),
		vm_snapshot_restore.Tools(),
	)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	return slices.Concat(
		initVMTroubleshoot(),
	)
}

func init() {
	toolsets.Register(&Toolset{})
}
