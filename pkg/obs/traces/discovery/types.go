package discovery

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	tempoStackGVR = schema.GroupVersionResource{
		Group:    "tempo.grafana.com",
		Version:  "v1alpha1",
		Resource: "tempostacks",
	}
	tempoMonolithicGVR = schema.GroupVersionResource{
		Group:    "tempo.grafana.com",
		Version:  "v1alpha1",
		Resource: "tempomonolithics",
	}
	routeGVR = schema.GroupVersionResource{
		Group:    "route.openshift.io",
		Version:  "v1",
		Resource: "routes",
	}
)

// TempoStack represents the TempoStack CR
type TempoStack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              TempoStackSpec   `json:"spec"`
	Status            TempoStackStatus `json:"status"`
}

type TempoStackSpec struct {
	Tenants *TempoStackTenants `json:"tenants,omitempty"`
}

type TempoStackTenants struct {
	Mode           string                 `json:"mode,omitempty"`
	Authentication []TenantAuthentication `json:"authentication,omitempty"`
}

type TenantAuthentication struct {
	TenantName string `json:"tenantName,omitempty"`
}

type TempoStackStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// TempoMonolithic represents the TempoMonolithic CR
type TempoMonolithic struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              TempoMonolithicSpec   `json:"spec"`
	Status            TempoMonolithicStatus `json:"status"`
}

type TempoMonolithicSpec struct {
	Multitenancy *TempoMonolithicMultitenancy `json:"multitenancy,omitempty"`
}

type TempoMonolithicMultitenancy struct {
	Enabled        bool                   `json:"enabled,omitempty"`
	Mode           string                 `json:"mode,omitempty"`
	Authentication []TenantAuthentication `json:"authentication,omitempty"`
}

type TempoMonolithicStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Route represents the OpenShift Route CR
type Route struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              RouteSpec `json:"spec"`
}

type RouteSpec struct {
	Host string `json:"host,omitempty"`
}
