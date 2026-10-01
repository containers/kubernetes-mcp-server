package console

import (
	"slices"
	"testing"

	"k8s.io/utils/ptr"
)

func TestTools(t *testing.T) {
	tools := Tools(nil)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	tool := tools[0].Tool
	if tool.Name != "vm_console_screenshot" {
		t.Fatalf("expected name 'vm_console_screenshot', got %q", tool.Name)
	}
	if tool.Description == "" {
		t.Fatal("expected a non-empty description")
	}

	t.Run("is annotated read-only and non-destructive", func(t *testing.T) {
		if !ptr.Deref(tool.Annotations.ReadOnlyHint, false) {
			t.Error("expected ReadOnlyHint true")
		}
		if ptr.Deref(tool.Annotations.DestructiveHint, true) {
			t.Error("expected DestructiveHint false")
		}
	})

	t.Run("requires namespace and name", func(t *testing.T) {
		if !slices.Contains(tool.InputSchema.Required, "namespace") {
			t.Error("expected 'namespace' to be required")
		}
		if !slices.Contains(tool.InputSchema.Required, "name") {
			t.Error("expected 'name' to be required")
		}
	})

	t.Run("exposes an optional wake_screen boolean", func(t *testing.T) {
		prop, ok := tool.InputSchema.Properties["wake_screen"]
		if !ok {
			t.Fatal("expected 'wake_screen' property")
		}
		if prop.Type != "boolean" {
			t.Errorf("expected wake_screen type 'boolean', got %q", prop.Type)
		}
		if slices.Contains(tool.InputSchema.Required, "wake_screen") {
			t.Error("wake_screen should not be required")
		}
	})

	t.Run("has a target compatibility filter", func(t *testing.T) {
		if len(tools[0].TargetCompatibilityFilters) == 0 {
			t.Error("expected a VirtualMachine compatibility filter")
		}
	})
}
