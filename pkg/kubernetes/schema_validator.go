package kubernetes

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/client-go/discovery"
	"k8s.io/klog/v2"
	kubectlopenapi "k8s.io/kubectl/pkg/util/openapi"
	kubectlvalidation "k8s.io/kubectl/pkg/validation"
)

const schemaCacheTTL = 15 * time.Minute

// SchemaValidator validates resource manifests against the OpenAPI schema.
type SchemaValidator struct {
	discoveryClientProvider func() discovery.DiscoveryInterface
	kubectlValidator        kubectlvalidation.Schema
	validatorMu             sync.Mutex
	validatorCachedAt       time.Time
}

// NewSchemaValidator creates a new schema validator.
func NewSchemaValidator(discoveryClientProvider func() discovery.DiscoveryInterface) *SchemaValidator {
	return &SchemaValidator{
		discoveryClientProvider: discoveryClientProvider,
	}
}

func (v *SchemaValidator) Name() string {
	return "schema"
}

func (v *SchemaValidator) Validate(ctx context.Context, req *api.HTTPValidationRequest) error {
	if req.GVK == nil || len(req.Body) == 0 {
		return nil
	}

	// Only validate for create/update operations (exclude patch as partial bodies cause false positives)
	if req.Verb != "create" && req.Verb != "update" {
		return nil
	}

	validator, err := v.getValidator()
	if err != nil {
		klog.V(4).Infof("Failed to get schema validator: %v", err)
		return nil
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
			klog.V(4).Infof("Schema validation skipped due to parsing error: %v", err)
			return nil
		}
		return convertKubectlValidationError(err)
	}

	return nil
}

// openAPIResourcesAdapter adapts CachedOpenAPIParser to OpenAPIResourcesGetter interface.
type openAPIResourcesAdapter struct {
	parser *kubectlopenapi.CachedOpenAPIParser
}

func (a *openAPIResourcesAdapter) OpenAPISchema() (kubectlopenapi.Resources, error) {
	return a.parser.Parse()
}

func (v *SchemaValidator) getValidator() (kubectlvalidation.Schema, error) {
	v.validatorMu.Lock()
	defer v.validatorMu.Unlock()

	if v.kubectlValidator != nil && time.Since(v.validatorCachedAt) <= schemaCacheTTL {
		return v.kubectlValidator, nil
	}

	discoveryClient := v.discoveryClientProvider()
	if discoveryClient == nil {
		return nil, nil
	}

	openAPIClient, ok := discoveryClient.(discovery.OpenAPISchemaInterface)
	if !ok {
		klog.V(4).Infof("Discovery client does not support OpenAPI schema")
		return nil, nil
	}

	parser := kubectlopenapi.NewOpenAPIParser(openAPIClient)
	adapter := &openAPIResourcesAdapter{parser: parser}

	v.kubectlValidator = kubectlvalidation.NewSchemaValidation(adapter)
	v.validatorCachedAt = time.Now()

	return v.kubectlValidator, nil
}

func convertKubectlValidationError(err error) *api.ValidationError {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	var field string
	if strings.Contains(errMsg, "unknown field") {
		if start := strings.Index(errMsg, "\""); start != -1 {
			if end := strings.Index(errMsg[start+1:], "\""); end != -1 {
				field = errMsg[start+1 : start+1+end]
			}
		}
	}

	return &api.ValidationError{
		Code:    api.ErrorCodeInvalidField,
		Message: errMsg,
		Field:   field,
	}
}
