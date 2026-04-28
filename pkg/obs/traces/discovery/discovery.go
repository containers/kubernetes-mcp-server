package discovery

import (
	"context"
	"fmt"
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
)

type TempoInstance struct {
	Kind         KindType `json:"kind"`
	Namespace    string   `json:"tempoNamespace"`
	Name         string   `json:"tempoName"`
	Multitenancy bool     `json:"multitenancy"`
	Tenants      []string `json:"tenants,omitempty"`
	Status       string   `json:"status"`
	baseURL      string
}

type KindType string

const (
	KindTempoStack      KindType = "TempoStack"
	KindTempoMonolithic KindType = "TempoMonolithic"
)

func ListInstances(ctx context.Context, k8sClient dynamic.Interface, useRoute bool) ([]TempoInstance, error) {
	tempos := []TempoInstance{}

	tempoStacks, err := listTempoStacks(ctx, k8sClient, useRoute)
	if err != nil {
		return nil, err
	}
	tempos = append(tempos, tempoStacks...)

	tempoMonolithics, err := listTempoMonolithics(ctx, k8sClient, useRoute)
	if err != nil {
		return nil, err
	}
	tempos = append(tempos, tempoMonolithics...)

	return tempos, nil
}

func listTempoStacks(ctx context.Context, k8sClient dynamic.Interface, useRoute bool) ([]TempoInstance, error) {
	list, err := k8sClient.Resource(tempoStackGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list TempoStacks: %w", err)
	}

	var instances []TempoInstance
	for _, item := range list.Items {
		var tempo TempoStack
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &tempo)
		if err != nil {
			return nil, fmt.Errorf("failed to parse TempoStack: %w", err)
		}

		multitenancy := tempo.Spec.Tenants != nil && len(tempo.Spec.Tenants.Authentication) > 0

		var serviceName string
		var tenants []string
		if multitenancy {
			serviceName = DNSName(fmt.Sprintf("tempo-%s-gateway", tempo.Name))
			for _, auth := range tempo.Spec.Tenants.Authentication {
				tenants = append(tenants, auth.TenantName)
			}
		} else {
			serviceName = DNSName(fmt.Sprintf("tempo-%s-query-frontend", tempo.Name))
		}

		baseURL, err := resolveBaseURL(ctx, k8sClient, useRoute, tempo.Namespace, serviceName, multitenancy)
		if err != nil {
			return nil, err
		}

		status := getStatusFromConditions(tempo.Status.Conditions)

		instances = append(instances, TempoInstance{
			Kind:         KindTempoStack,
			Namespace:    tempo.Namespace,
			Name:         tempo.Name,
			Multitenancy: multitenancy,
			Tenants:      tenants,
			Status:       status,
			baseURL:      baseURL,
		})
	}

	return instances, nil
}

func listTempoMonolithics(ctx context.Context, k8sClient dynamic.Interface, useRoute bool) ([]TempoInstance, error) {
	list, err := k8sClient.Resource(tempoMonolithicGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list TempoMonolithics: %w", err)
	}

	var instances []TempoInstance
	for _, item := range list.Items {
		var tempo TempoMonolithic
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &tempo)
		if err != nil {
			return nil, fmt.Errorf("failed to parse TempoMonolithic: %w", err)
		}

		multitenancy := tempo.Spec.Multitenancy != nil && tempo.Spec.Multitenancy.Enabled && len(tempo.Spec.Multitenancy.Authentication) > 0

		var serviceName string
		var tenants []string
		if multitenancy {
			serviceName = DNSName(fmt.Sprintf("tempo-%s-gateway", tempo.Name))
			for _, auth := range tempo.Spec.Multitenancy.Authentication {
				tenants = append(tenants, auth.TenantName)
			}
		} else {
			serviceName = DNSName(fmt.Sprintf("tempo-%s", tempo.Name))
		}

		baseURL, err := resolveBaseURL(ctx, k8sClient, useRoute, tempo.Namespace, serviceName, multitenancy)
		if err != nil {
			return nil, err
		}

		status := getStatusFromConditions(tempo.Status.Conditions)

		instances = append(instances, TempoInstance{
			Kind:         KindTempoMonolithic,
			Namespace:    tempo.Namespace,
			Name:         tempo.Name,
			Multitenancy: multitenancy,
			Tenants:      tenants,
			Status:       status,
			baseURL:      baseURL,
		})
	}

	return instances, nil
}

func getStatusFromConditions(conditions []metav1.Condition) string {
	for _, cond := range conditions {
		if cond.Status == metav1.ConditionTrue {
			return cond.Type
		}
	}
	return ""
}

func resolveBaseURL(ctx context.Context, k8sClient dynamic.Interface, useRoute bool, namespace, serviceName string, multitenancy bool) (string, error) {
	if useRoute {
		routeHost, err := resolveRoute(ctx, k8sClient, namespace, serviceName)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("https://%s", routeHost), nil
	}
	if multitenancy {
		return fmt.Sprintf("https://%s.%s.svc:8080", serviceName, namespace), nil
	}
	return fmt.Sprintf("http://%s.%s.svc:3200", serviceName, namespace), nil
}

func resolveRoute(ctx context.Context, k8sClient dynamic.Interface, namespace, routeName string) (string, error) {
	unstructured, err := k8sClient.Resource(routeGVR).Namespace(namespace).Get(ctx, routeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get route %s/%s: %w", namespace, routeName, err)
	}

	var route Route
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured.Object, &route)
	if err != nil {
		return "", fmt.Errorf("failed to parse route %s/%s: %w", namespace, routeName, err)
	}

	return route.Spec.Host, nil
}

func (t *TempoInstance) GetURL(tenant string) string {
	if t.Multitenancy {
		return fmt.Sprintf("%s/api/traces/v1/%s/tempo", t.baseURL, url.PathEscape(tenant))
	}
	return t.baseURL
}
