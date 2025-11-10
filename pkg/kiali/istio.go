package kiali

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// IstioConfig calls the Kiali Istio config API to get all Istio objects in the mesh.
// Returns the full YAML resources and additional details about each object.
func (k *Kiali) IstioConfig(ctx context.Context) (string, error) {
	endpoint := IstioConfigEndpoint + "?validate=true"

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}

// IstioObjectDetails returns detailed information about a specific Istio object.
// Parameters:
//   - namespace: the namespace containing the Istio object
//   - group: the API group (e.g., "networking.istio.io", "gateway.networking.k8s.io")
//   - version: the API version (e.g., "v1", "v1beta1")
//   - kind: the resource kind (e.g., "DestinationRule", "VirtualService", "HTTPRoute")
//   - name: the name of the resource
func (k *Kiali) IstioObjectDetails(ctx context.Context, namespace, group, version, kind, name string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if group == "" {
		return "", fmt.Errorf("group is required")
	}
	if version == "" {
		return "", fmt.Errorf("version is required")
	}
	if kind == "" {
		return "", fmt.Errorf("kind is required")
	}
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	endpoint := fmt.Sprintf(IstioObjectEndpoint+"?validate=true&help=true",
		url.PathEscape(namespace),
		url.PathEscape(group),
		url.PathEscape(version),
		url.PathEscape(kind),
		url.PathEscape(name))

	return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
}

// IstioObjectPatch patches an existing Istio object using PATCH method.
// Parameters:
//   - namespace: the namespace containing the Istio object
//   - group: the API group (e.g., "networking.istio.io", "gateway.networking.k8s.io")
//   - version: the API version (e.g., "v1", "v1beta1")
//   - kind: the resource kind (e.g., "DestinationRule", "VirtualService", "HTTPRoute")
//   - name: the name of the resource
//   - jsonPatch: the JSON patch data to apply
func (k *Kiali) IstioObjectPatch(ctx context.Context, namespace, group, version, kind, name, jsonPatch string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if group == "" {
		return "", fmt.Errorf("group is required")
	}
	if version == "" {
		return "", fmt.Errorf("version is required")
	}
	if kind == "" {
		return "", fmt.Errorf("kind is required")
	}
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if jsonPatch == "" {
		return "", fmt.Errorf("json patch data is required")
	}
	endpoint := fmt.Sprintf(IstioObjectEndpoint,
		url.PathEscape(namespace),
		url.PathEscape(group),
		url.PathEscape(version),
		url.PathEscape(kind),
		url.PathEscape(name))

	return k.executeRequest(ctx, http.MethodPatch, endpoint, "application/json", strings.NewReader(jsonPatch))
}

// IstioObjectCreate creates a new Istio object using POST method.
// Parameters:
//   - namespace: the namespace where the Istio object will be created
//   - group: the API group (e.g., "networking.istio.io", "gateway.networking.k8s.io")
//   - version: the API version (e.g., "v1", "v1beta1")
//   - kind: the resource kind (e.g., "DestinationRule", "VirtualService", "HTTPRoute")
//   - jsonData: the JSON data for the new object
func (k *Kiali) IstioObjectCreate(ctx context.Context, namespace, group, version, kind, jsonData string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if group == "" {
		return "", fmt.Errorf("group is required")
	}
	if version == "" {
		return "", fmt.Errorf("version is required")
	}
	if kind == "" {
		return "", fmt.Errorf("kind is required")
	}
	if jsonData == "" {
		return "", fmt.Errorf("json data is required")
	}
	endpoint := fmt.Sprintf(IstioObjectCreateEndpoint,
		url.PathEscape(namespace),
		url.PathEscape(group),
		url.PathEscape(version),
		url.PathEscape(kind))

	return k.executeRequest(ctx, http.MethodPost, endpoint, "application/json", strings.NewReader(jsonData))
}

// IstioObjectDelete deletes an existing Istio object using DELETE method.
// Parameters:
//   - namespace: the namespace containing the Istio object
//   - group: the API group (e.g., "networking.istio.io", "gateway.networking.k8s.io")
//   - version: the API version (e.g., "v1", "v1beta1")
//   - kind: the resource kind (e.g., "DestinationRule", "VirtualService", "HTTPRoute", "Gateway")
//   - name: the name of the resource
func (k *Kiali) IstioObjectDelete(ctx context.Context, namespace, group, version, kind, name string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if group == "" {
		return "", fmt.Errorf("group is required")
	}
	if version == "" {
		return "", fmt.Errorf("version is required")
	}
	if kind == "" {
		return "", fmt.Errorf("kind is required")
	}
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	endpoint := fmt.Sprintf(IstioObjectEndpoint,
		url.PathEscape(namespace),
		url.PathEscape(group),
		url.PathEscape(version),
		url.PathEscape(kind),
		url.PathEscape(name))

	return k.executeRequest(ctx, http.MethodDelete, endpoint, "", nil)
}
