package kiali

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type IstioConfigListResponse struct {
	Configs     json.RawMessage `json:"configs"`
	Validations json.RawMessage `json:"validations"`
}

// Helper builders to avoid repeated url.PathEscape boilerplate
func buildIstioObjectEndpoint(namespace, group, version, kind, name string) string {
	return fmt.Sprintf(IstioObjectEndpoint,
		url.PathEscape(namespace),
		url.PathEscape(group),
		url.PathEscape(version),
		url.PathEscape(kind),
		url.PathEscape(name),
	)
}

func buildIstioObjectCreateEndpoint(namespace, group, version, kind string) string {
	return fmt.Sprintf(IstioObjectCreateEndpoint,
		url.PathEscape(namespace),
		url.PathEscape(group),
		url.PathEscape(version),
		url.PathEscape(kind),
	)
}

// IstioConfig calls the Kiali Istio config API to get all Istio objects in the mesh.
// Returns the full YAML resources and additional details about each object.
func (k *Kiali) IstioConfig(ctx context.Context, action string, namespace string, group string, version string, kind string, name string, jsonData string) (string, error) {
	switch action {
	case "get":
		endpoint := buildIstioObjectEndpoint(namespace, group, version, kind, name) + "?validate=true&help=true"
		return k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
	case "create":
		endpoint := buildIstioObjectCreateEndpoint(namespace, group, version, kind)
		return k.executeRequest(ctx, http.MethodPost, endpoint, "application/json", strings.NewReader(jsonData))
	case "patch":
		endpoint := buildIstioObjectEndpoint(namespace, group, version, kind, name)
		return k.executeRequest(ctx, http.MethodPatch, endpoint, "application/json", strings.NewReader(jsonData))
	case "delete":
		endpoint := buildIstioObjectEndpoint(namespace, group, version, kind, name)
		return k.executeRequest(ctx, http.MethodDelete, endpoint, "", nil)
	default:
		var wg sync.WaitGroup
		wg.Add(2)
		var configsContent string
		var configsErr error
		var validationsContent string
		var validationsErr error

		// List configs (existing list behavior)
		go func() {
			defer wg.Done()
			endpoint := IstioConfigEndpoint + "?validate=true"
			configsContent, configsErr = k.executeRequest(ctx, http.MethodGet, endpoint, "", nil)
		}()

		// List validations, optionally scoped to provided namespace
		go func() {
			defer wg.Done()
			var namespaces []string
			if ns := strings.TrimSpace(namespace); ns != "" {
				namespaces = []string{ns}
			}
			validationsContent, validationsErr = k.ValidationsList(ctx, namespaces)
		}()

		wg.Wait()
		if configsErr != nil {
			return "", configsErr
		}
		if validationsErr != nil {
			return "", validationsErr
		}

		resp := IstioConfigListResponse{
			Configs:     json.RawMessage([]byte(configsContent)),
			Validations: json.RawMessage([]byte(validationsContent)),
		}
		out, err := json.Marshal(resp)
		if err != nil {
			return "", fmt.Errorf("failed to marshal istio list response: %w", err)
		}
		return string(out), nil
	}
}
