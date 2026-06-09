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

// NewCore creates a Core wrapper around a Kubernetes client.
// If redactSecretsMode is non-empty ("opaque" or "hashed"), Secret values
// are automatically redacted in ResourcesGet, ResourcesList, and
// ResourcesCreateOrUpdate responses. The redaction covers data.*,
// stringData.*, and strips the last-applied-configuration annotation.
func NewCore(client api.KubernetesClient, redactSecretsMode string) *Core {
	return &Core{
		KubernetesClient: client,
		redactor:         redaction.NewSecretRedactor(redactSecretsMode),
	}
}

// RedactResource applies Secret redaction to a single unstructured resource.
// It is a no-op if no redactor is configured or the resource is not a Secret.
func (c *Core) RedactResource(obj *unstructured.Unstructured) {
	if c.redactor != nil {
		c.redactor.Apply(obj)
	}
}

// RedactResourceList applies Secret redaction to all items in an unstructured list.
// It is a no-op if no redactor is configured.
func (c *Core) RedactResourceList(list *unstructured.UnstructuredList) {
	if c.redactor != nil {
		c.redactor.ApplyToList(list)
	}
}
