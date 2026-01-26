package tools

import "testing"

func TestValidateIstioConfigInputRejectsReadActions(t *testing.T) {
	tests := []struct {
		name   string
		action string
	}{
		{name: "list is rejected", action: "list"},
		{name: "get is rejected", action: "get"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := validateIstioConfigInput(tt.action, "ns", "group", "v1", "Kind", "name", "{}")
			if err == nil {
				t.Fatalf("expected error for action %q, got nil", tt.action)
			}
			expected := "invalid action " + `"` + tt.action + `"` + ": must be one of create, patch, delete"
			if err.Error() != expected {
				t.Fatalf("error mismatch: got %q, want %q", err.Error(), expected)
			}
		})
	}
}

func TestValidateIstioConfigInputReadRejectsWriteActions(t *testing.T) {
	tests := []struct {
		name   string
		action string
	}{
		{name: "create is rejected", action: "create"},
		{name: "patch is rejected", action: "patch"},
		{name: "delete is rejected", action: "delete"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := validateIstioConfigInputRead(tt.action, "ns", "group", "v1", "Kind", "name")
			if err == nil {
				t.Fatalf("expected error for action %q, got nil", tt.action)
			}
			expected := "invalid action " + `"` + tt.action + `"` + ": must be one of list, get"
			if err.Error() != expected {
				t.Fatalf("error mismatch: got %q, want %q", err.Error(), expected)
			}
		})
	}
}
