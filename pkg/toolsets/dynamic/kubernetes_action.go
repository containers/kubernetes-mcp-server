package dynamic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/redact"
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
func (t *compiledTool) runKubernetes(action *compiledAction, env runEnv, vars evalVars) (any, error) {
	a := action.kubernetes
	gv, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid apiVersion %q: %w", a.APIVersion, err)
	}
	resource, namespaced, err := a.resolveResource(env.client, gv)
	if err != nil {
		return nil, err
	}
	client, err := newRESTClient(env.client.RESTConfig(), gv)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client: %w", err)
	}

	req := client.Verb(verbToMethod[a.Verb]).Resource(resource)
	if namespaced {
		namespace, all, err := t.resolveNamespace(a.Namespace, vars)
		if err != nil {
			return nil, err
		}
		if !all {
			req = req.Namespace(env.client.NamespaceOrDefault(namespace))
		}
	}
	name, err := t.resolveRef(a.Name, vars)
	if err != nil {
		return nil, err
	}
	if name == "" && a.Verb != "list" && a.Verb != "create" {
		return nil, fmt.Errorf("verb %q requires a resource name", a.Verb)
	}
	if name != "" {
		req = req.Name(name)
	}
	if a.Subresource != nil {
		subresource, err := t.resolveRef(a.Subresource, vars)
		if err != nil {
			return nil, err
		}
		if subresource == "" {
			return nil, fmt.Errorf("subresource resolved to an empty value")
		}
		req = req.SubResource(strings.Split(strings.Trim(subresource, "/"), "/")...)
	}
	query, err := t.resolveQuery(a.Query, action.queryExpr, vars)
	if err != nil {
		return nil, err
	}
	for key, value := range query {
		req = req.Param(key, value)
	}
	if a.Body != nil || a.Verb == "create" || a.Verb == "update" || a.Verb == "patch" {
		body, err := t.resolveBody(a.Body, action.bodyExpr, vars)
		if err != nil {
			return nil, err
		}
		contentType := "application/json"
		if a.Verb == "patch" {
			contentType = "application/merge-patch+json"
		}
		req = req.Body(body).SetHeader("Content-Type", contentType)
	}

	result := req.Do(env.ctx)
	// Result.Error decodes the API server's Status (e.g. `configmaps "x" not
	// found`); Raw alone would only yield client-go's generic message.
	if err := result.Error(); err != nil {
		return nil, err
	}
	raw, err := result.Raw()
	if err != nil {
		return nil, err
	}
	var contentType string
	result.ContentType(&contentType)
	return decodeResponseAs(raw, a.Response, contentType, a.MIMEType, a.Redact), nil
}

// decodeResponseAs applies the declared response mode: binary keeps the
// bytes with their media type, text keeps the body as a string, json (the
// default) decodes JSON and falls back to text. Redact removes secrets from
// text results.
func decodeResponseAs(raw []byte, mode, contentType, mimeType string, redactText bool) any {
	switch mode {
	case ResponseBinary:
		mediaType := mimeType
		if mediaType == "" {
			mediaType = contentType
		}
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}
		return &binaryResult{mimeType: mediaType, data: raw}
	case ResponseText:
		return redactIf(string(raw), redactText)
	}
	decoded := decodeResponse(raw)
	if text, ok := decoded.(string); ok {
		return redactIf(text, redactText)
	}
	return decoded
}

func redactIf(text string, enabled bool) string {
	if enabled {
		return redact.Secrets(text)
	}
	return text
}

// newRESTClient builds a REST client for one group/version on top of the
// caller's rest.Config. dynamic.ConfigFor supplies the same JSON negotiation
// the dynamic client uses, so API errors keep the server's own message
// (e.g. `configmaps "x" not found`) instead of a generic one.
// resolveResource returns the plural resource and whether it is namespaced,
// either from the explicit declaration or through the RESTMapper.
func (a *KubernetesAction) resolveResource(client api.KubernetesClient, gv schema.GroupVersion) (string, bool, error) {
	if a.Resource != "" {
		return a.Resource, a.Namespaced == nil || *a.Namespaced, nil
	}
	mapping, err := client.RESTMapper().RESTMapping(schema.GroupKind{Group: gv.Group, Kind: a.Kind}, gv.Version)
	if err != nil {
		return "", false, fmt.Errorf("failed to resolve %s/%s: %w", a.APIVersion, a.Kind, err)
	}
	return mapping.Resource.Resource, mapping.Scope.Name() == meta.RESTScopeNameNamespace, nil
}

// resolveQuery merges the query produced by the expression (if any) with the
// declared references; references win.
func (t *compiledTool) resolveQuery(refs map[string]Ref, expr *expression, vars evalVars) (map[string]string, error) {
	query := map[string]string{}
	if expr != nil {
		values, err := expr.evalStringMap(vars)
		if err != nil {
			return nil, err
		}
		query = values
	}
	for key, ref := range refs {
		value, err := t.resolveRef(&ref, vars)
		if err != nil {
			return nil, err
		}
		if value != "" {
			query[key] = value
		}
	}
	return query, nil
}

func newRESTClient(restConfig *rest.Config, gv schema.GroupVersion) (rest.Interface, error) {
	cfg := dynamic.ConfigFor(restConfig)
	cfg.GroupVersion = &gv
	cfg.APIPath = "/apis"
	if gv.Group == "" {
		cfg.APIPath = "/api"
	}
	return rest.RESTClientFor(cfg)
}

// resolveRef resolves a reference: argument, then expression, then the fixed
// value with `{argument}` placeholders expanded. Placeholders are substituted
// verbatim: client-go validates and escapes path segments itself (and
// rejects a '%' in a name or subresource) and encodes query parameters.
func (t *compiledTool) resolveRef(ref *Ref, vars evalVars) (string, error) {
	if ref == nil {
		return "", nil
	}
	if ref.Argument != "" {
		if value := argString(vars.args, ref.Argument); value != "" {
			return value, nil
		}
	}
	if ref.Expression != "" {
		expr, ok := t.refExprs[ref.Expression]
		if !ok {
			return "", fmt.Errorf("expression %q was not compiled", ref.Expression)
		}
		value, err := expr.eval(vars)
		if err != nil {
			return "", err
		}
		if s := scalarString(value); s != "" {
			return s, nil
		}
	}
	if ref.Value != "" {
		return expandPlaceholders(ref.Value, vars.args, func(value string) string { return value })
	}
	return "", nil
}

// resolveNamespace returns the namespace to use and whether the request spans
// all namespaces.
func (t *compiledTool) resolveNamespace(ref *NamespaceRef, vars evalVars) (namespace string, all bool, err error) {
	if ref == nil {
		return "", false, nil
	}
	if namespace, err = t.resolveRef(ref.ref(), vars); err != nil || namespace != "" {
		return namespace, false, err
	}
	return "", ref.All, nil
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
func (t *compiledTool) resolveBody(body *Body, bodyExpr *expression, vars evalVars) ([]byte, error) {
	switch {
	case body == nil:
		return []byte("{}"), nil
	case body.Argument != "":
		value, ok := vars.args[body.Argument]
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
		value, err := bodyExpr.eval(vars)
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
