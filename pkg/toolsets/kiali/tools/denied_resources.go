package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
)

// ExecuteManageIstioConfigRead enforces denied_resources, calls Kiali, and
// strips denied GVKs from list responses so prompts/tools cannot disclose them.
func ExecuteManageIstioConfigRead(ctx context.Context, kiali *kialiclient.Kiali, cfg *config.Config, arguments map[string]any) (string, error) {
	if err := enforceIstioConfigDenied(cfg, arguments); err != nil {
		return "", err
	}
	content, err := kiali.ExecuteRequest(ctx, KialiManageIstioConfigReadEndpoint, remapMeshCluster(arguments))
	if err != nil {
		return "", err
	}
	return filterDeniedIstioConfigList(content, deniedResources(cfg))
}

// ExecuteManageIstioConfig enforces denied_resources before create/patch/delete.
func ExecuteManageIstioConfig(ctx context.Context, kiali *kialiclient.Kiali, cfg *config.Config, arguments map[string]any) (string, error) {
	if err := enforceIstioConfigDenied(cfg, arguments); err != nil {
		return "", err
	}
	return kiali.ExecuteRequest(ctx, KialiManageIstioConfigEndpoint, remapMeshCluster(arguments))
}

func deniedResources(cfg *config.Config) []config.GroupVersionKind {
	if cfg == nil {
		return nil
	}
	return cfg.DeniedResources.Get()
}

func enforceIstioConfigDenied(cfg *config.Config, arguments map[string]any) error {
	denied := deniedResources(cfg)
	if len(denied) == 0 {
		return nil
	}

	group := argumentString(arguments, "group")
	version := argumentString(arguments, "version")
	kind := argumentString(arguments, "kind")
	if group == "" && version == "" && kind == "" {
		// Broad list: filter denied GVKs from the response after the call.
		return nil
	}
	if version == "" {
		return nil
	}

	apiVersion := version
	if group != "" {
		apiVersion = group + "/" + version
	}
	if config.IsDeniedResource(apiVersion, kind, denied) {
		return deniedResourceError(group, version, kind)
	}
	return nil
}

func deniedResourceError(group, version, kind string) error {
	return fmt.Errorf("resource not allowed: %s", schema.GroupVersionKind{
		Group:   group,
		Version: version,
		Kind:    kind,
	}.String())
}

func argumentString(arguments map[string]any, key string) string {
	if arguments == nil {
		return ""
	}
	v, ok := arguments[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// filterDeniedIstioConfigList removes denied GVK entries from a Kiali list
// payload shaped as {namespaces:{ns:{"group/version/kind":{valid,invalid}}}}.
// Non-list payloads (e.g. get YAML) are returned unchanged.
func filterDeniedIstioConfigList(content string, denied []config.GroupVersionKind) (string, error) {
	if len(denied) == 0 || content == "" {
		return content, nil
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return content, nil
	}
	namespaces, ok := root["namespaces"].(map[string]any)
	if !ok {
		return content, nil
	}

	changed := false
	for ns, nsVal := range namespaces {
		nsMap, ok := nsVal.(map[string]any)
		if !ok {
			continue
		}
		for gvkKey := range nsMap {
			group, version, kind, parsed := parseGroupVersionKindKey(gvkKey)
			if !parsed {
				continue
			}
			apiVersion := version
			if group != "" {
				apiVersion = group + "/" + version
			}
			if config.IsDeniedResource(apiVersion, kind, denied) {
				delete(nsMap, gvkKey)
				changed = true
			}
		}
		namespaces[ns] = nsMap
	}
	if !changed {
		return content, nil
	}

	out, err := json.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("failed to marshal filtered istio config list: %w", err)
	}
	return string(out), nil
}

// parseGroupVersionKindKey parses Kiali list keys of the form
// "group/version/kind" (group may contain dots / slashes via Join of leading parts).
func parseGroupVersionKindKey(key string) (group, version, kind string, ok bool) {
	parts := strings.Split(key, "/")
	if len(parts) < 2 {
		return "", "", "", false
	}
	kind = parts[len(parts)-1]
	version = parts[len(parts)-2]
	if kind == "" || version == "" {
		return "", "", "", false
	}
	group = strings.Join(parts[:len(parts)-2], "/")
	return group, version, kind, true
}
