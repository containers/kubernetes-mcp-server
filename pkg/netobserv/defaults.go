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

	// DefaultPluginHTTPTimeout bounds waits for the console plugin HTTP API (Loki/Prometheus work behind it).
	DefaultPluginHTTPTimeout = 120 * time.Second
)

// DefaultPluginServiceCAPath is the OpenShift-projected service-serving CA bundle.
const DefaultPluginServiceCAPath = "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"

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
