package prompts

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

func InitIstioConfigReview(ctx context.Context, inspector api.ClusterInspector) []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "istio-config-review",
				Title:       "Review Istio Configuration",
				Description: "Review and validate Istio configuration in a namespace, checking for misconfigurations and best practice violations",
				Arguments: []config.PromptArgument{
					{
						Name:        "namespace",
						Description: "Namespace to review Istio configuration for",
						Required:    true,
					},
				},
			},
			RBAC:    tools.MergeRBACBounded(tools.IstioConfigReadRBAC(), tools.ResourcesRBAC(ctx, inspector)),
			Handler: istioConfigReviewHandler,
		},
	}
}

func istioConfigReviewHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	namespace := args["namespace"]

	if namespace == "" {
		return nil, fmt.Errorf("namespace argument is required")
	}

	klogutil.FromContext(params.Context).Info("Starting Istio config review...", "namespace", namespace)

	kiali, err := kialiclient.NewKiali(params.Config, params.RESTConfig())
	if err != nil {
		return nil, err
	}

	b := api.NewPromptBudget(params.Context, istioConfigReviewAssistantText)
	writeIstioConfigReviewPrompt(kiali, params, namespace, b)

	return api.NewPromptCallResult(
		"Istio configuration data gathered successfully",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: b.String(),
				},
			},
			{
				Role: "assistant",
				Content: api.PromptContent{
					Type: "text",
					Text: istioConfigReviewAssistantText,
				},
			},
		},
		nil,
	), nil
}

const istioConfigReviewAssistantText = "I'll review the Istio configuration and check for issues or misconfigurations."

// writeIstioConfigReviewPrompt writes the review into one prompt budget.
// The service list does not start once that budget is spent.
func writeIstioConfigReviewPrompt(kiali *kialiclient.Kiali, params api.PromptHandlerParams, namespace string, b *api.BuilderWithBudget) {
	_ = b.Sections(
		api.Text(fmt.Sprintf(`# Istio Configuration Review

## Namespace: %s

Review the Istio configuration in this namespace for correctness, best practices, and potential issues.

---

## Istio Resources

`, namespace)),
		func(b *api.BuilderWithBudget) error {
			writeKialiData(b, kiali, params, tools.KialiManageIstioConfigReadEndpoint,
				map[string]any{"namespace": namespace, "action": "list"})
			return nil
		},
		api.Text(`

---

## Services in Namespace

`),
		func(b *api.BuilderWithBudget) error {
			writeKialiData(b, kiali, params, tools.KialiListOrGetResourcesEndpoint,
				map[string]any{"namespaces": namespace, "resourceType": "service"})
			return nil
		},
		api.Text(`

---

## Review Checklist

Analyze the configuration above and check for:

1. **Validation Errors**: Are there any Istio validation warnings or errors reported?
2. **VirtualService Issues**: Do VirtualServices reference hosts and subsets that actually exist? Are weights valid?
3. **DestinationRule Consistency**: Do DestinationRules define subsets that match actual workload labels?
4. **Traffic Policy**: Are there conflicting traffic policies or missing mTLS settings?
5. **Gateway Configuration**: Are Gateways properly configured with correct hosts and ports?
6. **Best Practices**: Are there any configuration anti-patterns (e.g., overly broad host matches, missing retries)?

Provide a summary of findings with severity (critical, warning, info) and recommendations.
`),
	)
}
