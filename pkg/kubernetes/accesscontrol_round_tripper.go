package kubernetes

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/klog/v2"
)

// AccessControlRoundTripper intercepts HTTP requests to enforce access control
// and optionally run validators before they reach the Kubernetes API.
type AccessControlRoundTripper struct {
	delegate                http.RoundTripper
	deniedResourcesProvider api.DeniedResourcesProvider
	restMapperProvider      func() meta.RESTMapper
	validationEnabled       bool
	validators              []api.HTTPValidator
}

// AccessControlRoundTripperConfig configures the AccessControlRoundTripper.
type AccessControlRoundTripperConfig struct {
	Delegate                http.RoundTripper
	DeniedResourcesProvider api.DeniedResourcesProvider
	RestMapperProvider      func() meta.RESTMapper
	DiscoveryProvider       func() discovery.DiscoveryInterface
	AuthClientProvider      func() authv1client.AuthorizationV1Interface
	ValidationEnabled       bool
}

// NewAccessControlRoundTripper creates a new AccessControlRoundTripper.
func NewAccessControlRoundTripper(cfg AccessControlRoundTripperConfig) *AccessControlRoundTripper {
	rt := &AccessControlRoundTripper{
		delegate:                cfg.Delegate,
		deniedResourcesProvider: cfg.DeniedResourcesProvider,
		restMapperProvider:      cfg.RestMapperProvider,
		validationEnabled:       cfg.ValidationEnabled,
	}

	if cfg.ValidationEnabled {
		rt.validators = CreateValidators(ValidatorProviders{
			Discovery:  cfg.DiscoveryProvider,
			AuthClient: cfg.AuthClientProvider,
		})
	}

	return rt
}

func (rt *AccessControlRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	gvr, ok := parseURLToGVR(req.URL.Path)
	// Not an API resource request, just pass through
	if !ok {
		return rt.delegate.RoundTrip(req)
	}

	// Get restMapper at request time (lazy evaluation)
	// This ensures we get the initialized restMapper even if the wrapper
	// was created before restMapper was set (fixes issue #688)
	restMapper := rt.restMapperProvider()
	if restMapper == nil {
		return nil, fmt.Errorf("failed to make request: AccessControlRoundTripper restMapper not initialized")
	}

	gvk, err := restMapper.KindFor(gvr)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return nil, &api.ValidationError{
				Code:    api.ErrorCodeResourceNotFound,
				Message: fmt.Sprintf("Resource %s does not exist in the cluster", api.FormatResourceName(&gvr)),
			}
		}
		return nil, fmt.Errorf("failed to make request: AccessControlRoundTripper failed to get kind for gvr %v: %w", gvr, err)
	}
	if !rt.isAllowed(gvk) {
		return nil, fmt.Errorf("resource not allowed: %s", gvk.String())
	}

	// Skip validators if disabled or if this is SelfSubjectAccessReview (used by RBAC validator)
	skipValidation := !rt.validationEnabled || (gvr.Group == "authorization.k8s.io" && gvr.Resource == "selfsubjectaccessreviews")
	if skipValidation {
		return rt.delegate.RoundTrip(req)
	}

	namespace, resourceName := parseURLToNamespaceAndName(req.URL.Path)
	verb := httpMethodToVerb(req.Method, req.URL.Path)

	validationReq := &api.HTTPValidationRequest{
		GVR:          &gvr,
		GVK:          &gvk,
		HTTPMethod:   req.Method,
		Verb:         verb,
		Namespace:    namespace,
		ResourceName: resourceName,
		Path:         req.URL.Path,
	}

	if req.Body != nil && (req.Method == "POST" || req.Method == "PUT" || req.Method == "PATCH") {
		body, readErr := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read request body: %w", readErr)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		validationReq.Body = body
	}

	for _, v := range rt.validators {
		if validationErr := v.Validate(req.Context(), validationReq); validationErr != nil {
			if ve, ok := validationErr.(*api.ValidationError); ok {
				klog.V(4).Infof("Validation failed [%s]: %v", v.Name(), ve)
			}
			return nil, validationErr
		}
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

func parseURLToNamespaceAndName(path string) (namespace, name string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	for i, part := range parts {
		if part == "namespaces" && i+1 < len(parts) {
			namespace = parts[i+1]
			break
		}
	}

	resourceIdx := findResourceTypeIndex(parts)
	if resourceIdx >= 0 && resourceIdx+1 < len(parts) {
		name = parts[resourceIdx+1]
	}

	return namespace, name
}

func findResourceTypeIndex(parts []string) int {
	if len(parts) == 0 {
		return -1
	}

	switch parts[0] {
	case "api":
		if len(parts) < 3 {
			return -1
		}
		if parts[2] == "namespaces" && len(parts) > 4 {
			return 4
		}
		return 2
	case "apis":
		if len(parts) < 4 {
			return -1
		}
		if parts[3] == "namespaces" && len(parts) > 5 {
			return 5
		}
		return 3
	}
	return -1
}

func httpMethodToVerb(method, path string) string {
	switch method {
	case "GET":
		if isCollectionPath(path) {
			return "list"
		}
		return "get"
	case "POST":
		return "create"
	case "PUT":
		return "update"
	case "PATCH":
		return "patch"
	case "DELETE":
		if isCollectionPath(path) {
			return "deletecollection"
		}
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

func isCollectionPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	resourceIdx := findResourceTypeIndex(parts)
	if resourceIdx < 0 {
		return false
	}
	return resourceIdx == len(parts)-1
}
