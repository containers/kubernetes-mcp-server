package kubernetes

import (
	"context"
	"net/http"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	openapi_v2 "github.com/google/gnostic-models/openapiv2"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

type SchemaValidatorTestSuite struct {
	suite.Suite
	mockServer      *test.MockServer
	schemaValidator *SchemaValidator
}

func (s *SchemaValidatorTestSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	s.mockServer.Handle(newOpenAPISchemaHandler())

	clientSet, err := kubernetes.NewForConfig(s.mockServer.Config())
	s.Require().NoError(err)
	discoveryClient := clientSet.Discovery()

	s.schemaValidator = NewSchemaValidator(func() discovery.DiscoveryInterface {
		return discoveryClient
	})
}

func (s *SchemaValidatorTestSuite) TearDownTest() {
	s.mockServer.Close()
}

func (s *SchemaValidatorTestSuite) createRequest(verb string, body string) *api.HTTPValidationRequest {
	return &api.HTTPValidationRequest{
		GVK:  &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		Body: []byte(body),
		Verb: verb,
	}
}

func (s *SchemaValidatorTestSuite) TestName() {
	s.Equal("schema", s.schemaValidator.Name())
}

func (s *SchemaValidatorTestSuite) TestValidate() {
	s.Run("nil GVK returns no error", func() {
		err := s.schemaValidator.Validate(context.Background(), &api.HTTPValidationRequest{
			Body: []byte(`{"apiVersion":"v1","kind":"Pod"}`),
			Verb: "create",
		})
		s.NoError(err)
	})
	s.Run("empty body returns no error", func() {
		err := s.schemaValidator.Validate(context.Background(), &api.HTTPValidationRequest{
			GVK:  &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Verb: "create",
		})
		s.NoError(err)
	})
	s.Run("non create or update verb returns no error", func() {
		for _, verb := range []string{"get", "list", "delete", "patch", "watch"} {
			err := s.schemaValidator.Validate(context.Background(), s.createRequest(verb, `{"apiVersion":"v1","kind":"Pod","specTypo":"bad"}`))
			s.NoError(err, "verb %q should not trigger validation", verb)
		}
	})
	s.Run("nil discovery client returns no error", func() {
		sv := NewSchemaValidator(func() discovery.DiscoveryInterface { return nil })
		err := sv.Validate(context.Background(), s.createRequest("create", `{"apiVersion":"v1","kind":"Pod"}`))
		s.NoError(err)
	})
	s.Run("valid manifest returns no error", func() {
		err := s.schemaValidator.Validate(context.Background(), s.createRequest("create",
			`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"test"},"spec":{"containers":[]}}`))
		s.NoError(err)
	})
	s.Run("unknown field returns validation error with field name", func() {
		err := s.schemaValidator.Validate(context.Background(), s.createRequest("create",
			`{"apiVersion":"v1","kind":"Pod","specTypo":"bad"}`))
		s.Require().Error(err)
		var ve *api.ValidationError
		s.Require().ErrorAs(err, &ve)
		s.Equal(api.ErrorCodeInvalidField, ve.Code)
		s.Equal("specTypo", ve.Field)
		s.Contains(ve.Message, "specTypo")
	})
	s.Run("unknown nested field returns validation error with field name", func() {
		err := s.schemaValidator.Validate(context.Background(), s.createRequest("create",
			`{"apiVersion":"v1","kind":"Pod","spec":{"badField":"value"}}`))
		s.Require().Error(err)
		var ve *api.ValidationError
		s.Require().ErrorAs(err, &ve)
		s.Equal(api.ErrorCodeInvalidField, ve.Code)
		s.Equal("badField", ve.Field)
	})
	s.Run("update verb triggers validation", func() {
		err := s.schemaValidator.Validate(context.Background(), s.createRequest("update",
			`{"apiVersion":"v1","kind":"Pod","unknownField":"bad"}`))
		s.Error(err)
	})
	s.Run("unknown resource type skips validation", func() {
		req := &api.HTTPValidationRequest{
			GVK:  &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "UnknownKind"},
			Body: []byte(`{"apiVersion":"v1","kind":"UnknownKind","whatever":"bad"}`),
			Verb: "create",
		}
		err := s.schemaValidator.Validate(context.Background(), req)
		s.NoError(err)
	})
}

func TestSchemaValidator(t *testing.T) {
	suite.Run(t, new(SchemaValidatorTestSuite))
}

// newOpenAPISchemaHandler creates an HTTP handler that serves a minimal OpenAPI v2
// document at /openapi/v2 as protobuf. It defines a Pod resource (v1/Pod) with
// apiVersion, kind, metadata, and spec properties so that schema validation can
// detect unknown fields.
func newOpenAPISchemaHandler() http.Handler {
	doc := &openapi_v2.Document{
		Swagger: "2.0",
		Info:    &openapi_v2.Info{Title: "Test", Version: "v1"},
		Definitions: &openapi_v2.Definitions{
			AdditionalProperties: []*openapi_v2.NamedSchema{
				{
					Name: "io.k8s.api.core.v1.Pod",
					Value: &openapi_v2.Schema{
						Type: &openapi_v2.TypeItem{Value: []string{"object"}},
						Properties: &openapi_v2.Properties{
							AdditionalProperties: []*openapi_v2.NamedSchema{
								{Name: "apiVersion", Value: &openapi_v2.Schema{Type: &openapi_v2.TypeItem{Value: []string{"string"}}}},
								{Name: "kind", Value: &openapi_v2.Schema{Type: &openapi_v2.TypeItem{Value: []string{"string"}}}},
								{Name: "metadata", Value: &openapi_v2.Schema{Type: &openapi_v2.TypeItem{Value: []string{"object"}}}},
								{Name: "spec", Value: &openapi_v2.Schema{
									Type: &openapi_v2.TypeItem{Value: []string{"object"}},
									Properties: &openapi_v2.Properties{
										AdditionalProperties: []*openapi_v2.NamedSchema{
											{Name: "containers", Value: &openapi_v2.Schema{
												Type: &openapi_v2.TypeItem{Value: []string{"array"}},
												Items: &openapi_v2.ItemsItem{
													Schema: []*openapi_v2.Schema{
														{Type: &openapi_v2.TypeItem{Value: []string{"object"}}},
													},
												},
											}},
										},
									},
								}},
							},
						},
						VendorExtension: []*openapi_v2.NamedAny{
							{
								Name: "x-kubernetes-group-version-kind",
								Value: &openapi_v2.Any{
									Yaml: "- group: \"\"\n  version: v1\n  kind: Pod\n",
								},
							},
						},
					},
				},
			},
		},
	}

	data, err := proto.Marshal(doc)
	if err != nil {
		panic("failed to marshal OpenAPI v2 document: " + err.Error())
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/openapi/v2" {
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
}
