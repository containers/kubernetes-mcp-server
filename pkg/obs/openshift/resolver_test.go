package openshift

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFakeClientWithRoutes(routes ...*unstructured.Unstructured) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	objs := make([]runtime.Object, len(routes))
	for i, r := range routes {
		objs[i] = r
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		RouteGVR: "RouteList",
	}, objs...)
}

func newRoute(namespace, name, host string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "route.openshift.io",
		Version: "v1",
		Kind:    "Route",
	})
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.Object["spec"] = map[string]any{
		"host": host,
	}
	return obj
}

func newRouteWithTarget(namespace, name, host, targetSvc string) *unstructured.Unstructured {
	obj := newRoute(namespace, name, host)
	obj.Object["spec"] = map[string]any{
		"host": host,
		"to": map[string]any{
			"name": targetSvc,
		},
	}
	return obj
}

func TestRouteClient_ResolveEndpoint_Found(t *testing.T) {
	client := newFakeClientWithRoutes(
		newRoute("myns", "my-route", "my-route.example.com"),
	)
	rc := &RouteClient{}
	url, err := rc.ResolveEndpoint(context.Background(), client, "myns", "my-route")
	require.NoError(t, err)
	require.Equal(t, "https://my-route.example.com", url)
}

func TestRouteClient_ResolveEndpoint_NotFound(t *testing.T) {
	client := newFakeClientWithRoutes()
	rc := &RouteClient{}
	_, err := rc.ResolveEndpoint(context.Background(), client, "myns", "missing-route")
	require.Error(t, err)
}

func TestLogsGatewayResolver_ByStackName(t *testing.T) {
	client := newFakeClientWithRoutes(
		newRoute("logging", "logging-loki", "logging-loki.apps.example.com"),
	)
	resolver := &LogsGatewayResolver{}
	url, err := resolver.ResolveGatewayURL(context.Background(), client, "logging", "logging-loki", "openshift-network")
	require.NoError(t, err)
	require.Equal(t, "https://logging-loki.apps.example.com/api/logs/v1", url)
}

func TestLogsGatewayResolver_ByGatewayName(t *testing.T) {
	client := newFakeClientWithRoutes(
		newRoute("logging", "logging-loki-gateway-http", "gateway.apps.example.com"),
	)
	resolver := &LogsGatewayResolver{}
	url, err := resolver.ResolveGatewayURL(context.Background(), client, "logging", "logging-loki", "openshift-network")
	require.NoError(t, err)
	require.Equal(t, "https://gateway.apps.example.com/api/logs/v1", url)
}

func TestLogsGatewayResolver_ByTargetService(t *testing.T) {
	client := newFakeClientWithRoutes(
		newRouteWithTarget("logging", "custom-route", "custom.apps.example.com", "logging-loki-gateway-http"),
	)
	resolver := &LogsGatewayResolver{}
	url, err := resolver.ResolveGatewayURL(context.Background(), client, "logging", "logging-loki", "openshift-network")
	require.NoError(t, err)
	require.Equal(t, "https://custom.apps.example.com/api/logs/v1", url)
}

func TestLogsGatewayResolver_FallbackOpenShiftServiceDNS(t *testing.T) {
	client := newFakeClientWithRoutes()
	resolver := &LogsGatewayResolver{}
	url, err := resolver.ResolveGatewayURL(context.Background(), client, "logging", "logging-loki", "openshift-network")
	require.NoError(t, err)
	require.Equal(t, "https://logging-loki-gateway-http.logging.svc:8080/api/logs/v1", url)
}

func TestLogsGatewayResolver_FallbackHTTPServiceDNS(t *testing.T) {
	client := newFakeClientWithRoutes()
	resolver := &LogsGatewayResolver{}
	url, err := resolver.ResolveGatewayURL(context.Background(), client, "logging", "logging-loki", "static")
	require.NoError(t, err)
	require.Equal(t, "http://logging-loki-gateway-http.logging.svc:8080", url)
}

func TestMetricsRouteResolver_Thanos(t *testing.T) {
	client := newFakeClientWithRoutes(
		newRoute(MonitoringNamespace, ThanosQuerierRouteName, "thanos.apps.example.com"),
	)
	resolver := &MetricsRouteResolver{}
	url, err := resolver.ResolveMetricsBackend(context.Background(), client, "thanos")
	require.NoError(t, err)
	require.Equal(t, "https://thanos.apps.example.com", url)
}

func TestMetricsRouteResolver_ThanosFallbackToPrometheus(t *testing.T) {
	client := newFakeClientWithRoutes(
		newRoute(MonitoringNamespace, PrometheusRouteName, "prometheus.apps.example.com"),
	)
	resolver := &MetricsRouteResolver{}
	url, err := resolver.ResolveMetricsBackend(context.Background(), client, "thanos")
	require.NoError(t, err)
	require.Equal(t, "https://prometheus.apps.example.com", url)
}

func TestMetricsRouteResolver_PrometheusStrict(t *testing.T) {
	client := newFakeClientWithRoutes(
		newRoute(MonitoringNamespace, PrometheusRouteName, "prometheus.apps.example.com"),
		newRoute(MonitoringNamespace, ThanosQuerierRouteName, "thanos.apps.example.com"),
	)
	resolver := &MetricsRouteResolver{}
	url, err := resolver.ResolveMetricsBackend(context.Background(), client, "prometheus")
	require.NoError(t, err)
	require.Equal(t, "https://prometheus.apps.example.com", url)
}

func TestMetricsRouteResolver_Alertmanager(t *testing.T) {
	client := newFakeClientWithRoutes(
		newRoute(MonitoringNamespace, AlertmanagerRouteName, "alertmanager.apps.example.com"),
	)
	resolver := &MetricsRouteResolver{}
	url, err := resolver.ResolveAlertmanager(context.Background(), client)
	require.NoError(t, err)
	require.Equal(t, "https://alertmanager.apps.example.com", url)
}
