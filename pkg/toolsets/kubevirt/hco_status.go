package kubevirt

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
)

func initHCOStatus() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: config.Prompt{
				Name:        "hco-status",
				Title:       fmt.Sprintf("%s HyperConverged Status", defaults.ProductName()),
				Description: fmt.Sprintf("Generate a status report for the HyperConverged Cluster Operator (HCO) managing %s and related components", defaults.ProductName()),
			},
			RBAC: api.RBACBounded(
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "hco.kubevirt.io", Resource: "hyperconvergeds"}},
					Namespace: &api.RBACNamespace{AllNamespaces: true},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "kubevirt.io", Resource: "kubevirts"}},
					Namespace: &api.RBACNamespace{AllNamespaces: true},
				},
				api.RBACRequirement{
					Verbs:  []string{"list"},
					Target: api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "cdi.kubevirt.io", Resource: "cdis"}},
				},
				api.RBACRequirement{
					Verbs:  []string{"list"},
					Target: api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "networkaddonsoperator.network.kubevirt.io", Resource: "networkaddonsconfigs"}},
				},
				api.RBACRequirement{
					Verbs:     []string{"list"},
					Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "events"}},
					Namespace: &api.RBACNamespace{AllNamespaces: true},
				},
			),
			Handler: hcoStatusHandler,
		},
	}
}

// hcoStatusAssistantText counts toward the same result as the user prompt.
const hcoStatusAssistantText = "I'll analyze the HyperConverged Cluster Operator status report to assess the overall health and configuration."

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

	// The assistant message is reserved because it counts toward the same
	// result. Component lists and the event list stop once the budget is
	// spent. See docs/specs/memory-bounds.md#retention.
	b := api.NewPromptBudget(ctx, hcoStatusAssistantText)
	writeHCOReport(ctx, dynamicClient, params.KubernetesClient, hco, namespace, b)

	return api.NewPromptCallResult(
		"HyperConverged status report generated",
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
					Text: hcoStatusAssistantText,
				},
			},
		},
		nil,
	), nil
}

func writeHCOReport(ctx context.Context, dynamicClient dynamic.Interface, client api.KubernetesClient, hco *unstructured.Unstructured, namespace string, b *api.BuilderWithBudget) {
	_ = b.Sections(
		func(b *api.BuilderWithBudget) error {
			writeHCOBeforeComponents(b, hco, namespace)
			return nil
		},
		func(b *api.BuilderWithBudget) error {
			writeManagedComponentStatus(ctx, dynamicClient, namespace, b)
			return nil
		},
		api.Text(`

---

## Step 4: Events

Review events in the HCO namespace for warnings or errors:

`),
		func(b *api.BuilderWithBudget) error {
			writeHCOEvents(ctx, client, namespace, b)
			return nil
		},
		api.Text(`

---

## Analysis

Based on the data above, determine:

1. **Overall Health**: Are all HCO conditions healthy (Available=True, Degraded=False)?
2. **Component Health**: Are all managed components (KubeVirt, CDI, Network Addons) available?
3. **Configuration**: Are there any non-default feature gates or virtualization settings?
4. **Warnings**: Are there any Warning events or TaintedConfiguration conditions?
5. **Versions**: What versions are installed for each component?
`),
	)
}

func writeHCOBeforeComponents(b *api.BuilderWithBudget, hco *unstructured.Unstructured, namespace string) {
	_ = b.Sections(
		api.Text(fmt.Sprintf(`# %s HyperConverged Cluster Operator Status Report

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

`, defaults.ProductName(), hco.GetName(), namespace)),
		func(b *api.BuilderWithBudget) error {
			writeHCOConditionsAndVersions(b, hco)
			return nil
		},
		api.Text(`

---

## Step 2: HCO Configuration

Review the active configuration including feature gates and virtualization settings:

`),
		func(b *api.BuilderWithBudget) error {
			writeHCOConfiguration(b, hco)
			return nil
		},
		api.Text(`

---

## Step 3: Managed Component Status

HCO manages several components. Each should have its own CR with conditions:

`),
	)
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

func writeHCOConditionsAndVersions(b *api.BuilderWithBudget, hco *unstructured.Unstructured) {
	_ = b.Sections(
		func(b *api.BuilderWithBudget) error {
			conditions, found, err := unstructured.NestedSlice(hco.Object, "status", "conditions")
			if err != nil {
				b.WriteString(fmt.Sprintf("### HCO Conditions\n\n*Error extracting conditions: %v*", err))
				return nil
			}
			if !found || len(conditions) == 0 {
				b.WriteString("### HCO Conditions\n\n*No conditions found*\n\n")
				return nil
			}
			yamlStr, err := output.MarshalYaml(conditions)
			if err != nil {
				b.WriteString(fmt.Sprintf("### HCO Conditions\n\n*Error marshaling conditions: %v*\n\n", err))
				return nil
			}
			b.WriteString(fmt.Sprintf("### HCO Conditions\n\n```yaml\n%s```\n\n", yamlStr))
			return nil
		},
		func(b *api.BuilderWithBudget) error {
			versions, found, err := unstructured.NestedSlice(hco.Object, "status", "versions")
			if err != nil {
				b.WriteString(fmt.Sprintf("### Component Versions\n\n*Error extracting versions: %v*", err))
				return nil
			}
			if !found || len(versions) == 0 {
				b.WriteString("### Component Versions\n\n*No version information found*")
				return nil
			}
			yamlStr, err := output.MarshalYaml(versions)
			if err != nil {
				b.WriteString(fmt.Sprintf("### Component Versions\n\n*Error marshaling versions: %v*", err))
				return nil
			}
			b.WriteString(fmt.Sprintf("### Component Versions\n\n```yaml\n%s```", yamlStr))
			return nil
		},
	)
}

func writeHCOConfiguration(b *api.BuilderWithBudget, hco *unstructured.Unstructured) {
	_ = b.Sections(
		func(b *api.BuilderWithBudget) error {
			featureGates, found, err := unstructured.NestedSlice(hco.Object, "spec", "featureGates")
			if err != nil {
				b.WriteString(fmt.Sprintf("### Feature Gates\n\n*Error extracting feature gates: %v*\n\n", err))
				return nil
			}
			if !found || len(featureGates) == 0 {
				b.WriteString("### Feature Gates\n\n*No feature gates configured*\n\n")
				return nil
			}
			yamlStr, err := output.MarshalYaml(featureGates)
			if err != nil {
				b.WriteString(fmt.Sprintf("### Feature Gates\n\n*Error marshaling feature gates: %v*\n\n", err))
				return nil
			}
			b.WriteString(fmt.Sprintf("### Feature Gates\n\n```yaml\n%s```\n\n", yamlStr))
			return nil
		},
		func(b *api.BuilderWithBudget) error {
			virtualization, found, err := unstructured.NestedMap(hco.Object, "spec", "virtualization")
			if err != nil {
				b.WriteString(fmt.Sprintf("### Virtualization Configuration\n\n*Error extracting virtualization config: %v*", err))
				return nil
			}
			if !found || len(virtualization) == 0 {
				b.WriteString("### Virtualization Configuration\n\n*Using default virtualization settings*")
				return nil
			}
			yamlStr, err := output.MarshalYaml(virtualization)
			if err != nil {
				b.WriteString(fmt.Sprintf("### Virtualization Configuration\n\n*Error marshaling virtualization config: %v*", err))
				return nil
			}
			b.WriteString(fmt.Sprintf("### Virtualization Configuration\n\n```yaml\n%s```", yamlStr))
			return nil
		},
	)
}

// writeManagedComponentStatus lists KubeVirt, CDI, then NetworkAddonsConfig.
// The next list does not start once the result budget is spent.
func writeManagedComponentStatus(ctx context.Context, dynamicClient dynamic.Interface, namespace string, b *api.BuilderWithBudget) {
	components := []struct {
		name string
		gvr  schema.GroupVersionResource
		ns   string
	}{
		{name: "KubeVirt", gvr: kubevirt.KubeVirtCRGVR, ns: namespace},
		{name: "CDI", gvr: kubevirt.CDIGVR},
		{name: "NetworkAddonsConfig", gvr: kubevirt.NetworkAddonsConfigGVR},
	}
	steps := make([]api.Step, 0, len(components)*2)
	for i, component := range components {
		if i > 0 {
			steps = append(steps, api.Text("\n\n"))
		}
		steps = append(steps, func(b *api.BuilderWithBudget) error {
			b.WriteString(fetchComponentCR(ctx, dynamicClient, component.name, component.gvr, component.ns))
			return nil
		})
	}
	_ = b.Sections(steps...)
}

func fetchComponentCR(ctx context.Context, dynamicClient dynamic.Interface, name string, gvr schema.GroupVersionResource, namespace string) string {
	list, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		switch {
		case apierrors.IsForbidden(err):
			// Access problem, not a component problem — tell the model not to infer health.
			return fmt.Sprintf("### %s\n\n*Access denied — no RBAC permission to read %s. "+
				"Component health is unknown (tooling limitation, not a failure).*", name, name)
		case apierrors.IsNotFound(err):
			return fmt.Sprintf("### %s\n\n*%s CRD not registered — this component is not installed.*", name, name)
		default:
			return fmt.Sprintf("### %s\n\n*Error fetching %s: %v*", name, name, err)
		}
	}

	if len(list.Items) == 0 {
		return fmt.Sprintf("### %s\n\n*%s CR not found*", name, name)
	}

	cr := &list.Items[0]

	conditions, found, err := unstructured.NestedSlice(cr.Object, "status", "conditions")
	if err != nil {
		return fmt.Sprintf("### %s: %s/%s\n\n*Error extracting conditions: %v*", name, cr.GetNamespace(), cr.GetName(), err)
	}
	if !found {
		status, sFound, sErr := unstructured.NestedMap(cr.Object, "status")
		if sErr != nil {
			return fmt.Sprintf("### %s: %s/%s\n\n*Error extracting status: %v*", name, cr.GetNamespace(), cr.GetName(), sErr)
		}
		if !sFound {
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

// writeHCOEvents lists events once and writes each warning as it is seen.
// Further warnings are not formatted once the budget is spent.
func writeHCOEvents(ctx context.Context, client api.KubernetesClient, namespace string, b *api.BuilderWithBudget) {
	if b == nil {
		return
	}
	core := kubernetes.NewCore(client)

	allEvents, err := core.EventsList(ctx, namespace, api.ListOptions{})
	if err != nil {
		b.WriteString(fmt.Sprintf("### Events\n\n*Error listing events: %v*", err))
		return
	}

	var warnings []map[string]any
	for _, event := range allEvents {
		eventType, _ := event["Type"].(string)
		if eventType == "Warning" {
			warnings = append(warnings, event)
		}
	}

	opened := false
	steps := make([]api.Step, 0, len(warnings)+1)
	for _, event := range warnings {
		steps = append(steps, func(b *api.BuilderWithBudget) error {
			item, err := yamlListItem(event)
			if err != nil {
				if !opened {
					b.WriteString(fmt.Sprintf("### Events\n\n*Error marshaling events: %v*", err))
				}
				return err
			}
			if !opened {
				b.WriteString(fmt.Sprintf("### Warning Events in %s namespace\n\n```yaml\n", namespace))
				opened = true
			}
			return b.Sections(func(b *api.BuilderWithBudget) error {
				b.WriteString(item)
				return nil
			})
		})
	}
	steps = append(steps, func(b *api.BuilderWithBudget) error {
		if !opened {
			b.WriteString("### Events\n\n*No warning events found in the HCO namespace*")
			return nil
		}
		b.WriteString("```")
		return nil
	})
	_ = b.Sections(steps...)
}

// yamlListItem renders one object as a YAML sequence entry so warnings can be
// written one at a time and still form one list when every warning fits.
func yamlListItem(v any) (string, error) {
	raw, err := output.MarshalYaml(v)
	if err != nil {
		return "", err
	}
	raw = strings.TrimRight(raw, "\n")
	lines := strings.Split(raw, "\n")
	lines[0] = "- " + lines[0]
	for i := 1; i < len(lines); i++ {
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n") + "\n", nil
}
