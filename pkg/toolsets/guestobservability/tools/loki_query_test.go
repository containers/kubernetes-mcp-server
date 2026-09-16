package tools

import (
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/guestobservability"
)

func TestInitLokiQuery(t *testing.T) {
	tools := InitLokiQuery()

	if len(tools) != 1 {
		t.Fatalf("expected 1 Loki tool, got %d", len(tools))
	}

	serverTool := tools[0]
	tool := serverTool.Tool

	t.Run("uses expected tool name", func(t *testing.T) {
		expected := "guest-observability_loki_query"

		if tool.Name != expected {
			t.Fatalf(
				"expected tool name %q, got %q",
				expected,
				tool.Name,
			)
		}
	})

	t.Run("has useful description", func(t *testing.T) {
		if tool.Description == "" {
			t.Fatal("expected non-empty tool description")
		}

		expectedFragments := []string{
			"LogQL range query",
			"label contract",
			"namespace",
			"vm_name",
			"KubeVirt VM",
			"os",
			"source",
			"incomplete contract labels",
			"label matcher",
			"excludes streams where that label is absent",
			"Windows guest-log investigations",
			"event IDs",
			"StorPort",
			"Event ID 129",
			"omits the missing identity matcher",
			"available classification labels",
			"specific event signature",
			"cannot establish namespace attribution",
			"cannot establish the affected VM",
			"classification uncertainty",
			"guest telemetry contract warnings",
			"VM attribution is reliable",
		}

		for _, fragment := range expectedFragments {
			if !strings.Contains(tool.Description, fragment) {
				t.Fatalf(
					"expected tool description to contain %q",
					fragment,
				)
			}
		}
	})
	t.Run("requires query argument", func(t *testing.T) {
		schema := tool.InputSchema

		if schema == nil {
			t.Fatal("expected input schema")
		}

		found := false
		for _, required := range schema.Required {
			if required == "query" {
				found = true
				break
			}
		}

		if !found {
			t.Fatal("expected query to be required")
		}
	})

	t.Run("defines expected properties", func(t *testing.T) {
		schema := tool.InputSchema

		expected := []string{
			"query",
			"start",
			"end",
			"limit",
			"direction",
			"step",
		}

		for _, name := range expected {
			if _, ok := schema.Properties[name]; !ok {
				t.Fatalf(
					"expected schema property %q",
					name,
				)
			}
		}
	})

	t.Run("does not advertise unsupported since argument", func(t *testing.T) {
		schema := tool.InputSchema

		if schema == nil {
			t.Fatal("expected input schema")
		}

		if _, ok := schema.Properties["since"]; ok {
			t.Fatal("did not expect unsupported since property")
		}
	})

	t.Run("limit defaults to safe value", func(t *testing.T) {
		property := tool.InputSchema.Properties["limit"]

		if property == nil {
			t.Fatal("limit property not found")
		}

		if property.Default == nil {
			t.Fatal("expected default limit")
		}

		expected := guestobservability.DefaultLokiLimit

		if string(property.Default) != "100" {
			t.Fatalf(
				"expected limit default %d, got %s",
				expected,
				string(property.Default),
			)
		}
	})

	t.Run("direction exposes forward and backward", func(t *testing.T) {
		property := tool.InputSchema.Properties["direction"]

		if property == nil {
			t.Fatal("direction property not found")
		}

		if len(property.Enum) != 2 {
			t.Fatalf(
				"expected 2 direction values, got %d",
				len(property.Enum),
			)
		}

		values := map[string]bool{}

		for _, value := range property.Enum {
			if direction, ok := value.(string); ok {
				values[direction] = true
			}
		}

		if !values["forward"] {
			t.Fatal("expected forward direction")
		}

		if !values["backward"] {
			t.Fatal("expected backward direction")
		}
	})

	t.Run("tool is read only and non destructive", func(t *testing.T) {
		annotations := tool.Annotations

		if annotations.ReadOnlyHint == nil ||
			!*annotations.ReadOnlyHint {
			t.Fatal("expected read-only hint")
		}

		if annotations.DestructiveHint == nil ||
			*annotations.DestructiveHint {
			t.Fatal("expected non-destructive hint")
		}

		if annotations.IdempotentHint == nil ||
			!*annotations.IdempotentHint {
			t.Fatal("expected idempotent hint")
		}
	})

	t.Run("handler is registered", func(t *testing.T) {
		if serverTool.Handler == nil {
			t.Fatal("expected Loki query handler")
		}
	})
}
