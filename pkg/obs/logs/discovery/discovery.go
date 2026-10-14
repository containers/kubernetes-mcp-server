package discovery

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
)

type LokiInstance struct {
	Namespace string `json:"lokiNamespace"`
	Name      string `json:"lokiName"`
	Status    string `json:"status"`
	baseURL   string
}

// ListInstances lists all LokiStack CRs and resolves their base URLs.
// resolver is used for cluster-based discovery (e.g. OpenShift Routes); nil falls back to HTTP service DNS.
func ListInstances(ctx context.Context, k8sClient dynamic.Interface, resolver GatewayResolver) ([]LokiInstance, error) {
	if k8sClient == nil {
		return nil, fmt.Errorf("kubernetes dynamic client is not available")
	}

	list, err := k8sClient.Resource(lokiStackGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list LokiStacks: %w", err)
	}

	instances := make([]LokiInstance, 0, len(list.Items))
	for _, item := range list.Items {
		var stack LokiStack
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &stack); err != nil {
			return nil, fmt.Errorf("failed to parse LokiStack: %w", err)
		}

		tenantsMode := ""
		if stack.Spec.Tenants != nil {
			tenantsMode = stack.Spec.Tenants.Mode
		}
		baseURL, err := resolveBaseURL(ctx, k8sClient, resolver, stack.Namespace, stack.Name, tenantsMode)
		if err != nil {
			return nil, err
		}
		instances = append(instances, LokiInstance{
			Namespace: stack.Namespace,
			Name:      stack.Name,
			Status:    getStatusFromConditions(stack.Status.Conditions),
			baseURL:   baseURL,
		})
	}

	return instances, nil
}

func FindInstanceByName(instances []LokiInstance, namespace, name string) (LokiInstance, error) {
	for _, instance := range instances {
		if instance.Namespace == namespace && instance.Name == name {
			return instance, nil
		}
	}
	return LokiInstance{}, fmt.Errorf("LokiStack %s/%s not found", namespace, name)
}

func resolveBaseURL(ctx context.Context, k8sClient dynamic.Interface, resolver GatewayResolver, namespace, stackName, tenantsMode string) (string, error) {
	if resolver != nil {
		return resolver.ResolveGatewayURL(ctx, k8sClient, namespace, stackName, tenantsMode)
	}
	gatewaySvcName := fmt.Sprintf("%s-gateway-http", stackName)
	return fmt.Sprintf("http://%s.%s.svc:8080", gatewaySvcName, namespace), nil
}

func getStatusFromConditions(conditions []metav1.Condition) string {
	for _, cond := range conditions {
		if cond.Status == metav1.ConditionTrue {
			return cond.Type
		}
	}
	return ""
}

func (l *LokiInstance) GetURL() string {
	return l.baseURL
}
