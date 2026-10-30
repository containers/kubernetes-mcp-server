package openshift

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	MonitoringNamespace    = "openshift-monitoring"
	ThanosQuerierRouteName = "thanos-querier"
	PrometheusRouteName    = "prometheus-k8s"
	AlertmanagerRouteName  = "alertmanager-main"

	// OpenShiftTenantModePrefix is the prefix for LokiStack/Tempo tenants mode strings
	// that indicate OpenShift-managed multi-tenancy (e.g., "openshift-network").
	// When present, HTTPS and the /api/logs/v1 gateway path prefix are required.
	OpenShiftTenantModePrefix = "openshift-"
)

var RouteGVR = schema.GroupVersionResource{
	Group:    "route.openshift.io",
	Version:  "v1",
	Resource: "routes",
}

type Route struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              RouteSpec `json:"spec"`
}

type RouteSpec struct {
	Host string  `json:"host,omitempty"`
	To   RouteTo `json:"to"`
}

type RouteTo struct {
	Name string `json:"name,omitempty"`
}
