package kubernetes

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AccessControlRoundTripper struct {
	delegate                http.RoundTripper
	deniedResourcesProvider api.DeniedResourcesProvider
	restMapper              meta.RESTMapper
}

func (rt *AccessControlRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	gvr, ok := parseURLToGVR(req.URL.Path)
	// Not an API resource request, just pass through
	if !ok {
		return rt.delegate.RoundTrip(req)
	}

	gvk, err := rt.restMapper.KindFor(gvr)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: AccessControlRoundTripper failed to get kind for gvr %v: %w", gvr, err)
	}
	if !rt.isAllowed(gvk) {
		return nil, fmt.Errorf("resource not allowed: %s", gvk.String())
	}

	return rt.delegate.RoundTrip(req)
}

// isAllowed checks the resource is in denied list or not.
// If it is in denied list, this function returns false.
func (rt *AccessControlRoundTripper) isAllowed(
	gvk schema.GroupVersionKind,
) bool {
	if rt.deniedResourcesProvider == nil {
		return true
	}

	for _, val := range rt.deniedResourcesProvider.GetDeniedResources() {
		// If kind is empty, that means Group/Version pair is denied entirely
		if val.Kind == "" {
			if gvk.Group == val.Group && gvk.Version == val.Version {
				return false
			}
		}
		if gvk.Group == val.Group &&
			gvk.Version == val.Version &&
			gvk.Kind == val.Kind {
			return false
		}
	}

	return true
}

func parseURLToGVR(path string) (gvr schema.GroupVersionResource, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	gvr = schema.GroupVersionResource{}
	switch parts[0] {
	case "api":
		// /api or /api/v1 are discovery endpoints
		if len(parts) < 3 {
			return
		}
		gvr.Group = ""
		gvr.Version = parts[1]
		if parts[2] == "namespaces" && len(parts) > 4 {
			gvr.Resource = parts[4]
		} else {
			gvr.Resource = parts[2]
		}
	case "apis":
		// /apis, /apis/apps, or /apis/apps/v1 are discovery endpoints
		if len(parts) < 4 {
			return
		}
		gvr.Group = parts[1]
		gvr.Version = parts[2]
		if parts[3] == "namespaces" && len(parts) > 5 {
			gvr.Resource = parts[5]
		} else {
			gvr.Resource = parts[3]
		}
	default:
		return
	}
	return gvr, true
}
