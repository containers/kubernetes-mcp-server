package kubernetes

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AccessControlRoundTripper struct {
	delegate     http.RoundTripper
	staticConfig *config.StaticConfig
	restMapper   meta.RESTMapper
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
	if !isAllowed(rt.staticConfig, &gvk) {
		return nil, isNotAllowedError(&gvk)
	}

	return rt.delegate.RoundTrip(req)
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
