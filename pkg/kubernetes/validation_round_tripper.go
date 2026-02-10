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

// ValidationRoundTripper intercepts HTTP requests to validate them before they reach the K8s API.
// It extends the AccessControlRoundTripper pattern with additional validator chain execution.
type ValidationRoundTripper struct {
	delegate                http.RoundTripper
	deniedResourcesProvider api.DeniedResourcesProvider
	restMapperProvider      func() meta.RESTMapper
	discoveryProvider       func() discovery.DiscoveryInterface
	authClientProvider      func() authv1client.AuthorizationV1Interface
	validatorConfig         api.ValidationConfig
	validators              []api.HTTPValidator
}

// ValidationRoundTripperConfig configures the ValidationRoundTripper.
type ValidationRoundTripperConfig struct {
	Delegate                http.RoundTripper
	DeniedResourcesProvider api.DeniedResourcesProvider
	RestMapperProvider      func() meta.RESTMapper
	DiscoveryProvider       func() discovery.DiscoveryInterface
	AuthClientProvider      func() authv1client.AuthorizationV1Interface
	ValidatorConfig         api.ValidationConfig
}

// NewValidationRoundTripper creates a new ValidationRoundTripper.
func NewValidationRoundTripper(cfg ValidationRoundTripperConfig) *ValidationRoundTripper {
	rt := &ValidationRoundTripper{
		delegate:                cfg.Delegate,
		deniedResourcesProvider: cfg.DeniedResourcesProvider,
		restMapperProvider:      cfg.RestMapperProvider,
		discoveryProvider:       cfg.DiscoveryProvider,
		authClientProvider:      cfg.AuthClientProvider,
		validatorConfig:         cfg.ValidatorConfig,
	}

	// Create validators with providers (only if validation is enabled)
	if cfg.ValidatorConfig != nil && cfg.ValidatorConfig.IsEnabled() {
		rt.validators = CreateValidators(ValidatorProviders{
			RestMapper: cfg.RestMapperProvider,
			Discovery:  cfg.DiscoveryProvider,
			AuthClient: cfg.AuthClientProvider,
		})
	}

	return rt
}

func (rt *ValidationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	gvr, ok := parseURLToGVR(req.URL.Path)
	if !ok {
		return rt.delegate.RoundTrip(req)
	}

	// Access control checks (from original AccessControlRoundTripper) - always enforced
	restMapper := rt.restMapperProvider()
	if restMapper == nil {
		return nil, fmt.Errorf("failed to make request: restMapper not initialized")
	}

	gvk, err := restMapper.KindFor(gvr)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: failed to get kind for gvr %v: %w", gvr, err)
	}

	if !rt.isAllowed(gvk) {
		return nil, fmt.Errorf("resource not allowed: %s", gvk.String())
	}

	// Skip validators if disabled or if this is SelfSubjectAccessReview (used by RBAC validator)
	skipValidation := rt.validatorConfig == nil || !rt.validatorConfig.IsEnabled()
	skipValidation = skipValidation || (gvr.Group == "authorization.k8s.io" && gvr.Resource == "selfsubjectaccessreviews")
	if skipValidation {
		return rt.delegate.RoundTrip(req)
	}

	// Extract namespace and resource name from URL
	namespace, resourceName := parseURLToNamespaceAndName(req.URL.Path)

	// Map HTTP method to K8s verb
	verb := httpMethodToVerb(req.Method, req.URL.Path)

	// Build validation request
	validationReq := &api.HTTPValidationRequest{
		GVR:          &gvr,
		GVK:          &gvk,
		HTTPMethod:   req.Method,
		Verb:         verb,
		Namespace:    namespace,
		ResourceName: resourceName,
		Path:         req.URL.Path,
	}

	// Buffer body for POST/PUT/PATCH (needed for schema validation)
	if req.Body != nil && (req.Method == "POST" || req.Method == "PUT" || req.Method == "PATCH") {
		body, readErr := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read request body: %w", readErr)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		validationReq.Body = body
	}

	// Run validators
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
func (rt *ValidationRoundTripper) isAllowed(gvk schema.GroupVersionKind) bool {
	if rt.deniedResourcesProvider == nil {
		return true
	}

	for _, val := range rt.deniedResourcesProvider.GetDeniedResources() {
		if val.Kind == "" {
			if gvk.Group == val.Group && gvk.Version == val.Version {
				return false
			}
		}
		if gvk.Group == val.Group && gvk.Version == val.Version && gvk.Kind == val.Kind {
			return false
		}
	}

	return true
}

// parseURLToNamespaceAndName extracts namespace and resource name from K8s API URL.
func parseURLToNamespaceAndName(path string) (namespace, name string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Find "namespaces" segment
	for i, part := range parts {
		if part == "namespaces" && i+1 < len(parts) {
			namespace = parts[i+1]
			break
		}
	}

	// Find resource type position and check for name after it
	resourceIdx := findResourceTypeIndex(parts)
	if resourceIdx >= 0 && resourceIdx+1 < len(parts) {
		name = parts[resourceIdx+1]
	}

	return namespace, name
}

// findResourceTypeIndex returns the index of the resource type segment in the URL parts.
// Uses the standard K8s API URL structure to locate the resource type.
func findResourceTypeIndex(parts []string) int {
	if len(parts) == 0 {
		return -1
	}

	switch parts[0] {
	case "api":
		// /api/v1/{resource} or /api/v1/namespaces/{ns}/{resource}
		if len(parts) < 3 {
			return -1
		}
		if parts[2] == "namespaces" && len(parts) > 4 {
			return 4
		}
		return 2
	case "apis":
		// /apis/{group}/{version}/{resource} or /apis/{group}/{version}/namespaces/{ns}/{resource}
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

// httpMethodToVerb maps HTTP method to K8s RBAC verb.
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

// isCollectionPath checks if the path targets a collection (list) vs a specific resource.
func isCollectionPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	resourceIdx := findResourceTypeIndex(parts)
	if resourceIdx < 0 {
		return false
	}
	// Collection path if the resource type is the last segment (no name after it)
	return resourceIdx == len(parts)-1
}
