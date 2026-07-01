package kubevirt

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
)

func initHCOStatus() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "hco-status",
				Title:       fmt.Sprintf("%s HyperConverged Status", defaults.ProductName()),
				Description: fmt.Sprintf("Generate a status report for the HyperConverged Cluster Operator (HCO) managing %s and related components", defaults.ProductName()),
			},
			Handler: hcoStatusHandler,
		},
	}
}

func hcoStatusHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	ctx := params.Context
	if ctx == nil {
		ctx = context.Background()
	}

	dynamicClient := params.DynamicClient()

	hco, namespace, err := discoverHyperConvergedCR(ctx, dynamicClient)
	if err != nil {
		return nil, err
	}

	hcoStatusYaml := fetchHCOConditionsAndVersions(hco)
	hcoConfigYaml := fetchHCOConfiguration(hco)
	componentsYaml := fetchManagedComponentStatus(ctx, dynamicClient, namespace)
	eventsYaml := fetchHCOEvents(ctx, params.KubernetesClient, namespace)

	reportText := fmt.Sprintf(`# %s HyperConverged Cluster Operator Status Report

## HCO: %s (namespace: %s)

Use this report to understand the current state of the HyperConverged Cluster Operator and all components it manages.

---

## Step 1: HCO Conditions and Versions

Check the HCO conditions:
- **Available** should be True — the operator and all components are running
- **Progressing** should be False — no reconciliation in progress
- **Degraded** should be False — no component failures
- **TaintedConfiguration** indicates manual modifications to managed CRs via jsonpatch annotations
- **Upgradeable** indicates whether the operator can be safely upgraded

%s

---

## Step 2: HCO Configuration

Review the active configuration including feature gates and live migration settings:

%s

---

## Step 3: Managed Component Status

HCO manages several components. Each should have its own CR with conditions:

%s

---

## Step 4: Events

Review events in the HCO namespace for warnings or errors:

%s

---

## Analysis

Based on the data above, determine:

1. **Overall Health**: Are all HCO conditions healthy (Available=True, Degraded=False)?
2. **Component Health**: Are all managed components (KubeVirt, CDI, Network Addons) available?
3. **Configuration**: Are there any non-default feature gates or live migration settings?
4. **Warnings**: Are there any Warning events or TaintedConfiguration conditions?
5. **Versions**: What versions are installed for each component?
`, defaults.ProductName(), hco.GetName(), namespace, hcoStatusYaml, hcoConfigYaml, componentsYaml, eventsYaml)

	return api.NewPromptCallResult(
		"HyperConverged status report generated",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: reportText,
				},
			},
			{
				Role: "assistant",
				Content: api.PromptContent{
					Type: "text",
					Text: "I'll analyze the HyperConverged Cluster Operator status report to assess the overall health and configuration.",
				},
			},
		},
		nil,
	), nil
}

func discoverHyperConvergedCR(ctx context.Context, dynamicClient dynamic.Interface) (*unstructured.Unstructured, string, error) {
	hcoList, err := dynamicClient.Resource(kubevirt.HyperConvergedGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("HyperConverged Cluster Operator is not installed (failed to list HyperConverged resources: %v)", err)
	}

	if len(hcoList.Items) == 0 {
		return nil, "", fmt.Errorf("HyperConverged Cluster Operator is not installed (no HyperConverged CR found)")
	}

	hco := &hcoList.Items[0]
	return hco, hco.GetNamespace(), nil
}

func fetchHCOConditionsAndVersions(hco *unstructured.Unstructured) string {
	var result strings.Builder

	conditions, found, err := unstructured.NestedSlice(hco.Object, "status", "conditions")
	if err != nil {
		return fmt.Sprintf("### HCO Conditions\n\n*Error extracting conditions: %v*", err)
	}
	if !found || len(conditions) == 0 {
		result.WriteString("### HCO Conditions\n\n*No conditions found*\n\n")
	} else {
		yamlStr, err := output.MarshalYaml(conditions)
		if err != nil {
			fmt.Fprintf(&result, "### HCO Conditions\n\n*Error marshaling conditions: %v*\n\n", err)
		} else {
			fmt.Fprintf(&result, "### HCO Conditions\n\n```yaml\n%s```\n\n", yamlStr)
		}
	}

	versions, found, err := unstructured.NestedSlice(hco.Object, "status", "versions")
	if err != nil {
		fmt.Fprintf(&result, "### Component Versions\n\n*Error extracting versions: %v*", err)
		return result.String()
	}
	if !found || len(versions) == 0 {
		result.WriteString("### Component Versions\n\n*No version information found*")
		return result.String()
	}

	yamlStr, err := output.MarshalYaml(versions)
	if err != nil {
		fmt.Fprintf(&result, "### Component Versions\n\n*Error marshaling versions: %v*", err)
	} else {
		fmt.Fprintf(&result, "### Component Versions\n\n```yaml\n%s```", yamlStr)
	}

	return result.String()
}

func fetchHCOConfiguration(hco *unstructured.Unstructured) string {
	var result strings.Builder

	featureGates, found, err := unstructured.NestedMap(hco.Object, "spec", "featureGates")
	if err != nil {
		fmt.Fprintf(&result, "### Feature Gates\n\n*Error extracting feature gates: %v*\n\n", err)
	} else if !found || len(featureGates) == 0 {
		result.WriteString("### Feature Gates\n\n*No feature gates configured*\n\n")
	} else {
		yamlStr, err := output.MarshalYaml(featureGates)
		if err != nil {
			fmt.Fprintf(&result, "### Feature Gates\n\n*Error marshaling feature gates: %v*\n\n", err)
		} else {
			fmt.Fprintf(&result, "### Feature Gates\n\n```yaml\n%s```\n\n", yamlStr)
		}
	}

	liveMigration, found, err := unstructured.NestedMap(hco.Object, "spec", "liveMigrationConfig")
	if err != nil {
		fmt.Fprintf(&result, "### Live Migration Configuration\n\n*Error extracting live migration config: %v*", err)
	} else if !found || len(liveMigration) == 0 {
		result.WriteString("### Live Migration Configuration\n\n*Using default live migration settings*")
	} else {
		yamlStr, err := output.MarshalYaml(liveMigration)
		if err != nil {
			fmt.Fprintf(&result, "### Live Migration Configuration\n\n*Error marshaling live migration config: %v*", err)
		} else {
			fmt.Fprintf(&result, "### Live Migration Configuration\n\n```yaml\n%s```", yamlStr)
		}
	}

	return result.String()
}

func fetchManagedComponentStatus(ctx context.Context, dynamicClient dynamic.Interface, namespace string) string {
	var result strings.Builder

	result.WriteString(fetchComponentCR(ctx, dynamicClient, "KubeVirt", kubevirt.KubeVirtCRGVR, namespace))
	result.WriteString("\n\n")
	result.WriteString(fetchComponentCR(ctx, dynamicClient, "CDI", kubevirt.CDIGVR, ""))
	result.WriteString("\n\n")
	result.WriteString(fetchComponentCR(ctx, dynamicClient, "NetworkAddonsConfig", kubevirt.NetworkAddonsConfigGVR, ""))

	return result.String()
}

func fetchComponentCR(ctx context.Context, dynamicClient dynamic.Interface, name string, gvr schema.GroupVersionResource, namespace string) string {
	list, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Sprintf("### %s\n\n*Error fetching %s: %v*", name, name, err)
	}

	if len(list.Items) == 0 {
		return fmt.Sprintf("### %s\n\n*%s CR not found*", name, name)
	}

	cr := &list.Items[0]

	conditions, found, err := unstructured.NestedSlice(cr.Object, "status", "conditions")
	if err != nil || !found {
		status, sFound, sErr := unstructured.NestedMap(cr.Object, "status")
		if sErr != nil || !sFound {
			return fmt.Sprintf("### %s: %s/%s\n\n*No status available*", name, cr.GetNamespace(), cr.GetName())
		}
		yamlStr, err := output.MarshalYaml(status)
		if err != nil {
			return fmt.Sprintf("### %s: %s/%s\n\n*Error marshaling status: %v*", name, cr.GetNamespace(), cr.GetName(), err)
		}
		return fmt.Sprintf("### %s: %s/%s\n\n```yaml\n%s```", name, cr.GetNamespace(), cr.GetName(), yamlStr)
	}

	yamlStr, err := output.MarshalYaml(conditions)
	if err != nil {
		return fmt.Sprintf("### %s: %s/%s\n\n*Error marshaling conditions: %v*", name, cr.GetNamespace(), cr.GetName(), err)
	}
	return fmt.Sprintf("### %s: %s/%s\n\n```yaml\n%s```", name, cr.GetNamespace(), cr.GetName(), yamlStr)
}

func fetchHCOEvents(ctx context.Context, client api.KubernetesClient, namespace string) string {
	core := kubernetes.NewCore(client)

	allEvents, err := core.EventsList(ctx, namespace, api.ListOptions{})
	if err != nil {
		return fmt.Sprintf("### Events\n\n*Error listing events: %v*", err)
	}

	var warningEvents []map[string]any
	for _, event := range allEvents {
		eventType, _ := event["Type"].(string)
		if eventType == "Warning" {
			warningEvents = append(warningEvents, event)
		}
	}

	if len(warningEvents) == 0 {
		return "### Events\n\n*No warning events found in the HCO namespace*"
	}

	yamlStr, err := output.MarshalYaml(warningEvents)
	if err != nil {
		return fmt.Sprintf("### Events\n\n*Error marshaling events: %v*", err)
	}

	return fmt.Sprintf("### Warning Events in %s namespace\n\n```yaml\n%s```", namespace, yamlStr)
}
