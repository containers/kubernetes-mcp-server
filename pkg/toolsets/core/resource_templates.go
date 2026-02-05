package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	apiextensionsv1spec "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func initResourceTemplates() []api.ServerResourceTemplate {
	return []api.ServerResourceTemplate{
		crdOpenAPISpecResourceTemplate(),
	}
}

// crdOpenAPISpecResourceTemplate returns a resource template that provides the OpenAPI spec for a CRD.
// The URI format is: k8s://crds/{name}/openapi
// Where {name} is the full CRD name (e.g., "virtualmachines.kubevirt.io")
func crdOpenAPISpecResourceTemplate() api.ServerResourceTemplate {
	return api.ServerResourceTemplate{
		ResourceTemplate: api.ResourceTemplate{
			Name:        "crd-openapi-spec",
			Title:       "CRD OpenAPI Specification",
			Description: "Returns the OpenAPI v3 schema for a Custom Resource Definition (CRD). The schema describes the structure and validation rules for custom resources of this type. Use this to figure out how to structure resource tool calls when you are unsure of the schema",
			URITemplate: "k8s://crds/{name}/openapi",
			MIMEType:    "application/json",
			Annotations: &api.ResourceAnnotations{
				Audience: []string{"assistant"},
				Priority: 0.5,
			},
		},
		Handler: handleCRDOpenAPISpec,
	}
}

func handleCRDOpenAPISpec(params api.ResourceHandlerParams) (*api.ResourceCallResult, error) {
	// Expected format: k8s://crds/{name}/openapi
	crdName, err := parseCRDNameFromURI(params.URI)
	if err != nil {
		return nil, err
	}

	apiExtClient, err := apiextensionsv1.NewForConfig(params.RESTConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create apiextensions client: %w", err)
	}

	crd, err := apiExtClient.CustomResourceDefinitions().Get(params.Context, crdName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get CRD %q: %w", crdName, err)
	}

	response := buildCRDOpenAPIResponse(crd.Spec.Group, crd.Spec.Names.Kind, crd.Spec.Versions)

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAPI spec: %w", err)
	}

	return api.NewResourceTextResult(params.URI, "application/json", string(jsonBytes)), nil
}

// parseCRDNameFromURI extracts the CRD name from a URI of the form k8s://crds/{name}/openapi
func parseCRDNameFromURI(uri string) (string, error) {
	// Remove the scheme prefix
	const prefix = "k8s://crds/"
	const suffix = "/openapi"

	if !strings.HasPrefix(uri, prefix) {
		return "", fmt.Errorf("invalid URI format: expected prefix %q, got %q", prefix, uri)
	}

	rest := strings.TrimPrefix(uri, prefix)

	if !strings.HasSuffix(rest, suffix) {
		return "", fmt.Errorf("invalid URI format: expected suffix %q in %q", suffix, uri)
	}

	name := strings.TrimSuffix(rest, suffix)
	if name == "" {
		return "", fmt.Errorf("CRD name cannot be empty")
	}

	return name, nil
}

// CRDOpenAPIResponse represents the OpenAPI spec response for a CRD
type CRDOpenAPIResponse struct {
	Group    string                  `json:"group"`
	Kind     string                  `json:"kind"`
	Versions []CRDVersionOpenAPISpec `json:"versions"`
}

// CRDVersionOpenAPISpec represents the OpenAPI spec for a specific CRD version
type CRDVersionOpenAPISpec struct {
	Name          string `json:"name"`
	Served        bool   `json:"served"`
	Storage       bool   `json:"storage"`
	OpenAPISchema any    `json:"openAPIV3Schema,omitempty"`
}

func buildCRDOpenAPIResponse(group, kind string, versions []apiextensionsv1spec.CustomResourceDefinitionVersion) *CRDOpenAPIResponse {
	response := &CRDOpenAPIResponse{
		Group:    group,
		Kind:     kind,
		Versions: make([]CRDVersionOpenAPISpec, 0, len(versions)),
	}

	for _, v := range versions {
		versionSpec := CRDVersionOpenAPISpec{
			Name:    v.Name,
			Served:  v.Served,
			Storage: v.Storage,
		}

		if v.Schema != nil && v.Schema.OpenAPIV3Schema != nil {
			versionSpec.OpenAPISchema = v.Schema.OpenAPIV3Schema
		}

		response.Versions = append(response.Versions, versionSpec)
	}

	return response
}
