package openshift

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"

	logsdiscovery "github.com/rhobs/obs-mcp/pkg/logs/discovery"
)

// LogsGatewayResolver implements logs/discovery.GatewayResolver using OpenShift Routes.
type LogsGatewayResolver struct {
	RouteClient
}

var _ logsdiscovery.GatewayResolver = (*LogsGatewayResolver)(nil)

// ResolveGatewayURL resolves the complete base URL for a LokiStack gateway.
//
// Resolution order:
//  1. Route named stackName
//  2. Route named <stackName>-gateway-http
//  3. Any Route in the namespace whose spec.to.name is <stackName>-gateway-http
//  4. HTTPS service DNS when tenantsMode has the OpenShift prefix
//  5. HTTP service DNS otherwise
//
// The returned URL includes /api/logs/v1 when a route or OpenShift tenants mode is active.
func (r *LogsGatewayResolver) ResolveGatewayURL(ctx context.Context, client dynamic.Interface, namespace, stackName, tenantsMode string) (string, error) {
	gatewaySvcName := fmt.Sprintf("%s-gateway-http", stackName)

	for _, routeName := range []string{stackName, gatewaySvcName} {
		host, err := r.GetRouteHost(ctx, client, namespace, routeName)
		if err == nil {
			return fmt.Sprintf("https://%s/api/logs/v1", host), nil
		}
		if !apierrors.IsNotFound(err) {
			return "", fmt.Errorf("failed to look up route %s/%s: %w", namespace, routeName, err)
		}
	}

	if host, err := r.FindRouteByTargetService(ctx, client, namespace, gatewaySvcName); err == nil {
		return fmt.Sprintf("https://%s/api/logs/v1", host), nil
	} else {
		slog.Debug("No route found by target service, falling back to service DNS", "namespace", namespace, "service", gatewaySvcName, "error", err)
	}

	if strings.HasPrefix(tenantsMode, OpenShiftTenantModePrefix) {
		return fmt.Sprintf("https://%s.%s.svc:8080/api/logs/v1", gatewaySvcName, namespace), nil
	}
	return fmt.Sprintf("http://%s.%s.svc:8080", gatewaySvcName, namespace), nil
}
