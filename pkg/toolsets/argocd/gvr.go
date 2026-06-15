package argocd

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	applicationGVR = schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}
	appProjectGVR = schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "appprojects",
	}
	argocdGVR = schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1beta1",
		Resource: "argocds",
	}
)

func listResources(gvr schema.GroupVersionResource, resourceName string) api.ToolHandlerFunc {
	return func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		p := api.WrapParams(params)
		namespace := p.OptionalString("namespace", "")
		labelSelector := p.OptionalString("labelSelector", "")
		if err := p.Err(); err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to list %s: %w", resourceName, err)), nil
		}
		listOpts := metav1.ListOptions{}
		if labelSelector != "" {
			listOpts.LabelSelector = labelSelector
		}
		ret, err := params.DynamicClient().Resource(gvr).Namespace(namespace).List(params.Context, listOpts)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to list %s: %w", resourceName, err)), nil
		}
		return api.NewToolCallResult(params.ListOutput.PrintObj(ret)), nil
	}
}
