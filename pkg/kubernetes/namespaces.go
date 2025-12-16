package kubernetes

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (c *Core) NamespacesList(ctx context.Context, options api.ListOptions) (runtime.Unstructured, error) {
	return c.ResourcesList(ctx, &schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Namespace",
	}, "", options)
}

func (c *Core) ProjectsList(ctx context.Context, options api.ListOptions) (runtime.Unstructured, error) {
	return c.ResourcesList(ctx, &schema.GroupVersionKind{
		Group: "project.openshift.io", Version: "v1", Kind: "Project",
	}, "", options)
}
