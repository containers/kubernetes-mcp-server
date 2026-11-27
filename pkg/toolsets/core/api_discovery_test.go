package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/openapi"
)

func TestAPIDiscoveryHandlersClassifyInvalidArguments(t *testing.T) {
	for _, tool := range []string{"list", "explain"} {
		for _, tc := range []struct {
			name string
			args map[string]any
		}{
			{name: "wrong kind type", args: map[string]any{"kind": 123}},
			{name: "empty kind", args: map[string]any{"kind": " "}},
			{name: "wrong apiVersion type", args: map[string]any{"kind": "Widget", "apiVersion": 123}},
			{name: "wrong tool-specific type", args: map[string]any{"kind": "Widget", "continue": 123, "fieldPath": 123}},
			{name: "missing kind", args: map[string]any{}},
		} {
			if tool == "list" && tc.name == "missing kind" {
				continue
			}
			t.Run(tool+"/"+tc.name, func(t *testing.T) {
				params := apiResourcesListParams(nil, nil, tc.args)
				handler := apiResourcesList
				if tool == "explain" {
					handler = apiResourcesExplain
				}
				result, err := handler(params)
				if err != nil {
					t.Fatalf("unexpected Go error: %v", err)
				}
				var text struct {
					Error kubernetes.APIDiscoveryError `json:"error"`
				}
				if err := json.Unmarshal([]byte(result.Content), &text); err != nil {
					t.Fatalf("invalid error JSON: %v", err)
				}
				if result.Error == nil || text.Error.Code != "invalid_argument" {
					t.Fatalf("expected invalid_argument, got %#v", result)
				}
			})
		}
	}
}

func TestAPIResourcesListHandlerFiltersDeniedTypes(t *testing.T) {
	discovery := &apiResourcesTestDiscovery{
		apiResourcesListDiscovery: apiResourcesListDiscovery{
			resources: []*metav1.APIResourceList{{
				GroupVersion: "example.com/v1",
				APIResources: []metav1.APIResource{
					{Name: "widgets", Kind: "Widget", Namespaced: true},
					{Name: "secrets", Kind: "Secret", Namespaced: true},
				},
			}},
		},
	}
	cfg := config.BaseDefault()
	cfg.DeniedResources.SetForTest([]config.GroupVersionKind{{Group: "example.com", Version: "v1", Kind: "Secret"}})
	params := apiResourcesListParams(cfg, apiResourcesListKubernetesClient{discovery: discovery}, map[string]any{
		"group": "example.com",
	})

	result, err := apiResourcesList(params)
	if err != nil {
		t.Fatalf("apiResourcesList returned an unexpected Go error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("apiResourcesList returned an unexpected tool error: %v", result.Error)
	}
	page, ok := result.StructuredContent.(kubernetes.APIResourcePage)
	if !ok {
		t.Fatalf("unexpected structured result type: %T", result.StructuredContent)
	}
	if len(page.Resources) != 1 || page.Resources[0].Kind != "Widget" {
		t.Fatalf("unexpected page: %#v", page)
	}
	if page.Continue != "" {
		t.Fatalf("expected no continuation after filtering denied resources, got %q", page.Continue)
	}
}

func TestAPIResourcesExplainHandlerReadsLiveSchema(t *testing.T) {
	document := map[string]any{
		"components": map[string]any{"schemas": map[string]any{
			"Widget": map[string]any{
				"type": "object",
				"x-kubernetes-group-version-kind": map[string]any{
					"group": "example.com", "version": "v1", "kind": "Widget",
				},
				"properties": map[string]any{
					"spec": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"mode": map[string]any{"type": "string", "enum": []any{"safe", "fast"}},
						},
					},
				},
			},
		}},
	}
	discovery := &apiResourcesTestDiscovery{
		apiResourcesListDiscovery: apiResourcesListDiscovery{},
		openAPIV3:                 apiResourcesTestOpenAPIV3Client{document: document},
	}
	result, err := apiResourcesExplain(apiResourcesListParams(config.BaseDefault(), apiResourcesListKubernetesClient{
		discovery: discovery,
	}, map[string]any{
		"apiVersion": "example.com/v1",
		"kind":       "Widget",
		"fieldPath":  "spec.mode",
	}))
	if err != nil {
		t.Fatalf("apiResourcesExplain returned an unexpected Go error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("apiResourcesExplain returned an unexpected tool error: %v", result.Error)
	}
	explained, ok := result.StructuredContent.(*kubernetes.APIResourceExplainResult)
	if !ok {
		t.Fatalf("unexpected structured result type: %T", result.StructuredContent)
	}
	if explained.Schema["type"] != "string" {
		t.Fatalf("expected the selected field schema, got %#v", explained.Schema)
	}
	if discovery.openAPICalls != 1 {
		t.Fatalf("expected one live OpenAPI request, got %d", discovery.openAPICalls)
	}
}

func TestAPIResourcesExplainHandlerBlocksDeniedSchemasBeforeFetch(t *testing.T) {
	discovery := &apiResourcesTestDiscovery{}
	cfg := config.BaseDefault()
	cfg.DeniedResources.SetForTest([]config.GroupVersionKind{{Group: "example.com", Version: "v1", Kind: "Widget"}})
	result, err := apiResourcesExplain(apiResourcesListParams(cfg, apiResourcesListKubernetesClient{
		discovery: discovery,
	}, map[string]any{
		"apiVersion": "example.com/v1",
		"kind":       "widget",
	}))
	if err != nil {
		t.Fatalf("apiResourcesExplain returned an unexpected Go error: %v", err)
	}
	if result.Error == nil || result.Error.Error() == "" {
		t.Fatalf("expected a schema access error, got %#v", result)
	}
	if discovery.openAPICalls != 0 {
		t.Fatalf("denied schema request reached OpenAPI discovery %d times", discovery.openAPICalls)
	}
}

func TestAPIResourcesExplainHandlerRejectsIncompleteDiscoveryBeforeFetch(t *testing.T) {
	discovery := &apiResourcesTestDiscovery{
		apiResourcesListDiscovery: apiResourcesListDiscovery{
			resources: []*metav1.APIResourceList{{
				GroupVersion: "example.com/v1",
				APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget"}},
			}},
			err: errors.New("one API group unavailable"),
		},
	}
	result, err := apiResourcesExplain(apiResourcesListParams(config.BaseDefault(), apiResourcesListKubernetesClient{
		discovery: discovery,
	}, map[string]any{"kind": "Widget"}))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	var detail *kubernetes.APIDiscoveryError
	if !errors.As(result.Error, &detail) || detail.Code != "discovery_incomplete" {
		t.Fatalf("expected structured discovery_incomplete error, got %#v", result)
	}
	if !strings.Contains(detail.Message, "explicit apiVersion") {
		t.Fatalf("missing recovery guidance: %s", detail.Message)
	}
	if len(detail.Candidates) != 1 || detail.Candidates[0].APIVersion != "example.com/v1" {
		t.Fatalf("missing known candidate: %#v", detail.Candidates)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["error"] != detail {
		t.Fatalf("structured content does not retain the discovery error: %#v", result.StructuredContent)
	}
	var text struct {
		Error kubernetes.APIDiscoveryError `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.Content), &text); err != nil {
		t.Fatalf("invalid text error JSON: %v", err)
	}
	if text.Error.Code != detail.Code || text.Error.Message != detail.Message || len(text.Error.Candidates) != 1 || text.Error.Candidates[0] != detail.Candidates[0] {
		t.Fatalf("text content does not retain the discovery error: %s", result.Content)
	}
	if discovery.openAPICalls != 0 {
		t.Fatalf("incomplete discovery reached OpenAPI %d times", discovery.openAPICalls)
	}
}

type apiResourcesTestDiscovery struct {
	apiResourcesListDiscovery
	openAPIV3    openapi.ClientWithContext
	openAPICalls int
}

func (d *apiResourcesTestDiscovery) OpenAPIV3WithContext(context.Context) openapi.ClientWithContext {
	d.openAPICalls++
	return d.openAPIV3
}

type apiResourcesTestOpenAPIV3Client struct {
	document map[string]any
}

func (c apiResourcesTestOpenAPIV3Client) PathsWithContext(context.Context) (map[string]openapi.GroupVersionWithContext, error) {
	return map[string]openapi.GroupVersionWithContext{
		"apis/example.com/v1": apiResourcesTestGroupVersion(c),
		"api/v1":              apiResourcesTestGroupVersion(c),
	}, nil
}

type apiResourcesTestGroupVersion struct {
	document map[string]any
}

func (g apiResourcesTestGroupVersion) SchemaWithContext(context.Context, string) ([]byte, error) {
	return json.Marshal(g.document)
}

func (apiResourcesTestGroupVersion) ServerRelativeURL() string {
	return "/openapi/v3/apis/example.com/v1?hash=test"
}
