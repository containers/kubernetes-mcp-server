package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func schemaViews(node map[string]any, schemas map[string]any, seen map[string]bool, work *int, depth int) ([]map[string]any, error) {
	*work++
	if *work > maxSchemaWork || depth > 128 {
		return nil, discoveryError("schema_work_limit", "Schema reference/composition traversal exceeds its work/depth guard; request a narrower fieldPath.")
	}
	views := []map[string]any{node}
	if ref, ok := node["$ref"].(string); ok {
		name, valid := localSchemaReference(ref)
		if !valid {
			return nil, fmt.Errorf("unsupported schema reference %q; only local component schemas are supported", ref)
		}
		if seen[name] {
			return nil, fmt.Errorf("schema reference cycle %q cannot be traversed without a property step", ref)
		}
		target, ok := schemas[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unresolved schema reference %q", ref)
		}
		seen[name] = true
		nested, err := schemaViews(target, schemas, seen, work, depth+1)
		delete(seen, name)
		if err != nil {
			return nil, err
		}
		views = append(views, nested...)
	}
	if allOf, ok := node["allOf"].([]any); ok {
		for _, part := range allOf {
			child, ok := part.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid allOf schema")
			}
			nested, err := schemaViews(child, schemas, seen, work, depth+1)
			if err != nil {
				return nil, err
			}
			views = append(views, nested...)
		}
	}
	return views, nil
}

func constraintView(node map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range node {
		switch key {
		case "properties", "items", "additionalProperties", "description", "$ref", "allOf":
			continue
		}
		result[key] = value
	}
	return result
}

func combinedSchemas(nodes []map[string]any) map[string]any {
	if len(nodes) == 1 {
		return nodes[0]
	}
	parts := make([]any, 0, len(nodes))
	for _, node := range nodes {
		parts = append(parts, node)
	}
	return map[string]any{"allOf": parts}
}

func schemaAtFieldPath(root map[string]any, schemas map[string]any, path string) (map[string]any, []SchemaAncestor, error) {
	if path == "" {
		return root, nil, nil
	}
	if len(path) > 512 || strings.Count(path, ".") >= MaxSchemaPathSegments {
		return nil, nil, discoveryError("invalid_field_path", "fieldPath exceeds 512 bytes or 32 property segments.")
	}
	segments := strings.Split(path, ".")
	current := root
	ancestors := []SchemaAncestor{}
	currentPath := ""
	work := 0
	for _, segment := range segments {
		if segment == "" || segment == "[]" {
			return nil, nil, discoveryError("invalid_field_path", "Use dotted property names; arrays and map values are traversed implicitly, without [].")
		}
		for implicit := 0; ; implicit++ {
			if implicit > 128 {
				return nil, nil, discoveryError("schema_work_limit", "Too many implicit array/map levels in fieldPath.")
			}
			views, err := schemaViews(current, schemas, map[string]bool{}, &work, 0)
			if err != nil {
				return nil, nil, err
			}
			constraints := []map[string]any{}
			matches := []map[string]any{}
			containers := []map[string]any{}
			fields := map[string]bool{}
			for _, view := range views {
				if cv := constraintView(view); len(cv) > 0 {
					constraints = append(constraints, cv)
				}
				properties, _ := view["properties"].(map[string]any)
				for name := range properties {
					fields[name] = true
				}
				if child, ok := properties[segment].(map[string]any); ok {
					matches = append(matches, child)
				}
				for _, key := range []string{"items", "additionalProperties"} {
					if child, ok := view[key].(map[string]any); ok {
						containers = append(containers, child)
					}
				}
			}
			if len(constraints) > 0 {
				ancestors = append(ancestors, SchemaAncestor{Path: currentPath, Schema: combinedSchemas(constraints)})
			}
			if len(matches) > 0 {
				current = combinedSchemas(matches)
				currentPath = joinFieldPath(currentPath, segment)
				break
			}
			if len(containers) > 0 {
				current = combinedSchemas(containers)
				continue
			}
			names := make([]string, 0, len(fields))
			for name := range fields {
				names = append(names, name)
			}
			sort.Strings(names)
			suggestions := []string{}
			for _, name := range names {
				if closeName(segment, name) {
					suggestions = append(suggestions, joinFieldPath(currentPath, name))
					if len(suggestions) == 3 {
						break
					}
				}
			}
			message := fmt.Sprintf("Field %q does not exist at %q. Retry with an available child field.", segment, currentPath)
			if len(suggestions) > 0 {
				message += " Did you mean " + strings.Join(suggestions, ", ") + "?"
			}
			if len(names) > 50 {
				names = names[:50]
				message += " Showing the first 50 child names."
			}
			return nil, nil, &APIDiscoveryError{Code: "unknown_field", Message: message, Fields: names}
		}
	}
	return current, ancestors, nil
}

// visitSchema visits schema keywords only; examples/defaults may contain arbitrary user data.
func visitSchema(node map[string]any, path string, visit func(map[string]any, string) error, work *int, depth int) error {
	*work++
	if *work > maxSchemaWork || depth > 128 {
		return discoveryError("schema_work_limit", "Schema traversal exceeds its work/depth guard; request a narrower fieldPath.")
	}
	if err := visit(node, path); err != nil {
		return err
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			child, ok := properties[name].(map[string]any)
			if !ok {
				return fmt.Errorf("invalid schema for property %q", name)
			}
			if err := visitSchema(child, joinFieldPath(path, name), visit, work, depth+1); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"items", "additionalProperties", "not", "if", "then", "else"} {
		if child, ok := node[key].(map[string]any); ok {
			if err := visitSchema(child, path, visit, work, depth+1); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if parts, ok := node[key].([]any); ok {
			for _, part := range parts {
				child, ok := part.(map[string]any)
				if !ok {
					return fmt.Errorf("invalid %s schema", key)
				}
				if err := visitSchema(child, path, visit, work, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func referenceClosure(ctx context.Context, result *APIResourceExplainResult, schemas map[string]any) error {
	definitions := map[string]any{}
	work := 0
	var visit func(map[string]any, string) error
	visit = func(node map[string]any, path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if ref, ok := node["$ref"].(string); ok {
			name, valid := localSchemaReference(ref)
			if !valid {
				return fmt.Errorf("unsupported schema reference %q", ref)
			}
			if _, ok := definitions[name]; ok {
				return nil
			}
			target, ok := schemas[name].(map[string]any)
			if !ok {
				return fmt.Errorf("unresolved schema reference %q", ref)
			}
			definitions[name] = target
			return visitSchema(target, path, visit, &work, 0)
		}
		return nil
	}
	if err := visitSchema(result.Schema, result.FieldPath, visit, &work, 0); err != nil {
		return err
	}
	for _, ancestor := range result.Ancestors {
		if err := visitSchema(ancestor.Schema, ancestor.Path, visit, &work, 0); err != nil {
			return err
		}
	}
	result.Components = nil
	if len(definitions) > 0 {
		result.Components = map[string]any{"schemas": definitions}
	}
	return nil
}

func schemaResultSize(result *APIResourceExplainResult) (int, error) {
	data, err := json.Marshal(result)
	return len(data), err
}

func cloneSchema(node map[string]any) (map[string]any, error) {
	data, err := json.Marshal(node)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	err = json.Unmarshal(data, &result)
	return result, err
}

type schemaBranch struct {
	node map[string]any
	path string
	size int
}

func schemaBranches(ctx context.Context, result *APIResourceExplainResult, schemas map[string]any) ([]schemaBranch, error) {
	branches := []schemaBranch{}
	seen := map[string]bool{}
	work := 0
	var walk func(map[string]any, string, int) error
	walk = func(node map[string]any, path string, depth int) error {
		work++
		if work > maxSchemaWork || depth > 128 {
			return discoveryError("schema_work_limit", "Schema branch traversal exceeds its work/depth guard.")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Only the outer node at a property path is an independently expandable boundary.
		if path != result.FieldPath && !seen[path] && node["x-mcp-omitted"] != true {
			seen[path] = true
			view := &APIResourceExplainResult{Schema: node}
			if err := referenceClosure(ctx, view, schemas); err != nil {
				return err
			}
			size, err := schemaResultSize(view)
			if err != nil {
				return err
			}
			branches = append(branches, schemaBranch{node: node, path: path, size: size})
		}
		properties, _ := node["properties"].(map[string]any)
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if child, ok := properties[name].(map[string]any); ok {
				if err := walk(child, joinFieldPath(path, name), depth+1); err != nil {
					return err
				}
			}
		}
		for _, key := range []string{"items", "additionalProperties"} {
			if child, ok := node[key].(map[string]any); ok {
				if err := walk(child, path, depth+1); err != nil {
					return err
				}
			}
		}
		// Alternative/conditional schemas are not traversable by a unique fieldPath.
		if parts, ok := node["allOf"].([]any); ok {
			for _, part := range parts {
				if child, ok := part.(map[string]any); ok {
					if err := walk(child, path, depth+1); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := walk(result.Schema, result.FieldPath, 0); err != nil {
		return nil, err
	}
	sort.SliceStable(branches, func(i, j int) bool {
		if branches[i].size != branches[j].size {
			return branches[i].size > branches[j].size
		}
		return branches[i].path < branches[j].path
	})
	return branches, nil
}

// Expand only references at the selected boundary, including its implicit containers.
// Property references remain separate so each omission has an unambiguous recovery path.
func expandSchemaBoundary(node map[string]any, schemas map[string]any, seen map[string]bool, depth int) error {
	if depth > 128 {
		return discoveryError("schema_work_limit", "Schema boundary expansion exceeds 128 levels.")
	}
	if ref, ok := node["$ref"].(string); ok {
		name, _ := localSchemaReference(ref)
		if seen[name] {
			return fmt.Errorf("schema reference cycle %q has no expandable property boundary", ref)
		}
		target, ok := schemas[name].(map[string]any)
		if !ok {
			return fmt.Errorf("unresolved schema reference %q", ref)
		}
		copied, err := cloneSchema(target)
		if err != nil {
			return err
		}
		seen[name] = true
		if err := expandSchemaBoundary(copied, schemas, seen, depth+1); err != nil {
			return err
		}
		delete(seen, name)
		delete(node, "$ref")
		if len(node) == 0 {
			replaceSchema(node, copied)
		} else {
			siblings, err := cloneSchema(node)
			if err != nil {
				return err
			}
			replaceSchema(node, combinedSchemas([]map[string]any{siblings, copied}))
		}
	}
	for _, key := range []string{"items", "additionalProperties", "not", "if", "then", "else"} {
		if child, ok := node[key].(map[string]any); ok {
			if err := expandSchemaBoundary(child, schemas, seen, depth+1); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if parts, ok := node[key].([]any); ok {
			for _, part := range parts {
				if child, ok := part.(map[string]any); ok {
					if err := expandSchemaBoundary(child, schemas, seen, depth+1); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func collapseBranch(node map[string]any, schemas map[string]any) map[string]any {
	// Local constraints remain intact; descendants and referenced definitions are explicitly omitted.
	view := map[string]any{}
	for key, value := range node {
		switch key {
		case "properties", "items", "additionalProperties", "$ref", "allOf", "anyOf", "oneOf", "not", "if", "then", "else":
			continue
		}
		view[key] = value
	}
	if ref, ok := node["$ref"].(string); ok {
		name, _ := localSchemaReference(ref)
		if target, ok := schemas[name].(map[string]any); ok {
			for _, key := range []string{"type", "description", "title"} {
				if _, exists := view[key]; !exists {
					if v, ok := target[key]; ok {
						view[key] = v
					}
				}
			}
		}
	}
	view["x-mcp-omitted"] = true
	return view
}

func replaceSchema(target, source map[string]any) {
	clear(target)
	for key, value := range source {
		target[key] = value
	}
}

func renderSchema(ctx context.Context, result *APIResourceExplainResult, source map[string]any) error {
	// Only the selected view is mutable; reference definitions remain shared, read-only source data.
	schemas := source
	var err error
	result.Schema, err = cloneSchema(result.Schema)
	if err != nil {
		return err
	}
	for i := range result.Ancestors {
		result.Ancestors[i].Schema, err = cloneSchema(result.Ancestors[i].Schema)
		if err != nil {
			return err
		}
	}
	if err := referenceClosure(ctx, result, schemas); err != nil {
		return err
	}
	size, err := schemaResultSize(result)
	if err != nil {
		return err
	}
	if size <= MaxSchemaResultBytes {
		return nil
	}
	if err := expandSchemaBoundary(result.Schema, schemas, map[string]bool{}, 0); err != nil {
		return err
	}
	if err := referenceClosure(ctx, result, schemas); err != nil {
		return err
	}
	size, err = schemaResultSize(result)
	if err != nil {
		return err
	}
	if size <= MaxSchemaResultBytes {
		return nil
	}
	result.Complete = false
	for attempts := 0; attempts < 3; attempts++ {
		branches, err := schemaBranches(ctx, result, schemas)
		if err != nil {
			return err
		}
		var chosen *schemaBranch
		deficit := size - MaxSchemaResultBytes + 512
		for i := range branches {
			branch := &branches[i]
			collapsed := collapseBranch(branch.node, schemas)
			b, err := json.Marshal(collapsed)
			if err != nil {
				return err
			}
			if branch.size-len(b) < 512 {
				continue
			}
			if chosen == nil {
				chosen = branch
			}
			if branch.size-len(b) >= deficit {
				chosen = branch
			} else {
				break
			}
		}
		if chosen == nil {
			break
		}
		// Prefer a coarser boundary if three independent explanations would otherwise be exceeded.
		remaining := []SchemaOmission{}
		for _, omission := range result.Omissions {
			if omission.Path != chosen.path && !strings.HasPrefix(omission.Path, chosen.path+".") {
				remaining = append(remaining, omission)
			}
		}
		replaceSchema(chosen.node, collapseBranch(chosen.node, schemas))
		result.Omissions = append(remaining, SchemaOmission{
			Path:    chosen.path,
			Message: "Fields/referenced schema details and their constraints in this branch are omitted to fit the 24 KiB response budget. Call api_resources_explain with expand arguments if needed.",
			Expand:  ExplainArguments{Kind: result.Kind, APIVersion: result.APIVersion, FieldPath: chosen.path},
		})
		if err := referenceClosure(ctx, result, schemas); err != nil {
			return err
		}
		size, err = schemaResultSize(result)
		if err != nil {
			return err
		}
		if size <= MaxSchemaResultBytes {
			return nil
		}
	}
	// A single coarse description-shortening step is preferable to many per-string omissions.
	if len(result.Omissions) < 3 {
		changed := false
		expansionPath := ""
		work := 0
		shorten := func(node map[string]any, path string) error {
			if path == result.FieldPath {
				return nil
			}
			if text, ok := node["description"].(string); ok && len([]rune(text)) > 100 {
				node["description"] = shortDescription(text)
				changed = true
				if expansionPath == "" {
					expansionPath = path
				}
			}
			return nil
		}
		if err := visitSchema(result.Schema, result.FieldPath, shorten, &work, 0); err != nil {
			return err
		}
		if changed {
			result.Schema["x-mcp-omitted"] = true
			result.Omissions = append(result.Omissions, SchemaOmission{
				Path: result.FieldPath, Message: "Long descriptions in this view are shortened to first-sentence summaries of at most 100 characters to fit 24 KiB. Request a narrower fieldPath for full descriptions.",
				Expand: ExplainArguments{Kind: result.Kind, APIVersion: result.APIVersion, FieldPath: expansionPath},
			})
			size, err = schemaResultSize(result)
			if err != nil {
				return err
			}
			if size <= MaxSchemaResultBytes {
				return nil
			}
		}
	}
	fields := []string{}
	if properties, ok := result.Schema["properties"].(map[string]any); ok {
		for name := range properties {
			fields = append(fields, joinFieldPath(result.FieldPath, name))
		}
		sort.Strings(fields)
	}
	if len(fields) > 20 {
		fields = fields[:20]
	}
	return &APIDiscoveryError{Code: "schema_too_large", Message: "A useful schema view and its validation context cannot fit the 24 KiB response budget. Retry with a narrower fieldPath from the available fields; large individual constraints are not silently cut.", Fields: fields}
}
