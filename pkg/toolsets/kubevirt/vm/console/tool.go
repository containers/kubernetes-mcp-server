package console

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"
)

func Tools(p api.FilteringProvider) []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_console_screenshot",
				Description: fmt.Sprintf("Capture a screenshot of a %s VirtualMachine's graphical (VNC) console as a PNG image. Use this to see what is currently displayed on the VM's screen (for example firmware, a bootloader, or a login prompt). Read-only: it never sends input to the guest.", defaults.ProductName()),
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
						"wake_screen": {
							Type:        "boolean",
							Description: "Move the mouse cursor before capturing to wake a blanked screen. Defaults to false.",
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
			Handler: screenshot,
			TargetCompatibilityFilters: []func() bool{
				kubevirt.HasVirtualMachine(p),
			},
		},
	}
}

func screenshot(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	namespace := p.RequiredString("namespace")
	name := p.RequiredString("name")
	wakeScreen := p.OptionalBool("wake_screen", false)
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	pngBytes, description, err := kubevirt.Screenshot(
		params.Context,
		params.DynamicClient(),
		params.RESTConfig(),
		namespace,
		name,
		wakeScreen,
	)
	if err != nil {
		// ConsoleError.Error() is a safe, client-facing message (it never
		// includes the underlying cause), so it can be surfaced directly while
		// the wrapped cause remains available for server-side logging.
		return api.NewToolCallResult("", err), nil
	}

	blocks := []*api.ToolContent{
		api.NewTextToolContent(description),
		api.NewImageToolContent(pngBytes, "image/png"),
	}
	return api.NewToolCallResultWithContent(blocks, nil, nil), nil
}
