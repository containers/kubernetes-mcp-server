package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/openapi"
)

const (
	MaxAPIResourceResultBytes = 16 * 1024
	MaxSchemaResultBytes      = 24 * 1024
	MaxSchemaPathSegments     = 32
	maxSchemaWork             = 100000
)

type APIResourceListOptions struct {
	Kind       string
	Query      string
	Group      *string
	APIVersion string
	Continue   string
	Denied     []config.GroupVersionKind
}

type APIResourceSummary struct {
	APIResourceInfo
	Description            string `json:"description,omitempty"`
	DescriptionUnavailable bool   `json:"descriptionUnavailable,omitempty"`
}

type APIResourcePage struct {
	Resources []APIResourceSummary `json:"resources"`
	Total     int                  `json:"total"`
	Returned  int                  `json:"returned"`
	Remaining int                  `json:"remaining"`
	Complete  bool                 `json:"complete"`
	Continue  string               `json:"continue,omitempty"`
	Warning   string               `json:"warning,omitempty"`
	Message   string               `json:"message,omitempty"`
}

type ExplainArguments struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	FieldPath  string `json:"fieldPath,omitempty"`
}

type SchemaOmission struct {
	Path    string           `json:"path"`
	Message string           `json:"message"`
	Expand  ExplainArguments `json:"expand"`
}

type SchemaAncestor struct {
	Path   string         `json:"path"`
	Schema map[string]any `json:"schema"`
}

type APIResourceExplainResult struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	FieldPath  string           `json:"fieldPath"`
	Schema     map[string]any   `json:"schema"`
	Components map[string]any   `json:"components,omitempty"`
	Ancestors  []SchemaAncestor `json:"ancestors,omitempty"`
	Complete   bool             `json:"complete"`
	Omissions  []SchemaOmission `json:"omissions"`
	SchemaHash string           `json:"schemaHash,omitempty"`
	Warning    string           `json:"warning,omitempty"`
}

// APIDiscoveryError carries actionable tool failures without masquerading as a schema.
type APIDiscoveryError struct {
	Code       string             `json:"code"`
	Message    string             `json:"message"`
	Candidates []ExplainArguments `json:"candidates,omitempty"`
	Fields     []string           `json:"fields,omitempty"`
}

func (e *APIDiscoveryError) Error() string { return e.Message }

func discoveryError(code, message string) *APIDiscoveryError {
	return &APIDiscoveryError{Code: code, Message: message}
}

type schemaDocument struct {
	schemas map[string]any
	hash    string
}

// schemaReader shares fetched group/version documents only for the lifetime of one call.
type schemaReader struct {
	ctx       context.Context
	client    openapi.ClientWithContext
	paths     map[string]openapi.GroupVersionWithContext
	documents map[string]schemaDocument
	failures  map[string]error
}

func newSchemaReader(ctx context.Context, disc discovery.DiscoveryInterface) (*schemaReader, error) {
	var client openapi.ClientWithContext
	if contextual, ok := disc.(discovery.OpenAPIV3SchemaInterfaceWithContext); ok {
		client = contextual.OpenAPIV3WithContext(ctx)
	} else if legacy, ok := disc.(discovery.OpenAPIV3SchemaInterface); ok {
		client = discovery.OpenAPIV3ToSchemaInterfaceWithContext(legacy).OpenAPIV3WithContext(ctx)
	}
	if client == nil {
		return nil, errors.New("Kubernetes discovery client does not support OpenAPI v3")
	}
	paths, err := client.PathsWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("read OpenAPI v3 index: %w", err)
	}
	return &schemaReader{ctx: ctx, client: client, paths: paths, documents: map[string]schemaDocument{}, failures: map[string]error{}}, nil
}

func (r *schemaReader) document(apiVersion string) (schemaDocument, error) {
	if doc, ok := r.documents[apiVersion]; ok {
		return doc, nil
	}
	if err, ok := r.failures[apiVersion]; ok {
		return schemaDocument{}, err
	}
	doc, err := r.fetch(apiVersion)
	if err != nil {
		r.failures[apiVersion] = err
	} else {
		r.documents[apiVersion] = doc
	}
	return doc, err
}

func (r *schemaReader) fetch(apiVersion string) (schemaDocument, error) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil || gv.Version == "" {
		return schemaDocument{}, fmt.Errorf("invalid apiVersion %q; use group/version or v1", apiVersion)
	}
	key := "apis/" + apiVersion
	if gv.Group == "" {
		key = "api/" + gv.Version
	}
	entry, ok := r.paths[key]
	if !ok {
		return schemaDocument{}, fmt.Errorf("OpenAPI v3 schema for %s is unavailable", apiVersion)
	}
	data, err := entry.SchemaWithContext(r.ctx, "application/json")
	if err != nil {
		return schemaDocument{}, fmt.Errorf("read OpenAPI v3 schema for %s: %w", apiVersion, err)
	}
	var document struct {
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return schemaDocument{}, fmt.Errorf("decode OpenAPI v3 schema for %s: %w", apiVersion, err)
	}
	if document.Components.Schemas == nil {
		return schemaDocument{}, fmt.Errorf("OpenAPI v3 document for %s has no component schemas", apiVersion)
	}
	u, err := url.Parse(entry.ServerRelativeURL())
	if err != nil {
		return schemaDocument{}, fmt.Errorf("invalid OpenAPI schema location: %w", err)
	}
	hash := u.Query().Get("hash")
	if hash == "" {
		hash = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return schemaDocument{schemas: document.Components.Schemas, hash: hash}, nil
}

func findResourceSchema(schemas map[string]any, group, version, kind string) (map[string]any, error) {
	var match map[string]any
	for _, value := range schemas {
		node, ok := value.(map[string]any)
		if !ok || !schemaMatchesGVK(node, group, version, kind) {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("OpenAPI v3 schema has multiple definitions for %s %s/%s", kind, group, version)
		}
		match = node
	}
	if match == nil {
		return nil, fmt.Errorf("OpenAPI v3 schema does not define %s %s/%s", kind, group, version)
	}
	return match, nil
}

func schemaMatchesGVK(node map[string]any, group, version, kind string) bool {
	values, ok := node["x-kubernetes-group-version-kind"].([]any)
	if !ok {
		values = []any{node["x-kubernetes-group-version-kind"]}
	}
	for _, value := range values {
		gvk, ok := value.(map[string]any)
		if ok && gvk["group"] == group && gvk["version"] == version && strings.EqualFold(fmt.Sprint(gvk["kind"]), kind) {
			return true
		}
	}
	return false
}

func shortDescription(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	summary := text
	if end := strings.Index(summary, ". "); end >= 0 {
		summary = summary[:end+1]
	}
	if summary == text && len([]rune(summary)) <= 100 {
		return summary
	}
	runes := []rune(summary)
	if len(runes) > 97 {
		summary = string(runes[:97])
	}
	return strings.TrimRight(strings.TrimSpace(summary), ".") + "..."
}

func (c *Core) APIResourceInventory(ctx context.Context, options APIResourceListOptions) (APIResourcePage, error) {
	page := APIResourcePage{Resources: []APIResourceSummary{}, Complete: true}
	resources, discoveryErr := c.APIResourcesList(ctx, "")
	if discoveryErr != nil && len(resources) == 0 {
		return page, discoveryErr
	}
	warnings := []string{}
	if discoveryErr != nil {
		warnings = append(warnings, fmt.Sprintf("Discovery is incomplete: %v", discoveryErr))
		page.Complete = false
	}
	reader, readerErr := newSchemaReader(ctx, c.RawDiscoveryClient())
	if readerErr != nil {
		warnings = append(warnings, fmt.Sprintf("Descriptions unavailable: %v", readerErr))
	}
	reported := map[string]bool{}
	fingerprints := map[string]string{}
	filtered := []APIResourceSummary{}
	query := strings.ToLower(options.Query)
	for _, resource := range resources {
		if config.IsDeniedResource(resource.APIVersion, resource.Kind, options.Denied) {
			continue
		}
		gv, err := schema.ParseGroupVersion(resource.APIVersion)
		if err != nil {
			return page, err
		}
		if options.Group != nil && *options.Group != gv.Group || options.APIVersion != "" && options.APIVersion != resource.APIVersion || options.Kind != "" && !strings.EqualFold(options.Kind, resource.Kind) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(strings.Join([]string{resource.Kind, resource.Name, gv.Group}, " ")), query) {
			continue
		}
		entry := APIResourceSummary{APIResourceInfo: resource}
		var description string
		if readerErr == nil {
			doc, err := reader.document(resource.APIVersion)
			if err == nil {
				fingerprints[resource.APIVersion] = doc.hash
				var node map[string]any
				node, err = findResourceSchema(doc.schemas, gv.Group, gv.Version, resource.Kind)
				if err == nil {
					description, _ = node["description"].(string)
				}
			}
			if err != nil && !reported[resource.APIVersion] {
				warnings = append(warnings, fmt.Sprintf("Descriptions unavailable for %s: %v", resource.APIVersion, err))
				reported[resource.APIVersion] = true
			}
			entry.DescriptionUnavailable = err != nil || description == ""
		} else {
			entry.DescriptionUnavailable = true
		}
		entry.Description = shortDescription(description)
		filtered = append(filtered, entry)
	}
	if err := ctx.Err(); err != nil {
		return page, err
	}
	sort.Slice(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		if a.APIVersion != b.APIVersion {
			return a.APIVersion < b.APIVersion
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
	page.Warning = strings.Join(warnings, "\n")
	if len(page.Warning) > 2048 {
		page.Warning = validPrefix(page.Warning, 2048) + " [additional warnings omitted]"
	}
	fingerprintData, err := json.Marshal([]any{filtered, options.Kind, options.Query, options.Group, options.APIVersion, options.Denied, fingerprints, page.Warning, page.Complete})
	if err != nil {
		return page, err
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(fingerprintData))
	var cursor struct {
		Offset      int    `json:"offset"`
		Fingerprint string `json:"fingerprint"`
	}
	if options.Continue != "" {
		data, err := base64.RawURLEncoding.DecodeString(options.Continue)
		if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Offset < 0 || cursor.Offset >= len(filtered) || cursor.Fingerprint != fingerprint {
			return page, discoveryError("invalid_continue", "Continuation is invalid or discovery/filters changed. Restart api_resources_list with the same filters and no continue.")
		}
	}
	makePage := func(end int) APIResourcePage {
		p := page
		p.Total, p.Returned, p.Remaining = len(filtered), end-cursor.Offset, len(filtered)-end
		p.Resources = filtered[cursor.Offset:end]
		if end < len(filtered) {
			encoded, _ := json.Marshal(struct {
				Offset      int    `json:"offset"`
				Fingerprint string `json:"fingerprint"`
			}{end, fingerprint})
			p.Continue = base64.RawURLEncoding.EncodeToString(encoded)
			p.Complete = false
			p.Message = fmt.Sprintf("%d more types. Call api_resources_list with the same filters and this continue token, or narrow query.", p.Remaining)
		}
		return p
	}
	end := len(filtered)
	for low, high := cursor.Offset, len(filtered); low <= high; {
		mid := low + (high-low)/2
		candidate := makePage(mid)
		data, err := json.Marshal(candidate)
		if err != nil {
			return page, err
		}
		if len(data) <= MaxAPIResourceResultBytes {
			end = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	page = makePage(end)
	data, err := json.Marshal(page)
	if err != nil {
		return page, err
	}
	if len(data) > MaxAPIResourceResultBytes || len(filtered) > cursor.Offset && page.Returned == 0 {
		return APIResourcePage{}, discoveryError("inventory_too_large", "A discovery entry and its warnings exceed the 16 KiB response budget. Narrow kind, group or apiVersion.")
	}
	return page, nil
}

func validPrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func (c *Core) APIResourceExplain(ctx context.Context, apiVersion, kind, fieldPath string, denied []config.GroupVersionKind) (*APIResourceExplainResult, error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil, discoveryError("invalid_kind", "kind must be a non-empty Kubernetes kind.")
	}
	if apiVersion == "" {
		resources, err := c.APIResourcesList(ctx, "")
		matches := []APIResourceInfo{}
		for _, r := range resources {
			if !config.IsDeniedResource(r.APIVersion, r.Kind, denied) {
				matches = append(matches, r)
			}
		}
		candidates := []ExplainArguments{}
		seen := map[string]bool{}
		for _, r := range matches {
			if strings.EqualFold(r.Kind, kind) && !seen[r.APIVersion] {
				candidates = append(candidates, ExplainArguments{Kind: r.Kind, APIVersion: r.APIVersion})
				seen[r.APIVersion] = true
			}
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].APIVersion < candidates[j].APIVersion })
		if err != nil {
			e := discoveryError("discovery_incomplete", fmt.Sprintf("Cannot resolve kind %q because discovery is incomplete: %v. Additional matching kinds may exist. Retry after discovery recovers or supply an explicit apiVersion.", kind, err))
			e.Candidates = candidates
			if len(candidates) == 0 {
				e.Message += " No permitted exact matches were discovered; use a known apiVersion or retry api_resources_list when discovery recovers."
			} else {
				e.Message += " Known permitted matches are listed in candidates; select one only if it is the intended type. This list may be incomplete."
			}
			return nil, e
		}
		if len(candidates) != 1 {
			e := discoveryError("unknown_kind", fmt.Sprintf("Kind %q was not found. Use api_resources_list without a filter to discover available types.", kind))
			if len(candidates) > 1 {
				e.Code = "ambiguous_kind"
				e.Message = fmt.Sprintf("Kind %q exists in multiple API groups. Retry with one candidate's apiVersion.", kind)
				e.Candidates = candidates
			} else {
				for _, r := range matches {
					if closeName(kind, r.Kind) && len(e.Candidates) < 3 {
						e.Candidates = append(e.Candidates, ExplainArguments{Kind: r.Kind, APIVersion: r.APIVersion})
					}
				}
			}
			return nil, e
		}
		apiVersion, kind = candidates[0].APIVersion, candidates[0].Kind
	}
	for _, rule := range denied {
		if (rule.Kind == "" || strings.EqualFold(rule.Kind, kind)) && config.IsDeniedResource(apiVersion, rule.Kind, []config.GroupVersionKind{rule}) {
			return nil, discoveryError("schema_denied", fmt.Sprintf("Schema access denied for %s %s.", kind, apiVersion))
		}
	}
	reader, err := newSchemaReader(ctx, c.RawDiscoveryClient())
	if err != nil {
		return nil, err
	}
	doc, err := reader.document(apiVersion)
	if err != nil {
		return nil, err
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, err
	}
	root, err := findResourceSchema(doc.schemas, gv.Group, gv.Version, kind)
	if err != nil {
		return nil, err
	}
	values, ok := root["x-kubernetes-group-version-kind"].([]any)
	if !ok {
		values = []any{root["x-kubernetes-group-version-kind"]}
	}
	for _, value := range values {
		if gvk, ok := value.(map[string]any); ok && gvk["group"] == gv.Group && gvk["version"] == gv.Version {
			if canonical, ok := gvk["kind"].(string); ok && strings.EqualFold(canonical, kind) {
				kind = canonical
				break
			}
		}
	}
	selected, ancestors, err := schemaAtFieldPath(root, doc.schemas, fieldPath)
	if err != nil {
		return nil, err
	}
	result := &APIResourceExplainResult{APIVersion: apiVersion, Kind: kind, FieldPath: fieldPath, Schema: selected, Ancestors: ancestors, Complete: true, Omissions: []SchemaOmission{}, SchemaHash: doc.hash}
	if err := renderSchema(ctx, result, doc.schemas); err != nil {
		return nil, err
	}
	return result, nil
}

func closeName(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	if strings.Contains(b, a) || strings.Contains(a, b) {
		return true
	}
	ar, br := []rune(a), []rune(b)
	if len(ar) > 128 || len(br) > 128 {
		return false
	}
	previous := make([]int, len(br)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, x := range ar {
		current := make([]int, len(br)+1)
		current[0] = i + 1
		for j, y := range br {
			cost := 0
			if x != y {
				cost = 1
			}
			current[j+1] = min(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return previous[len(br)] <= 2
}

func joinFieldPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func localSchemaReference(ref string) (string, bool) {
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(ref, prefix)
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return strings.ReplaceAll(strings.ReplaceAll(name, "~1", "/"), "~0", "~"), true
}
