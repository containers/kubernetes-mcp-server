package prompts

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/tools"
)

func InitIstioConfigReview() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "istio-config-review",
				Title:       "Review Istio Configuration",
				Description: "Review and validate Istio configuration in a namespace, checking for misconfigurations and best practice violations",
				Arguments: []api.PromptArgument{
					{
						Name:        "namespace",
						Description: "Namespace to review Istio configuration for",
						Required:    true,
					},
				},
			},
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

	klog.FromContext(params.Context).Info("Starting Istio config review...", "namespace", namespace)

	kiali := kialiclient.NewKiali(params, params.RESTConfig())

	istioContent := fetchKialiData(kiali, params, tools.KialiManageIstioConfigReadEndpoint,
		map[string]any{"namespace": namespace, "action": "list"})

	resourceContent := fetchKialiData(kiali, params, tools.KialiListOrGetResourcesEndpoint,
		map[string]any{"namespaces": namespace, "resourceType": "service"})

	promptText := buildIstioConfigReviewPrompt(namespace, istioContent, resourceContent)

	return api.NewPromptCallResult(
		"Istio configuration data gathered successfully",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: promptText,
				},
			},
			{
				Role: "assistant",
				Content: api.PromptContent{
					Type: "text",
					Text: "I'll review the Istio configuration and check for issues or misconfigurations.",
				},
			},
		},
		nil,
	), nil
}

func buildIstioConfigReviewPrompt(namespace, istioData, resourceData string) string {
	return fmt.Sprintf(`# Istio Configuration Review

## Namespace: %s

Review the Istio configuration in this namespace for correctness, best practices, and potential issues.

---

## Istio Resources

%s

---

## Services in Namespace

%s

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
`, namespace, istioData, resourceData)
}
