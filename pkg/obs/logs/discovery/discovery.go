package discovery

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
)

type LokiInstance struct {
	Namespace string `json:"lokiNamespace"`
	Name      string `json:"lokiName"`
	Status    string `json:"status"`
	baseURL   string
}

func ListInstances(ctx context.Context, k8sClient dynamic.Interface, useRoute bool) ([]LokiInstance, error) {
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

		baseURL, err := resolveBaseURL(ctx, k8sClient, useRoute, stack.Namespace, stack.Name)
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

func resolveBaseURL(ctx context.Context, k8sClient dynamic.Interface, useRoute bool, namespace, stackName string) (string, error) {
	gatewaySvcName := fmt.Sprintf("%s-gateway-http", stackName)
	if useRoute {
		routeHost, err := resolveRouteHost(ctx, k8sClient, namespace, stackName, gatewaySvcName)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("https://%s", routeHost), nil
	}

	return fmt.Sprintf("https://%s.%s.svc:8080", gatewaySvcName, namespace), nil
}

func resolveRouteHost(ctx context.Context, k8sClient dynamic.Interface, namespace, stackName, gatewaySvcName string) (string, error) {
	for _, routeName := range []string{stackName, gatewaySvcName} {
		host, err := getRouteHost(ctx, k8sClient, namespace, routeName)
		if err == nil {
			return host, nil
		}
		if !apierrors.IsNotFound(err) {
			return "", err
		}
	}

	list, err := k8sClient.Resource(routeGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list routes in %s: %w", namespace, err)
	}
	for _, item := range list.Items {
		toName, found, err := unstructured.NestedString(item.Object, "spec", "to", "name")
		if err != nil || !found || toName != gatewaySvcName {
			continue
		}
		host, found, err := unstructured.NestedString(item.Object, "spec", "host")
		if err != nil || !found || host == "" {
			continue
		}
		return host, nil
	}

	return "", fmt.Errorf("no route found for gateway service %s/%s", namespace, gatewaySvcName)
}

func getRouteHost(ctx context.Context, k8sClient dynamic.Interface, namespace, routeName string) (string, error) {
	unstructuredRoute, err := k8sClient.Resource(routeGVR).Namespace(namespace).Get(ctx, routeName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	var route Route
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredRoute.Object, &route); err != nil {
		return "", fmt.Errorf("failed to parse route %s/%s: %w", namespace, routeName, err)
	}
	if route.Spec.Host == "" {
		return "", fmt.Errorf("route %s/%s has no host", namespace, routeName)
	}
	return route.Spec.Host, nil
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
