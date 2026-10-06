package kubevirt

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

func getVMI(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
	vmi, err := dynamicClient.Resource(VirtualMachineInstanceGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		switch {
		case apierrors.IsNotFound(err):
			return nil, &ConsoleError{
				Code: ConsoleCodeVMINotFound,
				Err:  fmt.Errorf("no running VirtualMachineInstance %q in namespace %q: the VirtualMachine does not exist or is not started", name, namespace),
			}
		case apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err):
			return nil, &ConsoleError{
				Code: ConsoleCodePermissionDenied,
				Err:  fmt.Errorf("permission denied reading VirtualMachineInstance %q in namespace %q: %w", name, namespace, err),
			}
		}
		return nil, &ConsoleError{Code: ConsoleCodeInternal, Err: fmt.Errorf("failed to retrieve the VirtualMachineInstance: %w", err)}
	}
	return vmi, nil
}

func vmiPhase(vmi *unstructured.Unstructured) string {
	if vmi == nil {
		return ""
	}
	phase, _, _ := unstructured.NestedString(vmi.Object, "status", "phase")
	return phase
}

func graphicsEnabled(vmi *unstructured.Unstructured) bool {
	if vmi == nil {
		return false
	}
	enabled, found, err := unstructured.NestedBool(vmi.Object, "spec", "domain", "devices", "autoattachGraphicsDevice")
	if err != nil || !found {
		return true
	}
	return enabled
}
