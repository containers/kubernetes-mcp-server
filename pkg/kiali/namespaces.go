package kiali

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
)

// getAllNamespaces queries the namespaces API and returns a comma-separated list of namespace names.
func (k *Kiali) GetAllNamespaces(ctx context.Context) ([]kialitypes.NamespaceListItemRaw, error) {
	raw, err := k.executeRequest(ctx, http.MethodGet, NamespacesEndpoint, "", nil)
	if err != nil {
		return nil, err
	}
	var list []kialitypes.NamespaceListItemRaw
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

// ListNamespaces calls the Kiali namespaces API and the health API, then returns a summarized
// response. healthType controls the shape: "app" (default), "service", or "workload".
// The transform expects raw JSON array of namespace objects; the Health API expects a comma-separated list of names.
func (k *Kiali) ListNamespaces(ctx context.Context, namespaces []string, queryParams map[string]string) (string, error) {
	namespacesParam := strings.Join(namespaces, ",")
	var err error
	namespacesData, err := k.GetAllNamespaces(ctx)
	if err != nil {
		return "", err
	}
	if namespacesParam == "" {
		list := make([]string, 0, len(namespacesData))
		for _, ns := range namespacesData {
			list = append(list, ns.Name)
		}
		namespacesParam = strings.Join(list, ",")
	}

	ht := strings.TrimSpace(queryParams["type"])
	if ht != "service" && ht != "workload" {
		ht = "app"
	}
	healthRaw, err := k.Health(ctx, namespacesParam, map[string]string{
		"rateInterval": DefaultRateInterval,
		"type":         ht,
	})
	if err != nil {
		return "", err
	}
	var health kialitypes.ClustersNamespaceHealth
	if err := json.Unmarshal([]byte(healthRaw), &health); err != nil {
		return "", errors.New("failed to unmarshal health")
	}
	var result interface{}
	result, err = TransformNamespacesToTypeHealth(ht, namespacesData, health)

	if err != nil {
		return "", errors.New("failed to transform namespaces with health")
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}
