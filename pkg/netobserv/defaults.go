package netobserv

import (
	"fmt"
	"time"
)

// Defaults match netobserv-operator (PluginName, DefaultOperatorNamespace, advanced.port).
const (
	DefaultPluginNamespace = "netobserv"
	DefaultPluginService   = "netobserv-plugin"
	DefaultPluginPort      = 9001

	// OpenShift cluster monitoring URLs (consoleplugin_objects.go prom auto mode).
	// Trailing dot on cluster.local is a DNS optimization used by the operator.
	DefaultOpenShiftPrometheusURL       = "https://thanos-querier.openshift-monitoring.svc.cluster.local.:9091"
	DefaultOpenShiftAlertmanagerURL     = "https://alertmanager-main.openshift-monitoring.svc.cluster.local.:9094"
	DefaultOpenShiftMonitoringNamespace = "openshift-monitoring"

	// DefaultPluginHTTPTimeout bounds waits for the console plugin HTTP API (Loki/Prometheus work behind it).
	DefaultPluginHTTPTimeout = 120 * time.Second
)

// DefaultPluginServiceCAPath is the OpenShift service CA bundle path when mounted into
// the pod (same convention as OpenShift platform components).
const DefaultPluginServiceCAPath = "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"

// DefaultPluginInsecureSkipVerify is used on OpenShift when the service CA file is not present.
const DefaultPluginInsecureSkipVerify = true

// DefaultPluginURL returns the in-cluster Service URL using HTTPS on OpenShift and HTTP otherwise.
func DefaultPluginURL(isOpenShift bool) string {
	return BuildPluginURL(DefaultPluginNamespace, DefaultPluginService, DefaultPluginPort, isOpenShift)
}

// BuildPluginURL builds a URL for the console plugin backend Service.
func BuildPluginURL(namespace, service string, port int, isOpenShift bool) string {
	scheme := "http"
	if isOpenShift {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d", scheme, service, namespace, port)
}
