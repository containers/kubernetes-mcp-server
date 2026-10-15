package mcp

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func newHandlerParams[T any](s *Server, ctx context.Context, cfg *Configuration, request T, cluster string) (api.HandlerParams[T], error) {
	k8s, err := s.p.GetDerivedKubernetes(ctx, cluster)
	if err != nil {
		return api.HandlerParams[T]{}, err
	}
	return api.HandlerParams[T]{
		Context:          ctx,
		Config:           cfg.Config,
		KubernetesClient: k8s,
		Request:          request,
		ListOutput:       cfg.ListOutput(),
		Elicitor:         &sessionElicitor{},
	}, nil
}
