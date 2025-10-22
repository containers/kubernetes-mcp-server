package troubleshoot

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"
)

//go:embed plan.tmpl
var planTemplate string

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_troubleshoot",
				Description: "Generate a comprehensive troubleshooting guide for a VirtualMachine, providing step-by-step instructions to diagnose common issues",
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
					Title:           "Virtual Machine: Troubleshoot",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: troubleshoot,
		},
	}
}

type troubleshootParams struct {
	Namespace string
	Name      string
}

func troubleshoot(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse required parameters
	namespace, err := getRequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	name, err := getRequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	// Prepare template parameters
	templateParams := troubleshootParams{
		Namespace: namespace,
		Name:      name,
	}

	// Render template
	tmpl, err := template.New("troubleshoot").Parse(planTemplate)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to parse template: %w", err)), nil
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, templateParams); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to render template: %w", err)), nil
	}

	return api.NewToolCallResult(result.String(), nil), nil
}

func getRequiredString(params api.ToolHandlerParams, key string) (string, error) {
	args := params.GetArguments()
	val, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s parameter required", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("%s parameter must be a string", key)
	}
	return str, nil
}
