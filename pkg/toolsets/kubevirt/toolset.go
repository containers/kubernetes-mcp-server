package kubevirt

import (
	"context"
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	kubevirtdefaults "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
	vm_clone "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/clone"
	vm_console "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/console"
	vm_create "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/create"
	vm_guestagent "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/guestagent"
	vm_lifecycle "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/lifecycle"
	vm_template "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/template"
	vm_troubleshoot "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/vm/troubleshoot"
)

type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

func (t *Toolset) GetName() string {
	return "kubevirt"
}

func (t *Toolset) GetDescription() string {
	return kubevirtdefaults.ToolsetDescription()
}

func (t *Toolset) GetTools(ctx context.Context, toolsetContext api.ToolsetContext) []api.ServerTool {
	return slices.Concat(
		vm_clone.Tools(ctx, toolsetContext.Inspector),
		vm_console.Tools(ctx, toolsetContext.Inspector),
		vm_create.Tools(ctx, toolsetContext.Inspector),
		vm_guestagent.Tools(ctx, toolsetContext.Inspector),
		vm_lifecycle.Tools(ctx, toolsetContext.Inspector),
		vm_template.Tools(ctx, toolsetContext.Inspector),
		vm_troubleshoot.Tools(ctx, toolsetContext.Inspector),
	)
}

func (t *Toolset) GetPrompts(_ context.Context, _ api.ToolsetContext) []api.ServerPrompt {
	return slices.Concat(
		initVMTroubleshoot(),
		initWindowsGoldenImage(),
		initHCOStatus(),
	)
}

func (t *Toolset) GetResources(_ context.Context, _ api.ToolsetContext) []api.ServerResource {
	return nil
}

func (t *Toolset) GetResourceTemplates(_ context.Context, _ api.ToolsetContext) []api.ServerResourceTemplate {
	return nil
}

func init() {
	toolsets.Register(&Toolset{})
}
