package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func TestEnforceIstioConfigDenied(t *testing.T) {
	cfg := config.New()
	cfg.DeniedResources.SetForTest([]config.GroupVersionKind{{
		Group:   "networking.istio.io",
		Version: "v1",
		Kind:    "VirtualService",
	}})

	t.Run("blocks denied get gvk", func(t *testing.T) {
		err := enforceIstioConfigDenied(cfg, map[string]any{
			"action":  "get",
			"group":   "networking.istio.io",
			"version": "v1",
			"kind":    "VirtualService",
			"object":  "routing-proof",
		})
		if err == nil {
			t.Fatal("expected deny error")
		}
		if !strings.Contains(err.Error(), "resource not allowed") {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(err.Error(), "VirtualService") {
			t.Fatalf("error should name VirtualService: %v", err)
		}
	})

	t.Run("allows other kinds", func(t *testing.T) {
		err := enforceIstioConfigDenied(cfg, map[string]any{
			"action":  "get",
			"group":   "networking.istio.io",
			"version": "v1",
			"kind":    "DestinationRule",
			"object":  "details",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("allows broad list for post-filter", func(t *testing.T) {
		err := enforceIstioConfigDenied(cfg, map[string]any{"action": "list"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("blocks denied write gvk", func(t *testing.T) {
		err := enforceIstioConfigDenied(cfg, map[string]any{
			"action":  "patch",
			"group":   "networking.istio.io",
			"version": "v1",
			"kind":    "VirtualService",
			"object":  "routing-proof",
		})
		if err == nil {
			t.Fatal("expected deny error")
		}
	})

	t.Run("blocks entire group version wildcard", func(t *testing.T) {
		wildcard := config.New()
		wildcard.DeniedResources.SetForTest([]config.GroupVersionKind{{
			Group:   "networking.istio.io",
			Version: "v1",
		}})
		err := enforceIstioConfigDenied(wildcard, map[string]any{
			"action":  "list",
			"group":   "networking.istio.io",
			"version": "v1",
		})
		if err == nil {
			t.Fatal("expected deny error for wildcard GV")
		}
	})
}

func TestFilterDeniedIstioConfigList(t *testing.T) {
	denied := []config.GroupVersionKind{{
		Group:   "networking.istio.io",
		Version: "v1",
		Kind:    "VirtualService",
	}}
	input := `{
		"cluster":"home",
		"namespaces":{
			"cve-lab":{
				"networking.istio.io/v1/VirtualService":{"valid":["routing-proof"],"invalid":[]},
				"networking.istio.io/v1/DestinationRule":{"valid":["details"],"invalid":[]}
			}
		}
	}`

	out, err := filterDeniedIstioConfigList(input, denied)
	if err != nil {
		t.Fatalf("filter failed: %v", err)
	}
	if strings.Contains(out, "routing-proof") || strings.Contains(out, "VirtualService") {
		t.Fatalf("denied VirtualService still present: %s", out)
	}
	if !strings.Contains(out, "details") || !strings.Contains(out, "DestinationRule") {
		t.Fatalf("allowed DestinationRule missing: %s", out)
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(out), &root); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
}

func TestParseGroupVersionKindKey(t *testing.T) {
	group, version, kind, ok := parseGroupVersionKindKey("networking.istio.io/v1/VirtualService")
	if !ok || group != "networking.istio.io" || version != "v1" || kind != "VirtualService" {
		t.Fatalf("got %q %q %q ok=%v", group, version, kind, ok)
	}
	group, version, kind, ok = parseGroupVersionKindKey("gateway.networking.k8s.io/v1/HTTPRoute")
	if !ok || group != "gateway.networking.k8s.io" || version != "v1" || kind != "HTTPRoute" {
		t.Fatalf("got %q %q %q ok=%v", group, version, kind, ok)
	}
	if _, _, _, ok := parseGroupVersionKindKey("invalid"); ok {
		t.Fatal("expected parse failure")
	}
}
