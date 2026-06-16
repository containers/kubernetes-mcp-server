package discovery

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	lokiStackGVR = schema.GroupVersionResource{
		Group:    "loki.grafana.com",
		Version:  "v1",
		Resource: "lokistacks",
	}
	routeGVR = schema.GroupVersionResource{
		Group:    "route.openshift.io",
		Version:  "v1",
		Resource: "routes",
	}
)

// LokiStack represents the LokiStack CR.
type LokiStack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              LokiStackSpec   `json:"spec"`
	Status            LokiStackStatus `json:"status"`
}

type LokiStackSpec struct {
	Tenants *LokiStackTenants `json:"tenants,omitempty"`
}

type LokiStackTenants struct {
	Mode string `json:"mode,omitempty"`
}

type LokiStackStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Route represents the OpenShift Route CR.
type Route struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              RouteSpec `json:"spec"`
}

type RouteSpec struct {
	Host string `json:"host,omitempty"`
}
