package kubernetes

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/redaction"
)

type Core struct {
	api.KubernetesClient
	redactor *redaction.Redactor
}

func NewCore(client api.KubernetesClient) *Core {
	return &Core{
		KubernetesClient: client,
	}
}

// NewCoreWithRedaction creates a Core with field-level redaction enabled.
// The redactor is applied automatically in ResourcesGet, ResourcesList, and
// ResourcesCreateOrUpdate. Toolsets that call the dynamic client directly
// should use RedactResources/RedactResourceList to apply redaction manually.
func NewCoreWithRedaction(client api.KubernetesClient, redactedResources []api.RedactedResource) *Core {
	c := &Core{
		KubernetesClient: client,
	}
	if len(redactedResources) > 0 {
		c.redactor = redaction.NewRedactor(redactedResources)
	}
	return c
}

// RedactResource applies field-level redaction to a single unstructured resource.
// This is exported for toolsets that bypass the Core wrapper and call the dynamic client directly.
// It is a no-op if no redactor is configured.
func (c *Core) RedactResource(obj *unstructured.Unstructured) {
	if c.redactor != nil {
		c.redactor.Apply(obj)
	}
}

// RedactResourceList applies field-level redaction to all items in an unstructured list.
// This is exported for toolsets that bypass the Core wrapper and call the dynamic client directly.
// It is a no-op if no redactor is configured.
func (c *Core) RedactResourceList(list *unstructured.UnstructuredList) {
	if c.redactor != nil {
		c.redactor.ApplyToList(list)
	}
}
