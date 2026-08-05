package openshift

import (
	"context"
	"log/slog"

	"k8s.io/client-go/dynamic"

	"github.com/rhobs/obs-mcp/pkg/metrics"
)

// MetricsRouteResolver implements metrics.BackendResolver using OpenShift Routes
// in the openshift-monitoring namespace.
type MetricsRouteResolver struct {
	RouteClient
}

var _ metrics.BackendResolver = (*MetricsRouteResolver)(nil)

// ResolveMetricsBackend resolves the metrics backend URL from openshift-monitoring.
// backend "thanos" tries Thanos first then falls back to Prometheus; "prometheus" is strict.
func (r *MetricsRouteResolver) ResolveMetricsBackend(ctx context.Context, client dynamic.Interface, backend string) (string, error) {
	if backend == "prometheus" {
		return r.resolveMonitoringRoute(ctx, client, PrometheusRouteName)
	}
	url, err := r.resolveMonitoringRoute(ctx, client, ThanosQuerierRouteName)
	if err == nil {
		return url, nil
	}
	slog.Info("Thanos route not found, falling back to prometheus", "error", err)
	return r.resolveMonitoringRoute(ctx, client, PrometheusRouteName)
}

func (r *MetricsRouteResolver) ResolveAlertmanager(ctx context.Context, client dynamic.Interface) (string, error) {
	return r.resolveMonitoringRoute(ctx, client, AlertmanagerRouteName)
}

func (r *MetricsRouteResolver) resolveMonitoringRoute(ctx context.Context, client dynamic.Interface, routeName string) (string, error) {
	url, err := r.ResolveEndpoint(ctx, client, MonitoringNamespace, routeName)
	if err != nil {
		slog.Error("Failed to discover route", "route", routeName, "error", err)
		return "", err
	}
	slog.Info("Successfully discovered route", "route", routeName, "url", url)
	return url, nil
}
