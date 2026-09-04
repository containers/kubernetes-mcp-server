package kubevirt

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newTestVMI(namespace, name, phase string, autoattachGraphics *bool) *unstructured.Unstructured {
	devices := map[string]any{}
	if autoattachGraphics != nil {
		devices["autoattachGraphicsDevice"] = *autoattachGraphics
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachineInstance",
		"metadata": map[string]any{
			"namespace": namespace,
			"name":      name,
		},
		"spec": map[string]any{
			"domain": map[string]any{
				"devices": devices,
			},
		},
		"status": map[string]any{
			"phase": phase,
		},
	}}
}

func newFakeDynamicClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		VirtualMachineInstanceGVR: "VirtualMachineInstanceList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
}

func requireConsoleCode(t *testing.T, err error, want ConsoleErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %q, got nil", want)
	}
	var ce *ConsoleError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConsoleError, got %T: %v", err, err)
	}
	if ce.Code != want {
		t.Fatalf("expected code %q, got %q (%s)", want, ce.Code, ce.Message)
	}
}

func TestGetVMI(t *testing.T) {
	ctx := context.Background()

	t.Run("retrieves an existing VMI", func(t *testing.T) {
		client := newFakeDynamicClient(newTestVMI("default", "test-vm", "Running", nil))
		vmi, err := getVMI(ctx, client, "default", "test-vm")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vmi.GetName() != "test-vm" {
			t.Fatalf("expected name 'test-vm', got %q", vmi.GetName())
		}
	})

	t.Run("maps NotFound to vmi_not_found", func(t *testing.T) {
		client := newFakeDynamicClient()
		_, err := getVMI(ctx, client, "default", "missing")
		requireConsoleCode(t, err, ConsoleCodeVMINotFound)
	})

	t.Run("maps Forbidden to permission_denied", func(t *testing.T) {
		client := newFakeDynamicClient()
		client.PrependReactor("get", "virtualmachineinstances", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachineinstances"}, "test-vm", errors.New("nope"))
		})
		_, err := getVMI(ctx, client, "default", "test-vm")
		requireConsoleCode(t, err, ConsoleCodePermissionDenied)
	})
}

func TestVMIPhase(t *testing.T) {
	if got := vmiPhase(newTestVMI("default", "vm", "Running", nil)); got != "Running" {
		t.Fatalf("expected 'Running', got %q", got)
	}
	if got := vmiPhase(nil); got != "" {
		t.Fatalf("expected empty phase for nil VMI, got %q", got)
	}
}

func TestGraphicsEnabled(t *testing.T) {
	tru, fls := true, false
	t.Run("enabled by default when field absent", func(t *testing.T) {
		if !graphicsEnabled(newTestVMI("default", "vm", "Running", nil)) {
			t.Fatal("expected graphics enabled when autoattachGraphicsDevice is unset")
		}
	})
	t.Run("enabled when explicitly true", func(t *testing.T) {
		if !graphicsEnabled(newTestVMI("default", "vm", "Running", &tru)) {
			t.Fatal("expected graphics enabled")
		}
	})
	t.Run("disabled when explicitly false", func(t *testing.T) {
		if graphicsEnabled(newTestVMI("default", "vm", "Running", &fls)) {
			t.Fatal("expected graphics disabled")
		}
	})
}
