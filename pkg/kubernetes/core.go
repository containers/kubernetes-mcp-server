package kubernetes

import "github.com/containers/kubernetes-mcp-server/pkg/api"

type Core struct {
	api.KubernetesClient
}

func NewCore(client api.KubernetesClient) *Core {
	return &Core{
		KubernetesClient: client,
	}
}
