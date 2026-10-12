package kubernetes

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/discovery"
	openapierrors "k8s.io/kube-openapi/pkg/util/proto/validation"
	kubectlopenapi "k8s.io/kubectl/pkg/util/openapi"
	kubectlvalidation "k8s.io/kubectl/pkg/validation"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
)

const schemaCacheTTL = 15 * time.Minute

// SchemaValidator validates resource manifests against the OpenAPI schema.
type SchemaValidator struct {
	discoveryClientProvider func() discovery.DiscoveryInterface
	cache                   *openAPISchemaCache
}

// NewSchemaValidator creates a new schema validator with its own schema cache.
func NewSchemaValidator(discoveryClientProvider func() discovery.DiscoveryInterface) *SchemaValidator {
	return newSchemaValidator(discoveryClientProvider, nil)
}

func newSchemaValidator(discoveryClientProvider func() discovery.DiscoveryInterface, cache *openAPISchemaCache) *SchemaValidator {
	if cache == nil {
		cache = newOpenAPISchemaCache()
	}
	return &SchemaValidator{
		discoveryClientProvider: discoveryClientProvider,
		cache:                   cache,
	}
}

// openAPISchemaCache is one parsed OpenAPI model for every client of a manager.
// /openapi/v2 is the apiserver's aggregated spec, so derived clients share it.
// Concurrent misses wait on one fetch. A failed fetch is not stored.
// See docs/specs/memory-bounds.md#retention.
type openAPISchemaCache struct {
	ttl      time.Duration
	mu       sync.Mutex
	schema   kubectlvalidation.Schema
	cachedAt time.Time
	flight   singleflight.Group
}

func newOpenAPISchemaCache() *openAPISchemaCache {
	return &openAPISchemaCache{ttl: schemaCacheTTL}
}

func (c *openAPISchemaCache) fresh() kubectlvalidation.Schema {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.schema == nil || c.ttl <= 0 || time.Since(c.cachedAt) > c.ttl {
		return nil
	}
	return c.schema
}

func (c *openAPISchemaCache) get(ctx context.Context, disc discovery.DiscoveryInterface) (kubectlvalidation.Schema, error) {
	if schema := c.fresh(); schema != nil {
		return schema, nil
	}
	if disc == nil {
		return nil, nil
	}
	openAPIClient, ok := disc.(discovery.OpenAPISchemaInterface)
	if !ok {
		klogutil.FromContext(ctx).V(4).Info("Discovery client does not support OpenAPI schema")
		return nil, nil
	}

	v, err, _ := c.flight.Do("openapi", func() (any, error) {
		if schema := c.fresh(); schema != nil {
			return schema, nil
		}
		parser := kubectlopenapi.NewOpenAPIParser(openAPIClient)
		resources, parseErr := parser.Parse()
		if parseErr != nil {
			return nil, parseErr
		}
		schema := kubectlvalidation.NewSchemaValidation(&parsedOpenAPI{resources: resources})
		c.mu.Lock()
		c.schema = schema
		c.cachedAt = time.Now()
		c.mu.Unlock()
		return schema, nil
	})
	if err != nil {
		return nil, err
	}
	schema, _ := v.(kubectlvalidation.Schema)
	return schema, nil
}

// parsedOpenAPI serves a model that has already been parsed, so holders of the
// cached schema do not each keep a parser or a second copy of the document.
type parsedOpenAPI struct {
	resources kubectlopenapi.Resources
}

var _ kubectlopenapi.OpenAPIResourcesGetter = (*parsedOpenAPI)(nil)

func (p *parsedOpenAPI) OpenAPISchema() (kubectlopenapi.Resources, error) {
	return p.resources, nil
}

func (v *SchemaValidator) Name() string {
	return "schema"
}

func (v *SchemaValidator) Validate(ctx context.Context, req *api.HTTPValidationRequest) error {
	if req.GVK == nil || len(req.Body) == 0 {
		return nil
	}

	logger := klogutil.FromContext(ctx)

	// Only validate for create/update operations (exclude patch as partial bodies cause false positives)
	if req.Verb != "create" && req.Verb != "update" {
		return nil
	}

	validator, err := v.cache.get(ctx, v.discoveryClientProvider())
	if err != nil {
		klogutil.LogInfo(logger.V(4), "Failed to get schema validator", klogutil.Err(err))
		return convertKubectlValidationError(err)
	}

	if validator == nil {
		return nil
	}

	err = validator.ValidateBytes(req.Body)
	if err != nil {
		// Check if this is a parsing error (e.g., binary data that can't be parsed as YAML)
		// In that case, skip validation rather than blocking the request
		errMsg := err.Error()
		if strings.Contains(errMsg, "yaml:") || strings.Contains(errMsg, "json:") {
			klogutil.LogInfo(logger.V(4), "Schema validation skipped due to parsing error", klogutil.Err(err))
			return nil
		}
		return convertKubectlValidationError(err)
	}

	return nil
}

func convertKubectlValidationError(err error) *api.ValidationError {
	if err == nil {
		return nil
	}

	var field string
	var agg utilerrors.Aggregate
	if errors.As(err, &agg) {
		for _, e := range agg.Errors() {
			if f := extractUnknownField(e); f != "" {
				field = f
				break
			}
		}
	} else {
		field = extractUnknownField(err)
	}

	return &api.ValidationError{
		Code:    api.ErrorCodeInvalidField,
		Message: err.Error(),
		Field:   field,
	}
}

// extractUnknownField extracts the field name from an UnknownFieldError.
// The error may be wrapped in a ValidationError from kube-openapi, which
// does not implement Unwrap(), so we check both the error itself and the
// wrapped Err field.
func extractUnknownField(err error) string {
	var unknownField openapierrors.UnknownFieldError
	if errors.As(err, &unknownField) {
		return unknownField.Field
	}
	var validationErr openapierrors.ValidationError
	if errors.As(err, &validationErr) {
		if errors.As(validationErr.Err, &unknownField) {
			return unknownField.Field
		}
	}
	return ""
}
