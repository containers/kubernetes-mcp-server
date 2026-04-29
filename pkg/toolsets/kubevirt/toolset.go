package kubevirt

import (
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	kubevirtdefaults "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
	vm_clone "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/clone"
	vm_create "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/create"
	vm_lifecycle "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/lifecycle"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "kubevirt"
}

func (t *Toolset) GetDescription() string {
	return kubevirtdefaults.ToolsetDescription()
}

func (t *Toolset) GetTools(_ api.Openshift) []api.ServerTool {
	return slices.Concat(
		vm_clone.Tools(),
		vm_create.Tools(),
		vm_lifecycle.Tools(),
	)
}

func (t *Toolset) GetPrompts() []api.ServerPrompt {
	return initVMTroubleshoot()
}

func (t *Toolset) GetResources() []api.ServerResource {
	return nil
}

func (t *Toolset) GetResourceTemplates() []api.ServerResourceTemplate {
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
