package tekton

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	versioned "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
)

// newTektonClient creates a Tekton clientset from the REST config in the provided KubernetesClient.
func newTektonClient(k api.KubernetesClient) (versioned.Interface, error) {
	return versioned.NewForConfig(k.RESTConfig())
}
