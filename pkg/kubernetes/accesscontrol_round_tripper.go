package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

// AccessControlRoundTripper intercepts HTTP requests to enforce access control
// and optionally run validators before they reach the Kubernetes API.
type AccessControlRoundTripper struct {
	delegate                http.RoundTripper
	deniedResourcesProvider api.DeniedResourcesProvider
	restMapperProvider      func() meta.RESTMapper
	rawDiscovery            *rawDiscoveryCache
	apiPathPrefix           string
	validators              []api.HTTPValidator
}

// AccessControlRoundTripperConfig configures the AccessControlRoundTripper.
type AccessControlRoundTripperConfig struct {
	Delegate                  http.RoundTripper
	DeniedResourcesProvider   api.DeniedResourcesProvider
	RestMapperProvider        func() meta.RESTMapper
	HostURL                   string
	DiscoveryProvider         func() discovery.DiscoveryInterface
	RawDiscoveryProvider      func() discovery.DiscoveryInterface
	AuthClientProvider        func() authv1client.AuthorizationV1Interface
	ValidationEnabled         bool
	ConfirmationRulesProvider api.ConfirmationRulesProvider
}

// NewAccessControlRoundTripper creates a new AccessControlRoundTripper.
func NewAccessControlRoundTripper(ctx context.Context, cfg AccessControlRoundTripperConfig) *AccessControlRoundTripper {
	var apiPathPrefix string
	if cfg.HostURL != "" {
		if hostURL, err := url.Parse(cfg.HostURL); err != nil {
			klogutil.LogWarn(klogutil.FromContext(ctx),
				"failed to parse Kubernetes API server host to determine API path prefix",
				klogutil.Field("url.full", cfg.HostURL),
				klogutil.Err(err),
			)
		} else {
			apiPathPrefix = hostURL.Path
		}
	}
	var rawDisc *rawDiscoveryCache
	if cfg.RawDiscoveryProvider != nil {
		if disc := cfg.RawDiscoveryProvider(); disc != nil {
			rawDisc = newRawDiscoveryCache(disc)
		}
	}
	rt := &AccessControlRoundTripper{
		delegate:                cfg.Delegate,
		deniedResourcesProvider: cfg.DeniedResourcesProvider,
		restMapperProvider:      cfg.RestMapperProvider,
		rawDiscovery:            rawDisc,
		apiPathPrefix:           apiPathPrefix,
	}

	// Schema/RBAC validators run first so the user isn't prompted for
	// confirmation on an operation that would fail anyway.
	if cfg.ValidationEnabled {
		rt.validators = append(rt.validators, CreateValidators(ValidatorProviders{
			Discovery:  cfg.DiscoveryProvider,
			AuthClient: cfg.AuthClientProvider,
		})...)
	}

	if cfg.ConfirmationRulesProvider != nil && len(cfg.ConfirmationRulesProvider.GetConfirmationRules()) > 0 {
		rt.validators = append(rt.validators, &ConfirmationValidator{rulesProvider: cfg.ConfirmationRulesProvider})
	}

	// Always enable Windows EULA validator for windows-efi-installer PipelineRuns
	rt.validators = append(rt.validators, &WindowsEULAValidator{})

	return rt
}

func (rt *AccessControlRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return rt.delegate
}

func (rt *AccessControlRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	kubernetesPath := stripAPIPathPrefix(req.URL.Path, rt.apiPathPrefix)
	gvr, ok := parseURLToGVR(kubernetesPath)
	// Not an API resource request, just pass through
	if !ok {
		return rt.delegate.RoundTrip(req)
	}

	subresource := parseSubresource(kubernetesPath)
	gvk, err := rt.getGVK(gvr, subresource)
	if err != nil {
		return nil, err
	}

	if !rt.isAllowed(gvk) {
		return nil, fmt.Errorf("resource not allowed: %s", gvk.String())
	}

	// Skip validators for SelfSubjectAccessReview to avoid recursion from RBAC validator
	if gvr.Group == "authorization.k8s.io" && gvr.Resource == "selfsubjectaccessreviews" {
		return rt.delegate.RoundTrip(req)
	}

	namespace, resourceName := parseURLToNamespaceAndName(kubernetesPath)
	verb := httpMethodToVerb(req.Method, kubernetesPath)

	validationReq := &api.HTTPValidationRequest{
		GVR:          &gvr,
		GVK:          &gvk,
		HTTPMethod:   req.Method,
		Verb:         verb,
		Namespace:    namespace,
		ResourceName: resourceName,
		Path:         kubernetesPath,
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

	logger := klogutil.FromContext(req.Context())
	for _, v := range rt.validators {
		if validationErr := v.Validate(req.Context(), validationReq); validationErr != nil {
			if ve, ok := validationErr.(*api.ValidationError); ok {
				klogutil.LogInfo(logger.V(4), "Validation failed", klogutil.Field("validator_name", v.Name()), klogutil.Err(ve))
			}
			return nil, validationErr
		}
	}

	return rt.delegate.RoundTrip(req)
}

func (rt *AccessControlRoundTripper) getGVK(
	gvr schema.GroupVersionResource,
	subresource string,
) (schema.GroupVersionKind, error) {
	// Get restMapper at request time (lazy evaluation)
	// This ensures we get the initialized restMapper even if the wrapper
	// was created before restMapper was set (fixes issue #688)
	restMapper := rt.restMapperProvider()
	if restMapper == nil {
		return schema.GroupVersionKind{},
			fmt.Errorf("failed to make request: AccessControlRoundTripper restMapper not initialized")
	}

	gvk, err := restMapper.KindFor(gvr)
	if err == nil {
		return gvk, nil
	}

	if meta.IsNoMatchError(err) {
		// Fallback: check if the group/version exists in discovery and
		// contains the resource (or resource/subresource pair).
		// This handles aggregated APIs like subresources.kubevirt.io that
		// only expose subresource endpoints with empty Kind.
		if rt.resourceExistsInDiscovery(gvr, subresource) {
			// No GVK is available for subresource-only API groups,
			// but we still check the group/version against the denied
			// resources list (the Kind="" wildcard entries).
			return schema.GroupVersionKind{Group: gvr.Group, Version: gvr.Version}, nil
		}

		return schema.GroupVersionKind{}, &api.ValidationError{
			Code: api.ErrorCodeResourceNotFound,
			Message: fmt.Sprintf(
				"Resource %s does not exist in the cluster",
				api.FormatResourceName(&gvr),
			),
		}
	}

	return schema.GroupVersionKind{},
		fmt.Errorf(
			"failed to make request: AccessControlRoundTripper failed to get kind for gvr %v: %w",
			gvr,
			err,
		)
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

// resourceExistsInDiscovery checks whether the given GVR (and optional
// subresource) corresponds to a resource advertised by the API server's
// discovery endpoint. This handles aggregated APIs like
// subresources.kubevirt.io that only expose subresource entries with empty
// Kind, which prevents the REST mapper from building GVR→GVK mappings.
//
// When subresource is non-empty, the method requires an exact match for the
// "resource/subresource" entry. When subresource is empty, any discovery entry
// that matches the resource name (either as a top-level resource or as a parent
// prefix of a subresource entry) is accepted.
func (rt *AccessControlRoundTripper) resourceExistsInDiscovery(gvr schema.GroupVersionResource, subresource string) bool {
	if rt.rawDiscovery == nil {
		return false
	}
	groupVersion := gvr.Group + "/" + gvr.Version
	if gvr.Group == "" {
		groupVersion = gvr.Version
	}
	resources, err := rt.rawDiscovery.ServerResourcesForGroupVersion(groupVersion)
	if err != nil || resources == nil {
		return false
	}
	// When a subresource is specified, look for an exact
	// "resource/subresource" entry (e.g. "virtualmachineinstances/pause").
	if subresource != "" {
		fullName := gvr.Resource + "/" + subresource
		for _, r := range resources.APIResources {
			if r.Name == fullName {
				return true
			}
		}
		return false
	}
	// No subresource — match the resource as a top-level resource or as a
	// parent of any subresource entry.
	for _, r := range resources.APIResources {
		if r.Name == gvr.Resource || strings.HasPrefix(r.Name, gvr.Resource+"/") {
			return true
		}
	}
	return false
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

func stripAPIPathPrefix(path, prefix string) string {
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" {
		return path
	}

	if path == prefix {
		return "/"
	}

	if strings.HasPrefix(path, prefix+"/") {
		return strings.TrimPrefix(path, prefix)
	}

	return path
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

// parseSubresource extracts the subresource segment from a Kubernetes API path.
// For example, given /apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/pause
// it returns "pause". Returns "" if the path has no subresource segment.
func parseSubresource(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	resourceIdx := findResourceTypeIndex(parts)
	// A subresource exists when the path has segments beyond
	// resource type (resourceIdx) + resource name (resourceIdx+1).
	if resourceIdx >= 0 && resourceIdx+2 < len(parts) {
		return parts[resourceIdx+2]
	}
	return ""
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

// rawDiscoveryCache caches per-group/version resource lists fetched from the
// raw (non-aggregated) discovery endpoint. This is needed because the
// aggregated discovery API filters out resources with empty Kind (e.g.
// subresources.kubevirt.io), causing the memory-cached discovery client to
// return empty results for those groups.
//
// Only successful lookups are cached. Errors (e.g. group not found) are not
// cached so that newly installed API groups are discovered on the next request.
// Cached entries are held indefinitely — if an API group is later removed, the
// API server will reject the request regardless.
type rawDiscoveryCache struct {
	delegate discovery.DiscoveryInterface

	mu    sync.RWMutex
	cache map[string]*metav1.APIResourceList
}

func newRawDiscoveryCache(delegate discovery.DiscoveryInterface) *rawDiscoveryCache {
	return &rawDiscoveryCache{
		delegate: delegate,
		cache:    make(map[string]*metav1.APIResourceList),
	}
}

func (c *rawDiscoveryCache) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	c.mu.RLock()
	if rl, ok := c.cache[groupVersion]; ok {
		c.mu.RUnlock()
		return rl, nil
	}
	c.mu.RUnlock()

	rl, err := c.delegate.ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		return rl, err
	}

	c.mu.Lock()
	c.cache[groupVersion] = rl
	c.mu.Unlock()
	return rl, nil
}
