package dynamic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
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
	if err := a.checkQueryKeys(query); err != nil {
		return nil, err
	}
	if t.paginates() && action == t.action {
		// The API server pages the list: limit from the model (clamped), the
		// continue token from the previous page.
		query["limit"] = []string{strconv.Itoa(t.effectiveLimit(vars.args))}
		if token := argString(vars.args, ContinueArgument); token != "" {
			query["continue"] = []string{token}
		}
	}
	for key, values := range query {
		for _, value := range values {
			req = req.Param(key, value)
		}
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
	return sanitizeObjects(decodeResponseAs(raw, a.Response, contentType, a.MIMEType, a.Redact)), nil
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
	if mode != ResponseJSON && !isJSONContentType(contentType) {
		// A text body that happens to parse as JSON (a container log whose
		// last lines are one JSON document) must stay text: the server said
		// what it is.
		return redactIf(string(raw), redactText)
	}
	decoded := decodeResponse(raw)
	if text, ok := decoded.(string); ok {
		return redactIf(text, redactText)
	}
	return decoded
}

// isJSONContentType reports whether a Content-Type announces JSON. An
// absent Content-Type is taken as JSON, which is what every Kubernetes API
// and the observability backends send; "+json" covers the structured
// syntax suffix (application/merge-patch+json, application/problem+json).
func isJSONContentType(contentType string) bool {
	mediaType, _, _ := strings.Cut(strings.TrimSpace(contentType), ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	switch {
	case mediaType == "", mediaType == "application/json", mediaType == "text/json":
		return true
	case strings.HasSuffix(mediaType, "+json"):
		return true
	}
	return false
}

func redactIf(text string, enabled bool) string {
	if enabled {
		return redact.Secrets(text)
	}
	return text
}

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
func (t *compiledTool) resolveQuery(refs map[string]Ref, expr *expression, vars evalVars) (map[string][]string, error) {
	query := map[string][]string{}
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
			query[key] = []string{value}
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
	cfg.Wrap(limitResponseBodies)
	return rest.RESTClientFor(cfg)
}

// reservedKubernetesParams are query parameters a Tool may only set through
// a declared `query` entry, never through a model-influenced
// queryExpression: they turn a request into a stream (watch, follow), change
// what a write does (dryRun, force, propagationPolicy, ...) or its timing.
var reservedKubernetesParams = map[string]bool{
	"watch": true, "follow": true, "allowWatchBookmarks": true, "sendInitialEvents": true,
	"dryRun": true, "force": true, "fieldManager": true, "fieldValidation": true,
	"propagationPolicy": true, "orphanDependents": true, "gracePeriodSeconds": true,
	"timeout": true, "timeoutSeconds": true, "resourceVersionMatch": true,
}

// checkQueryKeys refuses reserved parameters that came from the
// queryExpression rather than from a declared query entry.
func (a *KubernetesAction) checkQueryKeys(query map[string][]string) error {
	for key := range query {
		if _, declared := a.Query[key]; declared {
			continue
		}
		if reservedKubernetesParams[key] {
			return fmt.Errorf("query parameter %q can only be set by a declared query entry, not by queryExpression", key)
		}
	}
	return nil
}

// limitResponseBodies caps the body of every response the Kubernetes action
// reads, as readResponseBody does for http actions: a pods/log or a large
// list is read into memory in full by client-go, so the cap lives in the
// transport.
func limitResponseBodies(rt http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := rt.RoundTrip(req)
		if err != nil || resp == nil || resp.Body == nil {
			return resp, err
		}
		resp.Body = &limitedBody{ReadCloser: resp.Body, remaining: maxResponseBytes}
		return resp, nil
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// limitedBody fails the read, rather than truncating, once more than the
// allowed number of bytes has been delivered.
type limitedBody struct {
	io.ReadCloser
	remaining int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, fmt.Errorf("response exceeds the maximum size of %d bytes", maxResponseBytes)
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return 0, fmt.Errorf("response exceeds the maximum size of %d bytes", maxResponseBytes)
	}
	return n, err
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
	// Arguments arrive as JSON numbers (float64) or are normalized to int64;
	// either way a whole number renders as digits, never as 1e+06.
	return scalarString(value)
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

// decodeResponse decodes a JSON body into plain Go values. Numbers are
// decoded as json.Number first so that whole numbers become int64, like the
// results of CEL expressions, and only fractional ones become float64: an
// integer is an integer wherever it came from, and CEL arithmetic between a
// response field and a step result does not fail on int/double mixing.
func decodeResponse(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return string(raw)
	}
	if decoder.More() {
		// Trailing content: not one JSON document (a log with a JSON line).
		return string(raw)
	}
	return normalizeNumbers(decoded)
}

// normalizeNumbers walks a decoded JSON value turning json.Number into
// int64 when whole (and in range) and float64 otherwise, and whole float64
// values (tool arguments decoded by the MCP SDK) into int64.
func normalizeNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1<<53 {
			return int64(v)
		}
		return v
	case map[string]any:
		for key, entry := range v {
			v[key] = normalizeNumbers(entry)
		}
		return v
	case []any:
		for i, entry := range v {
			v[i] = normalizeNumbers(entry)
		}
		return v
	}
	return value
}

// sanitizeObjects removes what no model wants from a Kubernetes object or
// list: managedFields and the last-applied-configuration annotation, as
// kubectl does.
func sanitizeObjects(decoded any) any {
	object, ok := decoded.(map[string]any)
	if !ok {
		return decoded
	}
	sanitizeObject(object)
	if items, ok := object["items"].([]any); ok {
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				sanitizeObject(m)
			}
		}
	}
	return object
}

func sanitizeObject(object map[string]any) {
	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		return
	}
	delete(metadata, "managedFields")
	if annotations, ok := metadata["annotations"].(map[string]any); ok {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		if len(annotations) == 0 {
			delete(metadata, "annotations")
		}
	}
}
