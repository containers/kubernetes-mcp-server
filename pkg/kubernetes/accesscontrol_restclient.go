package kubernetes

import (
	"fmt"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AccessControlRoundTripper struct {
	delegate                http.RoundTripper
	accessControlRESTMapper *AccessControlRESTMapper
}

func (rt *AccessControlRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	gvr, err := parseURLToGVR(req.URL.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: AccessControlRoundTripper failed to parse url: %w", err)
	}

	_, err = rt.accessControlRESTMapper.KindFor(gvr)
	if err != nil {
		return nil, fmt.Errorf("not allowed to access resource: %v", gvr)
	}

	return rt.delegate.RoundTrip(req)
}

func parseURLToGVR(path string) (schema.GroupVersionResource, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) < 3 {
		return schema.GroupVersionResource{}, fmt.Errorf("not an api path: %s", path)
	}

	gvr := schema.GroupVersionResource{}

	switch parts[0] {
	case "api":
		gvr.Group = ""
		gvr.Version = parts[1]
		if parts[2] == "namespaces" && len(parts) > 4 {
			gvr.Resource = parts[4]
		} else {
			gvr.Resource = parts[2]
		}
	case "apis":
		gvr.Group = parts[1]
		gvr.Version = parts[2]
		if parts[3] == "namespaces" && len(parts) > 5 {
			gvr.Resource = parts[5]
		} else {
			gvr.Resource = parts[3]
		}
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("unknown prefix: %s", parts[0])
	}

	return gvr, nil
}
