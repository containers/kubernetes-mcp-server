package console

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"

	"k8s.io/utils/ptr"
)

func Tools(ctx context.Context, inspector api.ClusterInspector) []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_console_screenshot",
				Description: fmt.Sprintf("Capture a screenshot of the graphical (VNC) console of a VirtualMachine on %s as a PNG image. Use this to see what is currently displayed on the VM's screen (for example firmware, a bootloader, or a login prompt). Read-only: it never sends input to the guest.", defaults.ProductName()),
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the virtual machine",
						},
						"name": {
							Type:        "string",
							Description: "The name of the virtual machine",
						},
					},
					Required: []string{"namespace", "name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Console Screenshot",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			RBAC: api.RBACBounded(
				api.RBACRequirement{
					Verbs:        []string{"get"},
					Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "kubevirt.io", Resource: "virtualmachineinstances"}},
					Namespace:    &api.RBACNamespace{Argument: "namespace"},
					ResourceName: &api.RBACResourceName{Argument: "name"},
				},
				api.RBACRequirement{
					Verbs:        []string{"get"},
					Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "subresources.kubevirt.io", Resource: "virtualmachineinstances", Subresource: "vnc/screenshot"}},
					Namespace:    &api.RBACNamespace{Argument: "namespace"},
					ResourceName: &api.RBACResourceName{Argument: "name"},
				},
			),
			Handler: screenshot,
			TargetCompatibilityFilters: []func() bool{
				kubevirt.HasVirtualMachine(ctx, inspector),
			},
		},
	}
}

func screenshot(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	namespace := p.RequiredString("namespace")
	name := p.RequiredString("name")

	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	pngBytes, cfg, err := kubevirt.Screenshot(
		params.Context,
		params.DynamicClient(),
		params.RESTConfig(),
		namespace,
		name,
	)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	description := fmt.Sprintf(
		"Screenshot captured from the graphical (VNC) console of VirtualMachine %q in namespace %q (%dx%d PNG, %d bytes).",
		name, namespace, cfg.Width, cfg.Height, len(pngBytes),
	)

	return api.NewToolCallResultContentBlocks(
		api.NewTextToolContent(description),
		api.NewImageToolContent(pngBytes, "image/png"),
	), nil
}
