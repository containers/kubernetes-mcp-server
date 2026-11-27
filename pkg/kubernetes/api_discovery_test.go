package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/openapi"
	"k8s.io/client-go/rest"
)

type APIDiscoverySuite struct {
	suite.Suite
}

func TestAPIDiscovery(t *testing.T) {
	suite.Run(t, new(APIDiscoverySuite))
}

func (s *APIDiscoverySuite) TestAPIResourceInventoryFilters() {
	client := &apiDiscoveryTestClient{
		resources: []*metav1.APIResourceList{
			{GroupVersion: "example.com/v1", APIResources: []metav1.APIResource{
				{Name: "widgets-b", Kind: "WidgetB", Namespaced: true},
				{Name: "secrets", Kind: "Secret"},
				{Name: "widgets-a", Kind: "WidgetA", Namespaced: true},
			}},
			{GroupVersion: "other.example/v1", APIResources: []metav1.APIResource{
				{Name: "widgetcs", Kind: "WidgetC"},
			}},
		},
	}
	core := NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), client})

	group := "example.com"
	first, err := core.APIResourceInventory(s.T().Context(), APIResourceListOptions{
		Group:  &group,
		Query:  "widget",
		Denied: []config.GroupVersionKind{{Version: "v1", Kind: "Secret"}},
	})
	s.Require().NoError(err)
	s.Equal("widgets-a", first.Resources[0].Name)
	s.Equal("widgets-b", first.Resources[1].Name)
	s.Empty(first.Continue)
	s.Len(first.Resources, 2)
}

func (s *APIDiscoverySuite) TestAPIResourceInventoryReportsPartialDiscovery() {
	client := &apiDiscoveryTestClient{
		resources: []*metav1.APIResourceList{{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{{Name: "pods", Kind: "Pod", Namespaced: true}},
		}},
		err: errors.New("one API group unavailable"),
	}
	page, err := NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), client}).APIResourceInventory(
		s.T().Context(),
		APIResourceListOptions{},
	)
	s.Require().NoError(err)
	s.Len(page.Resources, 1)
	s.False(page.Complete)
	s.Contains(page.Warning, "one API group unavailable")
}

func (s *APIDiscoverySuite) TestAPIResourceExplainRequiresCompleteDiscoveryForResolution() {
	client := &apiDiscoveryTestClient{
		openAPIV3: newTestOpenAPIV3Client(map[string]any{
			"components": map[string]any{"schemas": map[string]any{
				"Widget": map[string]any{
					"type": "object",
					"x-kubernetes-group-version-kind": map[string]any{
						"group": "example.com", "version": "v1", "kind": "Widget",
					},
				},
			}},
		}),
		err: errors.New("one API group unavailable"),
	}
	core := NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), client})
	for _, group := range []string{"", "example.com/v1", "other.example/v1"} {
		if group != "" {
			client.resources = append(client.resources, &metav1.APIResourceList{
				GroupVersion: group,
				APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget"}},
			})
		}
		s.Run(fmt.Sprintf("%d discovered candidates", len(client.resources)), func() {
			result, err := core.APIResourceExplain(s.T().Context(), "", "Widget", "", nil)
			s.Nil(result)
			var detail *APIDiscoveryError
			s.Require().ErrorAs(err, &detail)
			s.Equal("discovery_incomplete", detail.Code)
			s.Contains(detail.Message, "explicit apiVersion")
			s.Contains(detail.Message, "one API group unavailable")
			s.Len(detail.Candidates, len(client.resources))
			if len(client.resources) == 0 {
				s.Contains(detail.Message, "No permitted exact matches")
			} else {
				s.Equal("example.com/v1", detail.Candidates[0].APIVersion)
				s.Contains(detail.Message, "list may be incomplete")
			}
		})
	}
	s.Run("partial candidates exclude denied types and unrelated kinds", func() {
		client.resources = append(client.resources, &metav1.APIResourceList{
			GroupVersion: "example.com/v1",
			APIResources: []metav1.APIResource{{Name: "gadgets", Kind: "Gadget"}, {Name: "widgets", Kind: "Widget"}},
		})
		_, err := core.APIResourceExplain(s.T().Context(), "", "widget", "", []config.GroupVersionKind{{Group: "other.example", Version: "v1"}})
		var detail *APIDiscoveryError
		s.Require().ErrorAs(err, &detail)
		s.Equal("discovery_incomplete", detail.Code)
		s.Equal([]ExplainArguments{{Kind: "Widget", APIVersion: "example.com/v1"}}, detail.Candidates)
	})
	s.Run("no permitted candidates does not expose denied matches", func() {
		_, err := core.APIResourceExplain(s.T().Context(), "", "Widget", "", []config.GroupVersionKind{
			{Group: "example.com", Version: "v1"},
			{Group: "other.example", Version: "v1"},
		})
		var detail *APIDiscoveryError
		s.Require().ErrorAs(err, &detail)
		s.Equal("discovery_incomplete", detail.Code)
		s.Empty(detail.Candidates)
		s.Contains(detail.Message, "No permitted exact matches")
	})
	s.Run("explicit version bypasses incomplete resource discovery", func() {
		result, err := core.APIResourceExplain(s.T().Context(), "example.com/v1", "widget", "", nil)
		s.Require().NoError(err)
		s.Equal("example.com/v1", result.APIVersion)
		s.Equal("Widget", result.Kind)
	})
	s.Run("complete discovery resolves a unique kind", func() {
		client.err = nil
		client.resources = client.resources[:1]
		result, err := core.APIResourceExplain(s.T().Context(), "", "widget", "", nil)
		s.Require().NoError(err)
		s.Equal("example.com/v1", result.APIVersion)
		s.Equal("Widget", result.Kind)
	})
}

func (s *APIDiscoverySuite) TestAPIResourceInventoryRejectsInvalidContinuation() {
	core := NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), &apiDiscoveryTestClient{}})
	for _, options := range []APIResourceListOptions{
		{Continue: "-1"},
		{Continue: "not-a-number"},
		{Continue: "1"},
	} {
		_, err := core.APIResourceInventory(s.T().Context(), options)
		s.Error(err)
	}
}

func (s *APIDiscoverySuite) TestAPIResourceExplainResolvesFieldReferencesAndParentConstraints() {
	document := map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{
				"example.v1.Widget": map[string]any{
					"type": "object",
					"x-kubernetes-group-version-kind": map[string]any{
						"group": "example.com", "version": "v1", "kind": "Widget",
					},
					"required": []any{"spec"},
					"properties": map[string]any{
						"spec": map[string]any{"$ref": "#/components/schemas/WidgetSpec"},
					},
				},
				"WidgetSpec": map[string]any{
					"type":                     "object",
					"required":                 []any{"fallbackPolicy"},
					"x-kubernetes-validations": []any{map[string]any{"rule": "self.fallbackPolicy != ''"}},
					"properties": map[string]any{
						"fallbackPolicy": map[string]any{
							"type":        "string",
							"description": "Behavior when preferred placement is unavailable.",
							"enum":        []any{"NextEligible", "FailIfPreferredUnavailable"},
						},
						"self": map[string]any{"$ref": "#/components/schemas/WidgetSpec"},
					},
				},
			},
		},
	}
	client := newTestOpenAPIV3Client(document)
	result, err := NewCore(apiDiscoveryTestKubernetesClient{
		api.KubernetesClient(nil),
		&apiDiscoveryTestClient{openAPIV3: client},
	}).APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", "spec.fallbackPolicy", nil)

	s.Require().NoError(err)
	s.Equal("string", result.Schema["type"])
	s.Equal([]any{"NextEligible", "FailIfPreferredUnavailable"}, result.Schema["enum"])
	s.Equal("Behavior when preferred placement is unavailable.", result.Schema["description"])
	s.Equal([]any{"spec"}, result.Ancestors[0].Schema["required"])
	s.Equal([]any{"fallbackPolicy"}, result.Ancestors[1].Schema["required"])
	s.NotEmpty(result.Ancestors[1].Schema["x-kubernetes-validations"])
	s.Equal("fixture-hash", result.SchemaHash)
}

func (s *APIDiscoverySuite) TestAPIResourceExplainValidatesPathsAndReferences() {
	document := map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{
				"example.v1.Widget": map[string]any{
					"type": "object",
					"x-kubernetes-group-version-kind": map[string]any{
						"group": "example.com", "version": "v1", "kind": "Widget",
					},
					"properties": map[string]any{
						"spec": map[string]any{"$ref": "#/components/schemas/WidgetSpec"},
					},
				},
				"WidgetSpec": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"self": map[string]any{"$ref": "#/components/schemas/WidgetSpec"},
					},
				},
			},
		},
	}
	core := NewCore(apiDiscoveryTestKubernetesClient{
		api.KubernetesClient(nil),
		&apiDiscoveryTestClient{openAPIV3: newTestOpenAPIV3Client(document)},
	})
	for _, fieldPath := range []string{"spec.missing", "spec." + strings.Repeat("self.", 32) + "self", "spec.[]"} {
		_, err := core.APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", fieldPath, nil)
		s.Error(err)
	}
	result, err := core.APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", "spec.self.self", nil)
	s.Require().NoError(err)
	s.True(result.Complete)
	s.NotEmpty(result.Components)
}

func (s *APIDiscoverySuite) TestAPIResourceExplainBoundsCompositionWork() {
	definitions := map[string]any{
		"Layer0": map[string]any{"type": "object", "properties": map[string]any{
			"name": map[string]any{"type": "string"},
		}},
	}
	for i := 1; i <= 30; i++ {
		ref := map[string]any{"$ref": fmt.Sprintf("#/components/schemas/Layer%d", i-1)}
		definitions[fmt.Sprintf("Layer%d", i)] = map[string]any{"allOf": []any{ref, ref}}
	}
	definitions["Layer0"].(map[string]any)["properties"].(map[string]any)["child"] = map[string]any{"$ref": "#/components/schemas/Layer14"}
	for _, tc := range []struct {
		name  string
		layer int
		path  string
	}{
		{name: "one composition exceeds budget", layer: 30, path: "name"},
		{name: "budget shared across property steps", layer: 14, path: "child.name"},
	} {
		s.Run(tc.name, func() {
			core := s.schemaCore(map[string]any{"allOf": []any{
				map[string]any{"$ref": fmt.Sprintf("#/components/schemas/Layer%d", tc.layer)},
			}}, definitions)
			result, err := core.APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", tc.path, nil)
			s.Nil(result)
			var detail *APIDiscoveryError
			s.Require().ErrorAs(err, &detail)
			s.Equal("schema_work_limit", detail.Code)
		})
	}
}

func (s *APIDiscoverySuite) TestAPIResourceExplainFieldPathLimits() {
	for _, length := range []int{512, 513} {
		name := strings.Repeat("x", length)
		core := s.schemaCore(map[string]any{"type": "object", "properties": map[string]any{
			name: map[string]any{"type": "string"},
		}}, nil)
		result, err := core.APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", name, nil)
		if length == 512 {
			s.Require().NoError(err)
			s.Equal("string", result.Schema["type"])
		} else {
			var detail *APIDiscoveryError
			s.Require().ErrorAs(err, &detail)
			s.Equal("invalid_field_path", detail.Code)
		}
	}
	root := map[string]any{"type": "string"}
	for range 33 {
		root = map[string]any{"type": "object", "properties": map[string]any{"x": root}}
	}
	core := s.schemaCore(root, nil)
	for _, segments := range []int{32, 33, 100000} {
		path := strings.TrimSuffix(strings.Repeat("x.", segments), ".")
		_, err := core.APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", path, nil)
		if segments == 32 {
			s.Require().NoError(err)
		} else {
			var detail *APIDiscoveryError
			s.Require().ErrorAs(err, &detail)
			s.Equal("invalid_field_path", detail.Code)
		}
	}
}

func (s *APIDiscoverySuite) TestAPIResourceExplainTraversesArrayItems() {
	document := map[string]any{
		"components": map[string]any{"schemas": map[string]any{
			"Widget": map[string]any{
				"type": "object",
				"x-kubernetes-group-version-kind": map[string]any{
					"group": "example.com", "version": "v1", "kind": "Widget",
				},
				"properties": map[string]any{
					"spec": map[string]any{"type": "object", "properties": map[string]any{
						"items": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{
							"type": "object", "properties": map[string]any{
								"name": map[string]any{"type": "string"},
							},
						}},
					}},
				},
			},
		}},
	}
	result, err := NewCore(apiDiscoveryTestKubernetesClient{
		api.KubernetesClient(nil),
		&apiDiscoveryTestClient{openAPIV3: newTestOpenAPIV3Client(document)},
	}).APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", "spec.items.name", nil)
	s.Require().NoError(err)
	s.Equal("string", result.Schema["type"])
	s.Equal(float64(1), result.Ancestors[2].Schema["minItems"])
}

func (s *APIDiscoverySuite) TestAPIResourceExplainReportsAmbiguousAndUnavailableSchemas() {
	base := map[string]any{
		"type": "object",
		"x-kubernetes-group-version-kind": map[string]any{
			"group": "example.com", "version": "v1", "kind": "Widget",
		},
	}
	document := map[string]any{
		"components": map[string]any{"schemas": map[string]any{
			"one": base,
			"two": base,
		}},
	}
	client := &apiDiscoveryTestClient{openAPIV3: newTestOpenAPIV3Client(document)}
	core := NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), client})
	_, err := core.APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", "", nil)
	s.ErrorContains(err, "multiple definitions")
	_, err = core.APIResourceExplain(s.T().Context(), "missing.example/v1", "Widget", "", nil)
	s.ErrorContains(err, "unavailable")

	client.openAPIV3 = newTestOpenAPIV3Client(map[string]any{"components": map[string]any{"schemas": map[string]any{}}})
	_, err = core.APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", "", nil)
	s.ErrorContains(err, "does not define")
}

func (s *APIDiscoverySuite) TestAPIResourceExplainPreservesDeepSmallSchema() {
	properties := map[string]any{
		"a": map[string]any{"type": "object", "properties": map[string]any{
			"b": map[string]any{"type": "object", "properties": map[string]any{"c": map[string]any{"type": "string"}}},
		}},
	}
	document := map[string]any{
		"components": map[string]any{"schemas": map[string]any{
			"Widget": map[string]any{
				"type": "object",
				"x-kubernetes-group-version-kind": map[string]any{
					"group": "example.com", "version": "v1", "kind": "Widget",
				},
				"properties": properties,
			},
		}},
	}
	result, err := NewCore(apiDiscoveryTestKubernetesClient{
		api.KubernetesClient(nil),
		&apiDiscoveryTestClient{openAPIV3: newTestOpenAPIV3Client(document)},
	}).APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", "", nil)
	s.Require().NoError(err)
	s.True(result.Complete)
	s.Empty(result.Omissions)
	s.Equal(properties["a"], result.Schema["properties"].(map[string]any)["a"])
	encoded, err := json.Marshal(result)
	s.Require().NoError(err)
	s.LessOrEqual(len(encoded), MaxSchemaResultBytes)
}

func (s *APIDiscoverySuite) TestAPIResourceExplainPreservesManySmallFields() {
	properties := map[string]any{}
	for i := range 80 {
		properties[fmt.Sprintf("field%02d", i)] = map[string]any{"type": "string", "description": "Small field."}
	}
	core := s.schemaCore(map[string]any{"type": "object", "properties": properties}, nil)
	result, err := core.APIResourceExplain(s.T().Context(), "", "Widget", "", nil)
	s.Require().NoError(err)
	s.True(result.Complete)
	s.Empty(result.Omissions)
	s.Len(result.Schema["properties"], 80, "there is no arbitrary 64-field rendering cap")
}

func (s *APIDiscoverySuite) TestAPIResourceExplainSelectsExplicitNonPreferredVersion() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi/v3":
			_, _ = w.Write([]byte(`{"paths":{"apis/example.com/v2":{"serverRelativeURL":"/openapi/v3/apis/example.com/v2"}}}`))
		case "/openapi/v3/apis/example.com/v2":
			_, _ = w.Write([]byte(`{"components":{"schemas":{"Widget":{"type":"object","description":"Explicit non-preferred version.","x-kubernetes-group-version-kind":[{"group":"example.com","version":"v2","kind":"Widget"}]}}}}`))
		default:
			http.Error(w, "unexpected preferred discovery or instance read", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: server.URL})
	s.Require().NoError(err)
	result, err := NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), client}).
		APIResourceExplain(s.T().Context(), "example.com/v2", "widget", "", nil)
	s.Require().NoError(err)
	s.Equal("Widget", result.Kind)
	s.Equal("example.com/v2", result.APIVersion)
}

func (s *APIDiscoverySuite) TestAPIResourceInventorySharesDocumentsOnlyWithinOneCall() {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi/v3":
			_, _ = w.Write([]byte(`{"paths":{"apis/example.com/v1":{"serverRelativeURL":"/openapi/v3/apis/example.com/v1"}}}`))
		case "/openapi/v3/apis/example.com/v1":
			requests.Add(1)
			_, _ = w.Write([]byte(`{"components":{"schemas":{"One":{"description":"First resource.","x-kubernetes-group-version-kind":[{"group":"example.com","version":"v1","kind":"One"}]},"Two":{"description":"Second resource.","x-kubernetes-group-version-kind":[{"group":"example.com","version":"v1","kind":"Two"}]}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: server.URL})
	s.Require().NoError(err)
	core := NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), &apiDiscoveryTestClient{
		resources: []*metav1.APIResourceList{{GroupVersion: "example.com/v1", APIResources: []metav1.APIResource{
			{Name: "ones", Kind: "One"}, {Name: "twos", Kind: "Two"},
		}}},
		openAPIV3: client.OpenAPIV3WithContext(s.T().Context()),
	}})
	for call := range 2 {
		page, err := core.APIResourceInventory(s.T().Context(), APIResourceListOptions{})
		s.Require().NoError(err)
		s.Require().Len(page.Resources, 2)
		s.Empty(page.Warning)
		s.Equal(int32(call+1), requests.Load())
	}
}

func (s *APIDiscoverySuite) TestAPIResourceInventoryMissingDescriptionsDoNotAffectNameSearchCoverage() {
	client := &apiDiscoveryTestClient{resources: []*metav1.APIResourceList{{
		GroupVersion: "example.com/v1", APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget"}},
	}}}
	core := NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), client})
	page, err := core.APIResourceInventory(s.T().Context(), APIResourceListOptions{Query: "tenant enrollment"})
	s.Require().NoError(err)
	s.Empty(page.Resources)
	s.True(page.Complete)
	s.NotContains(page.Warning, "Search coverage")
	page, err = core.APIResourceInventory(s.T().Context(), APIResourceListOptions{Query: "widget"})
	s.Require().NoError(err)
	s.Require().Len(page.Resources, 1)
	s.True(page.Resources[0].DescriptionUnavailable)
	s.True(page.Complete, "inventory coverage is independent of description availability")
}

func (s *APIDiscoverySuite) TestAPIResourceExplainTruncatesOversizedSchemasExplicitly() {
	properties := make(map[string]any, 40)
	for i := range 40 {
		properties[fmt.Sprintf("field-%02d", i)] = map[string]any{
			"type":        "string",
			"description": strings.Repeat("description ", 60),
		}
	}
	document := map[string]any{
		"components": map[string]any{"schemas": map[string]any{
			"Widget": map[string]any{
				"type": "object",
				"x-kubernetes-group-version-kind": map[string]any{
					"group": "example.com", "version": "v1", "kind": "Widget",
				},
				"properties": properties,
			},
		}},
	}
	result, err := NewCore(apiDiscoveryTestKubernetesClient{
		api.KubernetesClient(nil),
		&apiDiscoveryTestClient{openAPIV3: newTestOpenAPIV3Client(document)},
	}).APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", "", nil)
	s.Require().NoError(err)
	s.False(result.Complete)
	s.NotEmpty(result.Omissions)
	encoded, err := json.Marshal(result)
	s.Require().NoError(err)
	s.LessOrEqual(len(encoded), MaxSchemaResultBytes)
}

func (s *APIDiscoverySuite) TestAPIResourceExplainUsesWorkspacePrefixAndSchemaHash() {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		switch r.URL.Path {
		case "/workspaces/demo/openapi/v3":
			_, _ = w.Write([]byte(`{"paths":{"apis/example.com/v1":{"serverRelativeURL":"/openapi/v3/apis/example.com/v1?hash=project-hash"}}}`))
		case "/workspaces/demo/openapi/v3/apis/example.com/v1":
			if r.URL.Query().Get("hash") != "project-hash" {
				http.Error(w, "missing project schema hash", http.StatusBadRequest)
				return
			}
			_, _ = w.Write(schemaDocumentBytes(map[string]any{
				"components": map[string]any{"schemas": map[string]any{
					"Widget": map[string]any{
						"type": "object",
						"x-kubernetes-group-version-kind": map[string]any{
							"group": "example.com", "version": "v1", "kind": "Widget",
						},
					},
				}},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: server.URL + "/workspaces/demo"})
	s.Require().NoError(err)
	result, err := NewCore(apiDiscoveryTestKubernetesClient{
		api.KubernetesClient(nil),
		discoveryClient,
	}).APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", "", nil)

	s.Require().NoError(err)
	s.Equal("project-hash", result.SchemaHash)
	s.Len(requests, 2)
	indexRequest, err := url.ParseRequestURI(requests[0])
	s.Require().NoError(err)
	s.Equal("/workspaces/demo/openapi/v3", indexRequest.Path)
	schemaRequest, err := url.ParseRequestURI(requests[1])
	s.Require().NoError(err)
	s.Equal("/workspaces/demo/openapi/v3/apis/example.com/v1", schemaRequest.Path)
	s.Equal("project-hash", schemaRequest.Query().Get("hash"))
	s.NotContains(strings.Join(requests, "\n"), "/workspaces/tenant/")
}

func (s *APIDiscoverySuite) TestAPIResourceExplainUsesServedSchemaWhenCRDReadsAreDenied() {
	const (
		group = "example.com"
		kind  = "Widget"
	)
	document := map[string]any{
		"components": map[string]any{"schemas": map[string]any{
			"example.v1.Widget": map[string]any{
				"type": "object",
				"x-kubernetes-group-version-kind": []any{map[string]any{
					"group": group, "version": "v1", "kind": kind,
				}},
				"properties": map[string]any{
					"spec": map[string]any{
						"type":     "object",
						"required": []any{"mode"},
						"properties": map[string]any{
							"mode": map[string]any{
								"type": "string", "description": "The selected widget mode.",
								"enum": []any{"safe", "fast"},
							},
						},
					},
				},
			},
		}},
	}
	var crdReadAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi/v3":
			_, _ = w.Write([]byte(`{"paths":{"apis/example.com/v1":{"serverRelativeURL":"/openapi/v3/apis/example.com/v1?hash=fixture"}}}`))
		case "/openapi/v3/apis/example.com/v1":
			if r.URL.Query().Get("hash") != "fixture" {
				http.Error(w, "missing schema hash", http.StatusBadRequest)
				return
			}
			_, _ = w.Write(schemaDocumentBytes(document))
		default:
			if strings.HasPrefix(r.URL.Path, "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/") {
				crdReadAttempts.Add(1)
				http.Error(w, "CRD object reads are denied", http.StatusForbidden)
				return
			}
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	crdResponse, err := http.Get(server.URL + "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/widgets.example.com")
	s.Require().NoError(err)
	s.Equal(http.StatusForbidden, crdResponse.StatusCode)
	s.Require().NoError(crdResponse.Body.Close())
	s.Equal(int32(1), crdReadAttempts.Load())

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: server.URL})
	s.Require().NoError(err)
	result, err := NewCore(apiDiscoveryTestKubernetesClient{
		api.KubernetesClient(nil),
		discoveryClient,
	}).APIResourceExplain(s.T().Context(), group+"/v1", kind, "spec.mode", nil)
	s.Require().NoError(err)

	sourceResponse, err := http.Get(server.URL + "/openapi/v3/apis/example.com/v1?hash=fixture")
	s.Require().NoError(err)
	defer func() { _ = sourceResponse.Body.Close() }()
	var sourceDocument map[string]any
	s.Require().NoError(json.NewDecoder(sourceResponse.Body).Decode(&sourceDocument))
	sourceSchemas := sourceDocument["components"].(map[string]any)["schemas"].(map[string]any)
	sourceSchema, err := findResourceSchema(sourceSchemas, group, "v1", kind)
	s.Require().NoError(err)
	sourceSpec := sourceSchema["properties"].(map[string]any)["spec"].(map[string]any)
	sourceField := sourceSpec["properties"].(map[string]any)["mode"].(map[string]any)

	s.Equal(sourceField["type"], result.Schema["type"])
	s.Equal(sourceField["description"], result.Schema["description"])
	s.Equal(sourceField["enum"], result.Schema["enum"])
	s.Equal(sourceSpec["required"], result.Ancestors[1].Schema["required"])
	s.Equal("fixture", result.SchemaHash)
	s.Equal(int32(1), crdReadAttempts.Load(), "schema inspection must not retry the denied CRD read")
}

func (s *APIDiscoverySuite) TestAPIResourceExplainReturnsAuthorizationErrors() {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, http.StatusText(status), status)
		}))
		discoveryClient, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: server.URL})
		s.Require().NoError(err)
		_, err = NewCore(apiDiscoveryTestKubernetesClient{
			api.KubernetesClient(nil),
			discoveryClient,
		}).APIResourceExplain(s.T().Context(), "example.com/v1", "Widget", "", nil)
		s.Error(err)
		s.T().Cleanup(server.Close)
	}
}

func (s *APIDiscoverySuite) schemaCore(root map[string]any, definitions map[string]any) *Core {
	root["x-kubernetes-group-version-kind"] = []any{map[string]any{
		"group": "example.com", "version": "v1", "kind": "Widget",
	}}
	if definitions == nil {
		definitions = map[string]any{}
	}
	definitions["Widget"] = root
	return NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), &apiDiscoveryTestClient{
		resources: []*metav1.APIResourceList{{GroupVersion: "example.com/v1", APIResources: []metav1.APIResource{
			{Name: "widgets", Kind: "Widget", ShortNames: []string{"wdg"}},
		}}},
		openAPIV3: newTestOpenAPIV3Client(map[string]any{"components": map[string]any{"schemas": definitions}}),
	}})
}

func (s *APIDiscoverySuite) TestAPIResourceExplainCompactionAndExpansionProgress() {
	children := map[string]any{}
	for i := range 80 {
		children[fmt.Sprintf("field%02d", i)] = map[string]any{
			"type": "string", "description": strings.Repeat("Detailed documentation. ", 30),
		}
	}
	core := s.schemaCore(map[string]any{"type": "object", "description": "Widget root documentation.",
		"properties": map[string]any{
			"mode":     map[string]any{"type": "string", "description": "Keep this full description.", "enum": []any{"safe", "fast"}},
			"template": map[string]any{"$ref": "#/components/schemas/Template"},
		},
	}, map[string]any{"Template": map[string]any{
		"type": "object", "description": "Large template documentation.", "properties": children,
	}})
	whole, err := core.APIResourceExplain(s.T().Context(), "", "widget", "", nil)
	s.Require().NoError(err)
	s.Equal("Widget", whole.Kind)
	s.False(whole.Complete)
	s.Require().Len(whole.Omissions, 1)
	s.Equal("template", whole.Omissions[0].Path)
	props := whole.Schema["properties"].(map[string]any)
	s.Equal(true, props["template"].(map[string]any)["x-mcp-omitted"])
	s.Equal("Keep this full description.", props["mode"].(map[string]any)["description"])
	s.Empty(whole.Components, "unreachable definitions must not leak into a compacted response")
	expand := whole.Omissions[0].Expand
	expanded, err := core.APIResourceExplain(s.T().Context(), expand.APIVersion, expand.Kind, expand.FieldPath, nil)
	s.Require().NoError(err)
	s.NotEmpty(expanded.Schema["properties"], "expansion must reveal fields hidden in the first response")
	s.LessOrEqual(len(expanded.Omissions), 3)
	for _, omission := range expanded.Omissions {
		s.NotEqual(expanded.FieldPath, omission.Expand.FieldPath, "must not suggest an identical retry")
	}
	for _, result := range []*APIResourceExplainResult{whole, expanded} {
		data, err := json.Marshal(result)
		s.Require().NoError(err)
		s.LessOrEqual(len(data), MaxSchemaResultBytes)
	}
}

func (s *APIDiscoverySuite) TestAPIResourceExplainSharedReferencesDoNotMutateOtherPaths() {
	core := s.schemaCore(map[string]any{"type": "object", "properties": map[string]any{
		"left":    map[string]any{"$ref": "#/components/schemas/Shared"},
		"right":   map[string]any{"$ref": "#/components/schemas/Shared"},
		"example": map[string]any{"default": map[string]any{"$ref": "arbitrary user data"}},
	}}, map[string]any{"Shared": map[string]any{"type": "object", "properties": map[string]any{
		"next": map[string]any{"$ref": "#/components/schemas/Shared"},
		"name": map[string]any{"type": "string"},
	}}})
	for _, path := range []string{"", "left", "right.next.name"} {
		result, err := core.APIResourceExplain(s.T().Context(), "example.com/v1", "widget", path, nil)
		s.Require().NoError(err)
		s.True(result.Complete)
		if path != "right.next.name" {
			s.NotEmpty(result.Components["schemas"].(map[string]any)["Shared"])
		}
	}
}

func (s *APIDiscoverySuite) TestAPIResourceExplainRetainsAllOfAndMapValidationContext() {
	core := s.schemaCore(map[string]any{"type": "object", "properties": map[string]any{
		"spec": map[string]any{"type": "object", "additionalProperties": map[string]any{
			"allOf": []any{
				map[string]any{"type": "object", "required": []any{"mode"}, "properties": map[string]any{
					"mode": map[string]any{"type": "string", "minLength": 2},
				}},
				map[string]any{"x-kubernetes-validations": []any{map[string]any{"rule": "self.mode != self.other"}},
					"properties": map[string]any{"mode": map[string]any{"enum": []any{"safe", "fast"}}}},
			},
		}},
	}}, nil)
	result, err := core.APIResourceExplain(s.T().Context(), "", "Widget", "spec.mode", nil)
	s.Require().NoError(err)
	s.Require().Len(result.Schema["allOf"], 2)
	encoded, err := json.Marshal(result.Ancestors)
	s.Require().NoError(err)
	s.Contains(string(encoded), "self.mode != self.other")
	s.Contains(string(encoded), `"required":["mode"]`)
	s.True(result.Complete)
}

func (s *APIDiscoverySuite) TestAPIResourceExplainRejectsUnfitLeafConstraintsAndDescriptions() {
	for _, field := range []string{"enum", "description", "x-kubernetes-validations"} {
		s.Run(field, func() {
			text := strings.Repeat("不可省略", 6000)
			leaf := map[string]any{"type": "string", field: text}
			switch field {
			case "enum":
				leaf[field] = []any{text}
			case "x-kubernetes-validations":
				leaf[field] = []any{map[string]any{"rule": text}}
			}
			core := s.schemaCore(map[string]any{"type": "object", "properties": map[string]any{"huge": leaf}}, nil)
			_, err := core.APIResourceExplain(s.T().Context(), "", "Widget", "huge", nil)
			var detail *APIDiscoveryError
			s.Require().ErrorAs(err, &detail)
			s.Equal("schema_too_large", detail.Code)
		})
	}
}

func (s *APIDiscoverySuite) TestAPIResourceInventoryPagesByBytesAndRejectsChangedSnapshots() {
	resources := []metav1.APIResource{}
	schemas := map[string]any{}
	for i := range 180 {
		kind := fmt.Sprintf("Widget%03d", i)
		resources = append(resources, metav1.APIResource{Name: strings.ToLower(kind) + "s", Kind: kind})
		schemas[kind] = map[string]any{
			"description":                     strings.Repeat("文", 120),
			"x-kubernetes-group-version-kind": map[string]any{"group": "example.com", "version": "v1", "kind": kind},
		}
	}
	client := &apiDiscoveryTestClient{
		resources: []*metav1.APIResourceList{{GroupVersion: "example.com/v1", APIResources: resources}},
		openAPIV3: newTestOpenAPIV3Client(map[string]any{"components": map[string]any{"schemas": schemas}}),
	}
	core := NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), client})
	options := APIResourceListOptions{}
	seen := map[string]bool{}
	firstToken := ""
	for {
		page, err := core.APIResourceInventory(s.T().Context(), options)
		s.Require().NoError(err)
		data, err := json.Marshal(page)
		s.Require().NoError(err)
		s.LessOrEqual(len(data), MaxAPIResourceResultBytes)
		s.Equal(180, page.Total)
		s.Equal(page.Total-page.Returned-len(seen), page.Remaining)
		for _, resource := range page.Resources {
			s.False(seen[resource.Kind])
			seen[resource.Kind] = true
			s.Len([]rune(resource.Description), 100)
			s.True(strings.HasSuffix(resource.Description, "..."))
			s.NotContains(string(data), "descriptionShortened")
			s.NotContains(string(data), "shortNames")
			s.NotContains(string(data), "singularName")
		}
		if firstToken == "" {
			firstToken = page.Continue
		}
		if page.Continue == "" {
			s.True(page.Complete)
			break
		}
		s.False(page.Complete)
		s.NotEmpty(page.Message)
		options.Continue = page.Continue
	}
	s.Len(seen, 180)
	s.Require().NotEmpty(firstToken)
	_, err := core.APIResourceInventory(s.T().Context(), APIResourceListOptions{Continue: firstToken, Query: "Widget"})
	s.ErrorContains(err, "changed")
	schemas["Widget000"].(map[string]any)["description"] = "Changed source documentation."
	_, err = core.APIResourceInventory(s.T().Context(), APIResourceListOptions{Continue: firstToken})
	s.ErrorContains(err, "changed")
}

func (s *APIDiscoverySuite) TestAPIResourceInventorySearchesNamesOnly() {
	core := s.schemaCore(map[string]any{"type": "object",
		"description": "A widget. A domain-specific tenant enrollment request.",
	}, nil)
	for _, query := range []string{"WIDGET", "widgets", "example.com"} {
		page, err := core.APIResourceInventory(s.T().Context(), APIResourceListOptions{Query: query})
		s.Require().NoError(err)
		s.Require().Len(page.Resources, 1)
		s.Equal("A widget...", page.Resources[0].Description)
		s.True(page.Complete)
	}
	for _, query := range []string{"tenant enrollment", "WDG"} {
		page, err := core.APIResourceInventory(s.T().Context(), APIResourceListOptions{Query: query})
		s.Require().NoError(err)
		s.Empty(page.Resources, "descriptions and kubectl aliases are not searched")
		s.True(page.Complete)
	}
	page, err := core.APIResourceInventory(s.T().Context(), APIResourceListOptions{Kind: "Widg"})
	s.Require().NoError(err)
	s.Empty(page.Resources, "kind is exact, not prefix matching")
}

func (s *APIDiscoverySuite) TestAPIResourceExplainAmbiguityAndTypoRecovery() {
	client := &apiDiscoveryTestClient{resources: []*metav1.APIResourceList{
		{GroupVersion: "example.com/v1", APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget"}}},
		{GroupVersion: "other.example/v1", APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget"}}},
	}}
	core := NewCore(apiDiscoveryTestKubernetesClient{api.KubernetesClient(nil), client})
	_, err := core.APIResourceExplain(s.T().Context(), "", "Widget", "", nil)
	var detail *APIDiscoveryError
	s.Require().ErrorAs(err, &detail)
	s.Equal("ambiguous_kind", detail.Code)
	s.Len(detail.Candidates, 2)
	_, err = core.APIResourceExplain(s.T().Context(), "", "Widgte", "", []config.GroupVersionKind{{Group: "other.example", Version: "v1"}})
	s.Require().ErrorAs(err, &detail)
	s.Equal("unknown_kind", detail.Code)
	s.Require().Len(detail.Candidates, 1)
	s.Equal("example.com/v1", detail.Candidates[0].APIVersion)
}

type apiDiscoveryTestClient struct {
	discovery.DiscoveryInterface
	discovery.ServerResourcesInterfaceWithContext
	resources []*metav1.APIResourceList
	err       error
	openAPIV3 openapi.ClientWithContext
}

func (c *apiDiscoveryTestClient) ServerPreferredResourcesWithContext(context.Context) ([]*metav1.APIResourceList, error) {
	return c.resources, c.err
}

func (c *apiDiscoveryTestClient) OpenAPIV3WithContext(context.Context) openapi.ClientWithContext {
	return c.openAPIV3
}

type apiDiscoveryTestKubernetesClient struct {
	api.KubernetesClient
	discovery discovery.DiscoveryInterface
}

func (c apiDiscoveryTestKubernetesClient) RawDiscoveryClient() discovery.DiscoveryInterface {
	return c.discovery
}

type testOpenAPIV3Client struct {
	document map[string]any
	err      error
}

func (c testOpenAPIV3Client) PathsWithContext(context.Context) (map[string]openapi.GroupVersionWithContext, error) {
	if c.err != nil {
		return nil, c.err
	}
	return map[string]openapi.GroupVersionWithContext{
		"apis/example.com/v1": testOpenAPIV3GroupVersion{document: c.document, url: "/openapi/v3/apis/example.com/v1?hash=fixture-hash"},
	}, nil
}

type testOpenAPIV3GroupVersion struct {
	document map[string]any
	url      string
}

func (g testOpenAPIV3GroupVersion) SchemaWithContext(context.Context, string) ([]byte, error) {
	return schemaDocumentBytes(g.document), nil
}

func (g testOpenAPIV3GroupVersion) ServerRelativeURL() string {
	return g.url
}

func newTestOpenAPIV3Client(document map[string]any) openapi.ClientWithContext {
	return testOpenAPIV3Client{document: document}
}

func schemaDocumentBytes(document map[string]any) []byte {
	data, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return data
}
