package openshift

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"

	tracesdiscovery "github.com/rhobs/obs-mcp/pkg/traces/discovery"
)

// RouteClient provides common OpenShift Route operations shared by
// all signal-specific resolvers. It also directly satisfies the traces
// EndpointResolver interface.
type RouteClient struct{}

var _ tracesdiscovery.EndpointResolver = (*RouteClient)(nil)

func (c *RouteClient) ResolveEndpoint(ctx context.Context, client dynamic.Interface, namespace, routeName string) (string, error) {
	host, err := c.GetRouteHost(ctx, client, namespace, routeName)
	if err != nil {
		return "", err
	}
	return "https://" + host, nil
}

func (c *RouteClient) GetRouteHost(ctx context.Context, client dynamic.Interface, namespace, routeName string) (string, error) {
	unstructuredRoute, err := client.Resource(RouteGVR).Namespace(namespace).Get(ctx, routeName, metav1.GetOptions{})
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

func (c *RouteClient) FindRouteByTargetService(ctx context.Context, client dynamic.Interface, namespace, targetService string) (string, error) {
	list, err := client.Resource(RouteGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list routes in %s: %w", namespace, err)
	}

	for _, item := range list.Items {
		toName, found, err := unstructured.NestedString(item.Object, "spec", "to", "name")
		if err != nil || !found || toName != targetService {
			continue
		}
		host, found, err := unstructured.NestedString(item.Object, "spec", "host")
		if err != nil || !found || host == "" {
			continue
		}
		return host, nil
	}

	return "", fmt.Errorf("no route found targeting service %s in namespace %s", targetService, namespace)
}
