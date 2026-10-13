package dynamic

import (
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

var verbToMethod = map[string]string{
	"get":    http.MethodGet,
	"list":   http.MethodGet,
	"create": http.MethodPost,
	"update": http.MethodPut,
	"patch":  http.MethodPatch,
	"delete": http.MethodDelete,
}

// runKubernetes performs the declared request through a REST client derived
// from the caller's rest.Config, so the access-control round tripper, the
// validators, confirmation rules and the caller's identity all apply exactly
// as they do for built-in tools.
func (t *compiledTool) runKubernetes(params api.ToolHandlerParams, args map[string]any) (any, error) {
	a := t.spec.Kubernetes
	gv, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid apiVersion %q: %w", a.APIVersion, err)
	}
	mapping, err := params.RESTMapper().RESTMapping(schema.GroupKind{Group: gv.Group, Kind: a.Kind}, gv.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s/%s: %w", a.APIVersion, a.Kind, err)
	}
	client, err := newRESTClient(params.RESTConfig(), gv)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client: %w", err)
	}

	req := client.Verb(verbToMethod[a.Verb]).Resource(mapping.Resource.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if namespace, all := resolveNamespace(a.Namespace, args); !all {
			req = req.Namespace(params.NamespaceOrDefault(namespace))
		}
	}
	name := resolveRef(a.Name, args)
	if name == "" && a.Verb != "list" && a.Verb != "create" {
		return nil, fmt.Errorf("verb %q requires a resource name", a.Verb)
	}
	if name != "" {
		req = req.Name(name)
	}
	if a.Subresource != "" {
		req = req.SubResource(a.Subresource)
	}
	for key, ref := range a.Query {
		if value := resolveRef(&ref, args); value != "" {
			req = req.Param(key, value)
		}
	}
	if a.Body != nil || a.Verb == "create" || a.Verb == "update" || a.Verb == "patch" {
		body, err := t.resolveBody(a.Body, args)
		if err != nil {
			return nil, err
		}
		contentType := "application/json"
		if a.Verb == "patch" {
			contentType = "application/merge-patch+json"
		}
		req = req.Body(body).SetHeader("Content-Type", contentType)
	}

	result := req.Do(params)
	// Result.Error decodes the API server's Status (e.g. `configmaps "x" not
	// found`); Raw alone would only yield client-go's generic message.
	if err := result.Error(); err != nil {
		return nil, err
	}
	raw, err := result.Raw()
	if err != nil {
		return nil, err
	}
	return decodeResponse(raw), nil
}

// newRESTClient builds a REST client for one group/version on top of the
// caller's rest.Config. dynamic.ConfigFor supplies the same JSON negotiation
// the dynamic client uses, so API errors keep the server's own message
// (e.g. `configmaps "x" not found`) instead of a generic one.
func newRESTClient(restConfig *rest.Config, gv schema.GroupVersion) (rest.Interface, error) {
	cfg := dynamic.ConfigFor(restConfig)
	cfg.GroupVersion = &gv
	cfg.APIPath = "/apis"
	if gv.Group == "" {
		cfg.APIPath = "/api"
	}
	return rest.RESTClientFor(cfg)
}

func resolveRef(ref *Ref, args map[string]any) string {
	if ref == nil {
		return ""
	}
	if ref.Argument != "" {
		if value := argString(args, ref.Argument); value != "" {
			return value
		}
	}
	return ref.Value
}

// resolveNamespace returns the namespace to use and whether the request spans
// all namespaces.
func resolveNamespace(ref *NamespaceRef, args map[string]any) (namespace string, all bool) {
	if ref == nil {
		return "", false
	}
	if namespace = resolveRef(&ref.Ref, args); namespace != "" {
		return namespace, false
	}
	return "", ref.All
}

func argString(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

// resolveBody returns the JSON request body. A nil Body yields an empty JSON
// object, which is what subresource endpoints such as KubeVirt's
// virtualmachineinstances/pause expect.
func (t *compiledTool) resolveBody(body *Body, args map[string]any) ([]byte, error) {
	switch {
	case body == nil:
		return []byte("{}"), nil
	case body.Argument != "":
		value, ok := args[body.Argument]
		if !ok || value == nil {
			return nil, fmt.Errorf("%s parameter required", body.Argument)
		}
		if s, ok := value.(string); ok {
			out, err := yaml.YAMLToJSON([]byte(s))
			if err != nil {
				return nil, fmt.Errorf("%s parameter is not valid YAML or JSON: %w", body.Argument, err)
			}
			return out, nil
		}
		return json.Marshal(value)
	case len(body.Value) > 0:
		return body.Value, nil
	default:
		value, err := t.bodyExpr.eval(args, nil)
		if err != nil {
			return nil, err
		}
		return json.Marshal(value)
	}
}

// decodeResponse returns the JSON-decoded body, or the raw text when the body
// is not JSON (e.g. a plain-text subresource such as pods/log).
func decodeResponse(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	return decoded
}
